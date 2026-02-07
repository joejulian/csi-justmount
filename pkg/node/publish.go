package node

import (
	"context"
	"log"
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
		log.Printf("failed to verify if staging path is a mount point: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to verify if staging path is a mount point: %v", err)
	}
	if !isMounted {
		return nil, status.Error(codes.FailedPrecondition, "staging_target_path is not a mount point")
	}

	// Perform a bind mount from the staging path to the target path
	err = n.mounter.Mount(req.GetStagingTargetPath(), req.GetTargetPath(), "", syscall.MS_BIND, "")
	if err != nil {
		log.Printf("failed to bind-mount volume from %s to %s: %v", req.GetStagingTargetPath(), req.GetTargetPath(), err)
		return nil, status.Errorf(codes.Internal, "failed to bind-mount volume: %v", err)
	}

	// Return success response
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
