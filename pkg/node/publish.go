package node

import (
	"context"
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

	// Ensure the staging path is a mount point
	isMounted, err := n.mounter.IsMountPoint(req.GetStagingTargetPath())
	if err != nil {
		Logger(ctx).Error("failed to verify if staging path is a mount point", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to verify if staging path is a mount point: %v", err)
	}
	if !isMounted {
		for i := 0; i < 3; i++ {
			time.Sleep(100 * time.Millisecond)
			isMounted, err = n.mounter.IsMountPoint(req.GetStagingTargetPath())
			if err != nil {
				Logger(ctx).Error("failed to verify if staging path is a mount point",
					zap.Int("retry", i+1),
					zap.Error(err),
				)
				return nil, status.Errorf(codes.Internal, "failed to verify if staging path is a mount point: %v", err)
			}
			if isMounted {
				break
			}
		}
	}
	if !isMounted {
		Logger(ctx).Warn("staging path is not a mount point", zap.String("staging_target_path", req.GetStagingTargetPath()))
		if line, ok := findMountInfoLine(req.GetStagingTargetPath()); ok {
			Logger(ctx).Info("mountinfo for staging path", zap.String("mountinfo", line))
		} else {
			Logger(ctx).Info("mountinfo does not contain staging path")
		}
		return nil, status.Error(codes.FailedPrecondition, "staging_target_path is not a mount point")
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

	// Attempt to unmount the target path
	err := n.mounter.Unmount(req.GetTargetPath(), 0)
	if err != nil {
		Logger(ctx).Error("NodeUnpublishVolume failed to unmount target path", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to unmount target path: %v", err)
	}

	// Attempt to remove the target path
	if err := os.RemoveAll(req.GetTargetPath()); err != nil {
		Logger(ctx).Error("NodeUnpublishVolume failed to remove target path", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to remove target path: %v", err)
	}

	Logger(ctx).Info("NodeUnpublishVolume complete")
	return &csi.NodeUnpublishVolumeResponse{}, nil
}
