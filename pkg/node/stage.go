package node

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (n *Node) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	Logger(ctx).Info("NodeStageVolume start",
		zap.String("volume_id", req.GetVolumeId()),
		zap.String("staging_target_path", req.GetStagingTargetPath()),
	)
	// Check if volume_id is provided
	if req.GetVolumeId() == "" {
		Logger(ctx).Error("NodeStageVolume invalid argument: volume_id is required")
		return nil, status.Error(codes.InvalidArgument, "volume_id is required")
	}

	// Check if staging_target_path is provided
	if req.GetStagingTargetPath() == "" {
		Logger(ctx).Error("NodeStageVolume invalid argument: staging_target_path is required")
		return nil, status.Error(codes.InvalidArgument, "staging_target_path is required")
	}

	// Check if volume_capability is provided
	if req.GetVolumeCapability() == nil {
		Logger(ctx).Error("NodeStageVolume invalid argument: volume_capability is required")
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
		Logger(ctx).Error("NodeStageVolume invalid argument: fsType is required")
		return nil, status.Error(codes.InvalidArgument, "fsType is required in volume capability or volume context")
	}

	// Retrieve and apply file mode from VolumeContext; fileMode is required
	modeStr, ok := req.GetVolumeContext()["fileMode"]
	if !ok {
		Logger(ctx).Error("NodeStageVolume invalid argument: fileMode is required")
		return nil, status.Error(codes.InvalidArgument, "fileMode is a required parameter in VolumeContext")
	}
	mode, err := strconv.ParseUint(modeStr, 8, 32)
	if err != nil {
		Logger(ctx).Error("NodeStageVolume invalid argument: invalid fileMode", zap.Error(err))
		return nil, status.Errorf(codes.InvalidArgument, "invalid file mode: %v", err)
	}
	fileMode := os.FileMode(mode)

	// Retrieve mount source from VolumeContext
	source, ok := req.GetVolumeContext()["source"]
	if !ok || source == "" {
		Logger(ctx).Error("NodeStageVolume invalid argument: source is required")
		return nil, status.Error(codes.InvalidArgument, "source is a required parameter in VolumeContext")
	}

	// Create the staging path if it doesn't exist
	volumePath := req.GetStagingTargetPath()
	if err := os.MkdirAll(volumePath, 0755); err != nil {
		Logger(ctx).Error("NodeStageVolume failed to create staging path", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to create staging path: %v", err)
	}

	// If already mounted, return success (idempotent)
	isMounted, err := n.mounter.IsMountPoint(volumePath)
	if err == nil && isMounted {
		Logger(ctx).Info("NodeStageVolume already mounted")
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
			Logger(ctx).Info("mount failed with ENODEV, trying helper",
				zap.String("fs_type", fsType),
				zap.String("source", source),
				zap.String("target", volumePath),
				zap.String("opts", opts),
			)
			out, execErr := mountHelper(fsType, source, volumePath, opts)
			if execErr != nil {
				Logger(ctx).Error("mount helper failed",
					zap.String("fs_type", fsType),
					zap.String("source", source),
					zap.String("target", volumePath),
					zap.String("opts", opts),
					zap.String("output", out),
					zap.Error(execErr),
				)
				return nil, status.Errorf(
					codes.Internal,
					"failed to mount volume (fsType=%q): syscall mount returned ENODEV and helper failed; ensure mount.%s is installed in the node image and /dev/fuse is available, or ensure kernel support for %s. helper error: %v",
					fsType,
					fsType,
					fsType,
					execErr,
				)
			}
			Logger(ctx).Info("mount helper succeeded",
				zap.String("fs_type", fsType),
				zap.String("source", source),
				zap.String("target", volumePath),
				zap.String("opts", opts),
				zap.String("output", out),
			)
			logMountInfo(ctx, volumePath, "mountinfo after helper")
			time.Sleep(1 * time.Second)
			logMountInfo(ctx, volumePath, "mountinfo after helper delay")
		} else {
			Logger(ctx).Error("mount failed",
				zap.String("fs_type", fsType),
				zap.String("source", source),
				zap.String("target", volumePath),
				zap.String("opts", opts),
				zap.Error(err),
			)
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
	logMountInfo(ctx, volumePath, "mountinfo after syscall mount")
	time.Sleep(1 * time.Second)
	logMountInfo(ctx, volumePath, "mountinfo after syscall mount delay")

	// Re-apply file mode after mounting, as mount may override permissions
	if err := os.Chmod(volumePath, fileMode); err != nil {
		Logger(ctx).Error("NodeStageVolume failed to set file mode", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to set file mode after mount: %v", err)
	}

	// Return success if mounting succeeded
	Logger(ctx).Info("NodeStageVolume complete")
	return &csi.NodeStageVolumeResponse{}, nil
}

func (n *Node) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	Logger(ctx).Info("NodeUnstageVolume start",
		zap.String("volume_id", req.GetVolumeId()),
		zap.String("staging_target_path", req.GetStagingTargetPath()),
	)
	// Check if volume_id is provided
	if req.GetVolumeId() == "" {
		Logger(ctx).Error("NodeUnstageVolume invalid argument: volume_id is required")
		return nil, status.Error(codes.InvalidArgument, "volume_id is required")
	}

	// Check if staging_target_path is provided
	if req.GetStagingTargetPath() == "" {
		Logger(ctx).Error("NodeUnstageVolume invalid argument: staging_target_path is required")
		return nil, status.Error(codes.InvalidArgument, "staging_target_path is required")
	}

	// Attempt to unmount the staging target path
	err := n.mounter.Unmount(req.GetStagingTargetPath(), 0)
	if err != nil {
		Logger(ctx).Error("NodeUnstageVolume failed to unmount staging target path", zap.Error(err))
		return nil, status.Errorf(codes.Internal, "failed to unmount staging target path: %v", err)
	}

	// Return success response
	Logger(ctx).Info("NodeUnstageVolume complete")
	return &csi.NodeUnstageVolumeResponse{}, nil
}

func isNoSuchDevice(err error) bool {
	if errors.Is(err, syscall.ENODEV) {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "no such device")
}

func execMountHelper(fsType, source, target, opts string) (string, error) {
	helper := "mount." + fsType
	helperPath, err := exec.LookPath(helper)
	if err != nil {
		if alt := "/sbin/" + helper; fileExists(alt) {
			helperPath = alt
		} else if alt := "/usr/sbin/" + helper; fileExists(alt) {
			helperPath = alt
		} else {
			return "", fmt.Errorf("mount helper not found in PATH or /sbin:/usr/sbin (%s)", helper)
		}
	}

	args := []string{"-x", helperPath, source, target}
	if strings.TrimSpace(opts) != "" {
		args = append(args, "-o", opts)
	}
	cmd := exec.Command("sh", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return strings.TrimSpace(string(out)), fmt.Errorf("mount helper failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func logMountInfo(ctx context.Context, path, message string) {
	if line, ok := findMountInfoLine(path); ok {
		Logger(ctx).Info(message, zap.String("path", path), zap.String("mountinfo", line))
		return
	}
	Logger(ctx).Info(message+": path not found in mountinfo",
		zap.String("path", path),
		zap.Strings("mountinfo_sample", mountInfoSample(8)),
	)
}

func mountInfoSample(limit int) []string {
	data, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return []string{fmt.Sprintf("read mountinfo failed: %v", err)}
	}
	lines := strings.Split(string(data), "\n")
	var out []string
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		out = append(out, line)
		if len(out) >= limit {
			break
		}
	}
	return out
}

var mountHelper = execMountHelper

func isPermissionError(err error) bool {
	if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "permission denied") || strings.Contains(msg, "operation not permitted")
}
