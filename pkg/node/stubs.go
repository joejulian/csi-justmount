package node

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// NodeGetInfo is a stub implementation to retrieve node information
func (n *Node) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	Logger(ctx).Info("NodeGetInfo start")
	resp := &csi.NodeGetInfoResponse{
		NodeId: n.nodeID,
	}
	Logger(ctx).Info("NodeGetInfo complete", zap.String("node_id", resp.NodeId))
	return resp, nil
}

// NodeGetVolumeStats reports usage; health is exposed by NodeGetVolumeHealth.
func (n *Node) NodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	if req.GetVolumeId() == "" || req.GetVolumePath() == "" {
		return nil, status.Error(codes.InvalidArgument, "volume_id and volume_path are required")
	}
	stat, err := statVolumePath(req.GetVolumePath())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get volume usage: %v", err)
	}
	return &csi.NodeGetVolumeStatsResponse{Usage: volumeUsageFromStatfs(stat)}, nil
}

// NodeGetVolumeHealth reports mount health using the CSI health RPC.
func (n *Node) NodeGetVolumeHealth(ctx context.Context, req *csi.NodeGetVolumeHealthRequest) (*csi.NodeGetVolumeHealthResponse, error) {
	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "volume_id is required")
	}
	health := &csi.VolumeHealth{VolumeId: req.GetVolumeId()}
	for _, path := range []string{req.GetVolumePublishPath(), req.GetStagingTargetPath()} {
		if path == "" {
			continue
		}
		if !filepath.IsAbs(path) {
			return nil, status.Error(codes.InvalidArgument, "volume paths must be absolute")
		}
		if _, err := statVolumePath(path); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil, status.Errorf(codes.NotFound, "volume path does not exist: %v", err)
			}
			if !isDisconnectedMountError(err) {
				return nil, status.Errorf(codes.Internal, "failed to check volume health: %v", err)
			}
			health.HealthStatuses = []*csi.VolumeHealth_VolumeHealthEntry{{
				Status:  csi.VolumeHealthErrorType_INACCESSIBLE,
				Reason:  "MountDisconnected",
				Message: fmt.Sprintf("volume path %q is disconnected: %v", path, err),
			}}
			break
		}
	}
	return &csi.NodeGetVolumeHealthResponse{VolumeHealth: health}, nil
}

func statVolumePath(path string) (syscall.Statfs_t, error) {
	var stat syscall.Statfs_t
	if err := probeMountPath(path); err != nil {
		return stat, err
	}
	err := syscall.Statfs(path, &stat)
	return stat, err
}

func volumeUsageFromStatfs(stat syscall.Statfs_t) []*csi.VolumeUsage {
	blockSize := int64(stat.Bsize)
	totalBytes := int64(stat.Blocks) * blockSize
	availableBytes := int64(stat.Bavail) * blockSize
	usedBytes := totalBytes - int64(stat.Bfree)*blockSize
	if usedBytes < 0 {
		usedBytes = 0
	}

	usage := []*csi.VolumeUsage{
		{
			Available: availableBytes,
			Total:     totalBytes,
			Used:      usedBytes,
			Unit:      csi.VolumeUsage_BYTES,
		},
	}
	if stat.Files > 0 {
		usedInodes := int64(stat.Files) - int64(stat.Ffree)
		if usedInodes < 0 {
			usedInodes = 0
		}
		usage = append(usage, &csi.VolumeUsage{
			Available: int64(stat.Ffree),
			Total:     int64(stat.Files),
			Used:      usedInodes,
			Unit:      csi.VolumeUsage_INODES,
		})
	}
	return usage
}
