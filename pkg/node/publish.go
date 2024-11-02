package node

import (
	"context"
	"os"
	"syscall"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
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

	// Implement the actual logic to publish the volume here
	// For now, return success as a placeholder
	return &csi.NodePublishVolumeResponse{}, nil
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
	err := syscall.Unmount(req.GetTargetPath(), 0)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to unmount target path: %v", err)
	}

	// Attempt to remove the target path
	if err := os.RemoveAll(req.GetTargetPath()); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to remove target path: %v", err)
	}

	return &csi.NodeUnpublishVolumeResponse{}, nil
}
