package node

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (n *Node) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	Logger(ctx).Info("NodePublishVolume start",
		zap.String("volume_id", req.GetVolumeId()),
		zap.String("staging_target_path", req.GetStagingTargetPath()),
		zap.String("target_path", req.GetTargetPath()),
	)
	// Check if volume_id is provided
	if req.GetVolumeId() == "" {
		Logger(ctx).Error("NodePublishVolume invalid argument: volume_id is required")
		return nil, status.Error(codes.InvalidArgument, "volume_id is required")
	}

	// Check if target_path is provided
	if req.GetTargetPath() == "" {
		Logger(ctx).Error("NodePublishVolume invalid argument: target_path is required")
		return nil, status.Error(codes.InvalidArgument, "target_path is required")
	}

	// Check if volume_capability is provided
	if req.GetVolumeCapability() == nil {
		Logger(ctx).Error("NodePublishVolume invalid argument: volume_capability is required")
		return nil, status.Error(codes.InvalidArgument, "volume_capability is required")
	}

	// Check if the staging path is provided, as required for bind-mounting
	if req.GetStagingTargetPath() == "" {
		Logger(ctx).Error("NodePublishVolume invalid argument: staging_target_path is required")
		return nil, status.Error(codes.InvalidArgument, "staging_target_path is required")
	}

	// Ensure the target path exists
	if err := os.MkdirAll(req.GetTargetPath(), 0755); err != nil {
		Logger(ctx).Error("NodePublishVolume failed to create target path", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to create target path: %v", err)
	}

	repaired, err := n.waitForMountReady(ctx, req)
	if err != nil {
		if repaired {
			return nil, err
		}
		for i := 0; i < 3; i++ {
			time.Sleep(100 * time.Millisecond)
			repaired, err = n.waitForMountReady(ctx, req)
			if err == nil {
				break
			}
			if repaired {
				return nil, err
			}
		}
		if err != nil {
			return nil, err
		}
	}

	if published, err := n.preparePublishTarget(ctx, req); err != nil {
		return nil, err
	} else if published {
		Logger(ctx).Info("NodePublishVolume complete: target path already mounted and usable")
		return &csi.NodePublishVolumeResponse{}, nil
	}

	// Perform a bind mount from the staging path to the target path
	err = n.mounter.Mount(req.GetStagingTargetPath(), req.GetTargetPath(), "", syscall.MS_BIND, "")
	if err != nil {
		Logger(ctx).Error("failed to bind-mount volume",
			zap.String("staging_target_path", req.GetStagingTargetPath()),
			zap.String("target_path", req.GetTargetPath()),
			zap.Error(err),
		)
		return nil, status.Errorf(codes.Internal, "failed to bind-mount volume: %v", err)
	}

	// Return success response
	Logger(ctx).Info("NodePublishVolume complete")
	return &csi.NodePublishVolumeResponse{}, nil
}

func (n *Node) waitForMountReady(ctx context.Context, req *csi.NodePublishVolumeRequest) (bool, error) {
	path := req.GetStagingTargetPath()
	isMounted, err := n.mounter.IsMountPoint(path)
	if err != nil {
		Logger(ctx).Error("failed to verify if path is a mount point",
			zap.String("path", path),
			zap.Error(err),
		)
		return false, status.Errorf(codes.Internal, "failed to verify path mountpoint: %v", err)
	}
	if !isMounted {
		Logger(ctx).Warn("path is not a mount point", zap.String("path", path))
		if line, ok := findMountInfoLine(path); ok {
			Logger(ctx).Info("mountinfo for path", zap.String("mountinfo", line))
		} else {
			Logger(ctx).Info("mountinfo does not contain path")
		}
		return false, status.Error(codes.FailedPrecondition, "path is not a mount point")
	}
	if err := probeMountPath(path); err != nil {
		Logger(ctx).Warn("mount point is not usable",
			zap.String("path", path),
			zap.Error(err),
		)
		if !isDisconnectedMountError(err) {
			return false, status.Errorf(codes.FailedPrecondition, "mount point is not usable: %v", err)
		}

		Logger(ctx).Warn("NodePublishVolume unstaging disconnected staging mount",
			zap.String("staging_target_path", path),
			zap.Error(err),
		)
		n.reportRepairStarted(ctx, req, "JustmountStagingMountDisconnected",
			"Disconnected justmount staging mount detected; unmounting dependent bind mounts and staging target")
		if err := n.unmountDependentMounts(ctx, path); err != nil {
			return false, status.Errorf(codes.Internal, "failed to unmount dependent bind mounts: %v", err)
		}
		if err := n.unmountAllAtPath(ctx, path); err != nil {
			return false, status.Errorf(codes.Internal, "failed to unmount disconnected staging target path: %v", err)
		}
		n.reportRepairCompleted(ctx, req, "JustmountStagingMountUnstaged",
			"Disconnected justmount staging mount and dependent bind mounts were unmounted successfully")
		return true, status.Error(codes.FailedPrecondition, "staging mount was disconnected and has been unstaged; retry after staging")
	}
	return false, nil
}

func (n *Node) preparePublishTarget(ctx context.Context, req *csi.NodePublishVolumeRequest) (bool, error) {
	targetPath := req.GetTargetPath()
	isMounted, err := n.mounter.IsMountPoint(targetPath)
	if err != nil {
		Logger(ctx).Error("NodePublishVolume failed to check target mountpoint",
			zap.String("target_path", targetPath),
			zap.Error(err),
		)
		return false, status.Errorf(codes.Internal, "failed to verify target path mountpoint: %v", err)
	}
	if !isMounted {
		return false, nil
	}

	if err := probeMountPath(targetPath); err == nil {
		Logger(ctx).Info("NodePublishVolume target path already mounted and usable",
			zap.String("target_path", targetPath),
		)
		return true, nil
	} else if !isDisconnectedMountError(err) {
		Logger(ctx).Error("NodePublishVolume target path is mounted but not usable",
			zap.String("target_path", targetPath),
			zap.Error(err),
		)
		return false, status.Errorf(codes.Internal, "target_path is mounted but not usable: %v", err)
	}

	Logger(ctx).Warn("NodePublishVolume replacing disconnected target bind mount",
		zap.String("target_path", targetPath),
	)
	n.reportRepairStarted(ctx, req, "JustmountBindMountDisconnected",
		"Disconnected justmount bind mount detected; replacing target bind mount")
	if err := n.unmountAllAtPath(ctx, targetPath); err != nil {
		return false, status.Errorf(codes.Internal, "failed to unmount disconnected target path: %v", err)
	}
	n.reportRepairCompleted(ctx, req, "JustmountBindMountReplaced",
		"Disconnected justmount bind mount was unmounted successfully and will be replaced")
	return false, nil
}

func (n *Node) reportRepairStarted(
	ctx context.Context,
	req *csi.NodePublishVolumeRequest,
	reason string,
	message string,
) {
	if n.pvcReporter == nil {
		return
	}
	if err := n.pvcReporter.RepairStarted(ctx, req, reason, message); err != nil {
		Logger(ctx).Warn("failed to report justmount repair start",
			zap.String("reason", reason),
			zap.Error(err),
		)
	}
}

func (n *Node) reportRepairCompleted(
	ctx context.Context,
	req *csi.NodePublishVolumeRequest,
	reason string,
	message string,
) {
	if n.pvcReporter == nil {
		return
	}
	if err := n.pvcReporter.RepairCompleted(ctx, req, reason, message); err != nil {
		Logger(ctx).Warn("failed to report justmount repair completion",
			zap.String("reason", reason),
			zap.Error(err),
		)
	}
}

func findMountInfoLine(path string) (string, bool) {
	data, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return "", false
	}
	cleaned := filepath.Clean(path)
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.SplitN(line, " - ", 2)
		fields := strings.Fields(parts[0])
		if len(fields) < 5 {
			continue
		}
		mountPoint := unescapeMountPath(fields[4])
		if filepath.Clean(mountPoint) == cleaned {
			return line, true
		}
	}
	return "", false
}

func unescapeMountPath(path string) string {
	replacer := strings.NewReplacer(
		"\\040", " ",
		"\\011", "\t",
		"\\012", "\n",
		"\\134", "\\",
	)
	return replacer.Replace(path)
}

func (n *Node) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	Logger(ctx).Info("NodeUnpublishVolume start",
		zap.String("volume_id", req.GetVolumeId()),
		zap.String("target_path", req.GetTargetPath()),
	)
	// Check if volume_id is provided
	if req.GetVolumeId() == "" {
		Logger(ctx).Error("NodeUnpublishVolume invalid argument: volume_id is required")
		return nil, status.Error(codes.InvalidArgument, "volume_id is required")
	}

	// Check if target_path is provided
	if req.GetTargetPath() == "" {
		Logger(ctx).Error("NodeUnpublishVolume invalid argument: target_path is required")
		return nil, status.Error(codes.InvalidArgument, "target_path is required")
	}

	targetPath := filepath.Clean(req.GetTargetPath())
	if targetPath == "/" || targetPath == "." {
		Logger(ctx).Error("NodeUnpublishVolume invalid argument: unsafe target path",
			zap.String("target_path", req.GetTargetPath()),
			zap.String("cleaned_target_path", targetPath),
		)
		return nil, status.Error(codes.InvalidArgument, "unsafe target_path")
	}

	// Unmount all stacked mount layers at this path.
	for i := 0; i < 10; i++ {
		isMounted, err := n.mounter.IsMountPoint(targetPath)
		if err != nil {
			Logger(ctx).Error("NodeUnpublishVolume failed to check mountpoint",
				zap.String("target_path", targetPath),
				zap.Error(err),
			)
			return nil, status.Errorf(codes.Internal, "failed to verify target path mountpoint: %v", err)
		}
		if !isMounted {
			break
		}

		if err := n.unmountOnce(ctx, targetPath, i+1); err != nil {
			return nil, status.Errorf(codes.Internal, "failed to unmount target path: %v", err)
		}
	}

	// Ensure no mount remains before removing the target directory.
	if isMounted, err := n.mounter.IsMountPoint(targetPath); err != nil {
		Logger(ctx).Error("NodeUnpublishVolume failed final mountpoint check",
			zap.String("target_path", targetPath),
			zap.Error(err),
		)
		return nil, status.Errorf(codes.Internal, "failed to verify target path mountpoint: %v", err)
	} else if isMounted {
		Logger(ctx).Error("NodeUnpublishVolume mountpoint still present after unmount attempts",
			zap.String("target_path", targetPath),
		)
		return nil, status.Error(codes.Internal, "target_path remains mounted after unmount attempts")
	}

	// Never recurse during cleanup. target_path should be an empty directory.
	if err := os.Remove(targetPath); err != nil && !os.IsNotExist(err) {
		Logger(ctx).Error("NodeUnpublishVolume failed to remove target path", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to remove target path: %v", err)
	}

	Logger(ctx).Info("NodeUnpublishVolume complete")
	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (n *Node) unmountAllAtPath(ctx context.Context, path string) error {
	for i := 0; i < 10; i++ {
		isMounted, err := n.mounter.IsMountPoint(path)
		if err != nil {
			return fmt.Errorf("verify mountpoint %q: %w", path, err)
		}
		if !isMounted {
			return nil
		}
		if err := n.unmountOnce(ctx, path, i+1); err != nil {
			return err
		}
	}

	isMounted, err := n.mounter.IsMountPoint(path)
	if err != nil {
		return fmt.Errorf("verify mountpoint %q after unmount attempts: %w", path, err)
	}
	if isMounted {
		return fmt.Errorf("%q remains mounted after unmount attempts", path)
	}
	return nil
}

func (n *Node) unmountOnce(ctx context.Context, path string, attempt int) error {
	err := n.mounter.Unmount(path, 0)
	if err == nil || errors.Is(err, syscall.EINVAL) {
		return nil
	}
	Logger(ctx).Error("failed to unmount path",
		zap.String("path", path),
		zap.Int("attempt", attempt),
		zap.Error(err),
	)
	return err
}
