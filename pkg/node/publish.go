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
	// Check if volume_id is provided
	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "volume_id is required")
	}

	// Check if target_path is provided
	if req.GetTargetPath() == "" {
		return nil, status.Error(codes.InvalidArgument, "target_path is required")
	}

	// Check if volume_capability is provided
	if req.GetVolumeCapability() == nil {
		return nil, status.Error(codes.InvalidArgument, "volume_capability is required")
	}

	// Check if the staging path is provided, as required for bind-mounting
	if req.GetStagingTargetPath() == "" {
		return nil, status.Error(codes.InvalidArgument, "staging_target_path is required")
	}

	// Ensure the target path exists
	if err := os.MkdirAll(req.GetTargetPath(), 0755); err != nil {
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
	// Check if volume_id is provided
	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "volume_id is required")
	}

	// Check if target_path is provided
	if req.GetTargetPath() == "" {
		return nil, status.Error(codes.InvalidArgument, "target_path is required")
	}

	// Attempt to unmount the target path
	err := n.mounter.Unmount(req.GetTargetPath(), 0)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to unmount target path: %v", err)
	}

	// Attempt to remove the target path
	if err := os.RemoveAll(req.GetTargetPath()); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to remove target path: %v", err)
	}

	return &csi.NodeUnpublishVolumeResponse{}, nil
}
