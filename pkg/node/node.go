package node

import (
	"context"
	"log"
	"net"
	"os"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

type Node struct {
	// Fields for any required configuration can be added here
	nodeID   string
	endpoint string
	server   *grpc.Server
	mounter  Mounter

	csi.UnimplementedNodeServer
	csi.UnimplementedIdentityServer
}

// NewNode creates a new Node service
func NewNode(nodeID, endpoint string) *Node {
	return &Node{
		nodeID:   nodeID,
		endpoint: endpoint,
		mounter:  SyscallMounter{},
	}
}

// NewNodeWithMounter creates a new Node service with a custom mounter (for tests).
func NewNodeWithMounter(nodeID, endpoint string, mounter Mounter) *Node {
	return &Node{
		nodeID:   nodeID,
		endpoint: endpoint,
		mounter:  mounter,
	}
}

func (n *Node) Run() error {
	// Remove the socket file if it already exists
	if err := os.Remove(n.endpoint); err != nil && !os.IsNotExist(err) {
		return err
	}

	// Create the gRPC server and listen on the specified endpoint
	listener, err := net.Listen("unix", n.endpoint)
	if err != nil {
		return err
	}

	n.server = grpc.NewServer()

	// Register the Node service
	csi.RegisterNodeServer(n.server, n)
	csi.RegisterIdentityServer(n.server, n)

	// Register reflection service for debugging
	reflection.Register(n.server)

	log.Printf("Starting Node gRPC server on %s", n.endpoint)
	if err := n.server.Serve(listener); err != nil {
		return err
	}
	return nil
}

func (n *Node) Stop() {
	if n.server != nil {
		n.server.Stop()
	}
}

// NodeGetCapabilities is a stub implementation to get node capabilities
func (n *Node) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	// TODO: Implement capability reporting logic
	return &csi.NodeGetCapabilitiesResponse{
		Capabilities: []*csi.NodeServiceCapability{
			{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
					},
				},
			},
		},
	}, nil
}

func (n *Node) GetPluginCapabilities(ctx context.Context, req *csi.GetPluginCapabilitiesRequest) (*csi.GetPluginCapabilitiesResponse, error) {
	return &csi.GetPluginCapabilitiesResponse{
		Capabilities: []*csi.PluginCapability{}}, nil
}

func (n *Node) Probe(ctx context.Context, req *csi.ProbeRequest) (*csi.ProbeResponse, error) {
	// Indicate the plugin is ready
	return &csi.ProbeResponse{}, nil
}

func (n *Node) GetPluginInfo(ctx context.Context, req *csi.GetPluginInfoRequest) (*csi.GetPluginInfoResponse, error) {
	return &csi.GetPluginInfoResponse{
		Name:          "justmount.csi.driver", // Unique name for your CSI driver
		VendorVersion: "0.0.1",                // Driver version
	}, nil
}
