package node

import (
	"context"

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

// NodeGetVolumeStats is a stub implementation to get volume statistics
func (n *Node) NodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	// TODO: Implement volume stats reporting
	Logger(ctx).Warn("NodeGetVolumeStats unimplemented",
		zap.String("volume_id", req.GetVolumeId()),
		zap.String("volume_path", req.GetVolumePath()),
	)
	return nil, status.Error(codes.Unimplemented, "NodeGetVolumeStats not implemented")
}
