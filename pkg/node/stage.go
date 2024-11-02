package node

import (
	"context"
	"os"
	"strconv"
	"syscall"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (n *Node) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	// Check if volume_id is provided
	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "volume_id is required")
	}

	// Check if staging_target_path is provided
	if req.GetStagingTargetPath() == "" {
		return nil, status.Error(codes.InvalidArgument, "staging_target_path is required")
	}

	// Check if volume_capability is provided
	if req.GetVolumeCapability() == nil {
		return nil, status.Error(codes.InvalidArgument, "volume_capability is required")
	}

	// Retrieve the fsType from volume capability and ensure it is specified
	mount := req.GetVolumeCapability().GetMount()
	if mount == nil || mount.FsType == "" {
		return nil, status.Error(codes.InvalidArgument, "fsType is required in volume capability")
	}
	fsType := mount.FsType

	// Retrieve and apply file mode from VolumeContext; fileMode is required
	modeStr, ok := req.GetVolumeContext()["fileMode"]
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "fileMode is a required parameter in VolumeContext")
	}
	mode, err := strconv.ParseUint(modeStr, 8, 32)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid file mode: %v", err)
	}
	fileMode := os.FileMode(mode)

	// Create a unique subdirectory for the volume within the staging path, using the specified fileMode
	volumePath := req.GetStagingTargetPath()

	// Perform the mount operation with the specified fsType
	err = syscall.Mount(req.GetVolumeId(), volumePath, fsType, 0, "")
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to mount volume: %v", err)
	}

	// Re-apply file mode after mounting, as mount may override permissions
	if err := os.Chmod(volumePath, fileMode); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to set file mode after mount: %v", err)
	}

	// Return success if mounting succeeded
	return &csi.NodeStageVolumeResponse{}, nil
}

func (n *Node) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	// Check if volume_id is provided
	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "volume_id is required")
	}

	// Check if staging_target_path is provided
	if req.GetStagingTargetPath() == "" {
		return nil, status.Error(codes.InvalidArgument, "staging_target_path is required")
	}

	// Attempt to unmount the staging target path
	err := syscall.Unmount(req.GetStagingTargetPath(), 0)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to unmount staging target path: %v", err)
	}

	// Return success response
	return &csi.NodeUnstageVolumeResponse{}, nil
}
