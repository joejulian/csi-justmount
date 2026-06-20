package node

import (
	"context"
	"fmt"
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

// NodeGetVolumeStats reports volume usage and CSI volume health.
func (n *Node) NodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	Logger(ctx).Info("NodeGetVolumeStats start",
		zap.String("volume_id", req.GetVolumeId()),
		zap.String("volume_path", req.GetVolumePath()),
	)

	if req.GetVolumeId() == "" {
		Logger(ctx).Error("NodeGetVolumeStats invalid argument: volume_id is required")
		return nil, status.Error(codes.InvalidArgument, "volume_id is required")
	}
	if req.GetVolumePath() == "" {
		Logger(ctx).Error("NodeGetVolumeStats invalid argument: volume_path is required")
		return nil, status.Error(codes.InvalidArgument, "volume_path is required")
	}

	if err := probeMountPath(req.GetVolumePath()); err != nil {
		if !isDisconnectedMountError(err) {
			Logger(ctx).Error("NodeGetVolumeStats failed to probe volume path",
				zap.String("volume_id", req.GetVolumeId()),
				zap.String("volume_path", req.GetVolumePath()),
				zap.Error(err),
			)
			return nil, status.Errorf(codes.Internal, "failed to probe volume path: %v", err)
		}

		message := fmt.Sprintf("volume path is disconnected: %v", err)
		Logger(ctx).Warn("NodeGetVolumeStats reporting abnormal volume condition",
			zap.String("volume_id", req.GetVolumeId()),
			zap.String("volume_path", req.GetVolumePath()),
			zap.String("message", message),
			zap.Error(err),
		)
		return &csi.NodeGetVolumeStatsResponse{
			VolumeCondition: &csi.VolumeCondition{
				Abnormal: true,
				Message:  message,
			},
		}, nil
	}

	var stat syscall.Statfs_t
	if err := syscall.Statfs(req.GetVolumePath(), &stat); err != nil {
		if !isDisconnectedMountError(err) {
			Logger(ctx).Error("NodeGetVolumeStats failed to statfs volume path",
				zap.String("volume_id", req.GetVolumeId()),
				zap.String("volume_path", req.GetVolumePath()),
				zap.Error(err),
			)
			return nil, status.Errorf(codes.Internal, "failed to statfs volume path: %v", err)
		}

		message := fmt.Sprintf("volume path statfs reports disconnected mount: %v", err)
		Logger(ctx).Warn("NodeGetVolumeStats reporting abnormal volume condition",
			zap.String("volume_id", req.GetVolumeId()),
			zap.String("volume_path", req.GetVolumePath()),
			zap.String("message", message),
			zap.Error(err),
		)
		return &csi.NodeGetVolumeStatsResponse{
			VolumeCondition: &csi.VolumeCondition{
				Abnormal: true,
				Message:  message,
			},
		}, nil
	}

	usage := volumeUsageFromStatfs(stat)
	Logger(ctx).Info("NodeGetVolumeStats complete",
		zap.String("volume_id", req.GetVolumeId()),
		zap.String("volume_path", req.GetVolumePath()),
		zap.Int("usage_entries", len(usage)),
	)
	return &csi.NodeGetVolumeStatsResponse{
		Usage: usage,
		VolumeCondition: &csi.VolumeCondition{
			Abnormal: false,
			Message:  "volume path is usable",
		},
	}, nil
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
