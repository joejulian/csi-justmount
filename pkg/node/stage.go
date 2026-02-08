package node

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
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
	fsType := ""
	if mount != nil {
		fsType = mount.FsType
	}
	if fsType == "" {
		if v, ok := req.GetVolumeContext()["fsType"]; ok {
			fsType = v
		}
	}
	if fsType == "" {
		return nil, status.Error(codes.InvalidArgument, "fsType is required in volume capability or volume context")
	}

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

	// Retrieve mount source from VolumeContext
	source, ok := req.GetVolumeContext()["source"]
	if !ok || source == "" {
		return nil, status.Error(codes.InvalidArgument, "source is a required parameter in VolumeContext")
	}

	// Create the staging path if it doesn't exist
	volumePath := req.GetStagingTargetPath()
	if err := os.MkdirAll(volumePath, 0755); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create staging path: %v", err)
	}

	// If already mounted, return success (idempotent)
	isMounted, err := n.mounter.IsMountPoint(volumePath)
	if err == nil && isMounted {
		return &csi.NodeStageVolumeResponse{}, nil
	}

	// Parse mount options
	opts := strings.TrimSpace(req.GetVolumeContext()["mountOptions"])
	var flags uintptr
	var dataOpts []string
	if opts != "" {
		for _, opt := range strings.Split(opts, ",") {
			o := strings.TrimSpace(opt)
			if o == "" {
				continue
			}
			switch o {
			case "ro":
				flags |= syscall.MS_RDONLY
			case "rw":
				// no flag needed
			case "nosuid":
				flags |= syscall.MS_NOSUID
			case "nodev":
				flags |= syscall.MS_NODEV
			case "noexec":
				flags |= syscall.MS_NOEXEC
			case "noatime":
				flags |= syscall.MS_NOATIME
			case "relatime":
				flags |= syscall.MS_RELATIME
			default:
				dataOpts = append(dataOpts, o)
			}
		}
	}
	data := strings.Join(dataOpts, ",")

	// Perform the mount operation with the specified fsType
	err = n.mounter.Mount(source, volumePath, fsType, flags, data)
	if err != nil {
		if isNoSuchDevice(err) {
			log.Printf("mount failed with ENODEV (fsType=%q source=%q target=%q opts=%q), trying helper", fsType, source, volumePath, opts)
			if execErr := mountHelper(fsType, source, volumePath, opts); execErr != nil {
				log.Printf("mount helper failed (fsType=%q source=%q target=%q opts=%q): %v", fsType, source, volumePath, opts, execErr)
				return nil, status.Errorf(
					codes.Internal,
					"failed to mount volume (fsType=%q): syscall mount returned ENODEV and helper failed; ensure mount.%s is installed in the node image and /dev/fuse is available, or ensure kernel support for %s. helper error: %v",
					fsType,
					fsType,
					fsType,
					execErr,
				)
			}
			log.Printf("mount helper succeeded (fsType=%q source=%q target=%q opts=%q)", fsType, source, volumePath, opts)
	} else {
		log.Printf("mount failed (fsType=%q source=%q target=%q opts=%q): %v", fsType, source, volumePath, opts, err)
		if isPermissionError(err) {
			return nil, status.Errorf(
				codes.Internal,
				"failed to mount volume (fsType=%q): permission denied; ensure the node plugin has CAP_SYS_ADMIN (or privileged), and /dev/fuse is available for FUSE filesystems. mount error: %v",
				fsType,
				err,
			)
		}
		return nil, status.Errorf(codes.Internal, "failed to mount volume (fsType=%q): %v", fsType, err)
	}
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
	err := n.mounter.Unmount(req.GetStagingTargetPath(), 0)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to unmount staging target path: %v", err)
	}

	// Return success response
	return &csi.NodeUnstageVolumeResponse{}, nil
}

func isNoSuchDevice(err error) bool {
	if errors.Is(err, syscall.ENODEV) {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "no such device")
}

func execMountHelper(fsType, source, target, opts string) error {
	args := []string{"-t", fsType}
	if strings.TrimSpace(opts) != "" {
		args = append(args, "-o", opts)
	}
	args = append(args, source, target)
	cmd := exec.Command("mount", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return fmt.Errorf("mount helper not found in PATH (mount/mount.%s): %w", fsType, err)
		}
		return fmt.Errorf("mount helper failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

var mountHelper = execMountHelper

func isPermissionError(err error) bool {
	if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "permission denied") || strings.Contains(msg, "operation not permitted")
}
