package controller

import (
	"context"
	"log"
	"net"
	"os"
	"path/filepath"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
)

type Controller struct {
	name     string
	version  string
	server   *grpc.Server
	endpoint string
	test     bool

	csi.UnimplementedIdentityServer
	csi.UnimplementedControllerServer
}

func NewController(endpoint string, test bool) *Controller {
	return &Controller{
		name:     "justmount.csi.driver",
		version:  "0.0.1",
		endpoint: endpoint,
		test:     test,
	}
}

func (c *Controller) Run() error {
	// Remove the socket file if it already exists
	if err := os.Remove(c.endpoint); err != nil && !os.IsNotExist(err) {
		return err
	}

	// Create the directory for the socket if it doesn't exist
	dir := filepath.Dir(c.endpoint)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Start the gRPC server
	listener, err := net.Listen("unix", c.endpoint)
	if err != nil {
		return err
	}

	c.server = grpc.NewServer()

	// Register the Identity and Controller services
	csi.RegisterIdentityServer(c.server, c)
	csi.RegisterControllerServer(c.server, c)

	// Register reflection service on gRPC server.
	reflection.Register(c.server)

	// Start the server
	log.Printf("Starting gRPC server on %s", c.endpoint)
	if err := c.server.Serve(listener); err != nil {
		return err
	}
	return nil
}

func (c *Controller) Stop() {
	if c.server != nil {
		c.server.Stop()
	}
}

// Implement the ControllerServer interface

func (c *Controller) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	return &csi.ControllerGetCapabilitiesResponse{
		Capabilities: []*csi.ControllerServiceCapability{},
	}, nil
}

// Implement the IdentityServer interface

func (c *Controller) GetPluginInfo(ctx context.Context, req *csi.GetPluginInfoRequest) (*csi.GetPluginInfoResponse, error) {
	return &csi.GetPluginInfoResponse{
		Name:          c.name,
		VendorVersion: c.version,
	}, nil
}

func (c *Controller) GetPluginCapabilities(ctx context.Context, req *csi.GetPluginCapabilitiesRequest) (*csi.GetPluginCapabilitiesResponse, error) {
	return &csi.GetPluginCapabilitiesResponse{
		Capabilities: []*csi.PluginCapability{},
	}, nil
}

func (c *Controller) Probe(ctx context.Context, req *csi.ProbeRequest) (*csi.ProbeResponse, error) {
	// Indicate the plugin is ready
	return &csi.ProbeResponse{}, nil
}

func (c *Controller) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	// Check if the volume ID is provided
	if len(req.GetVolumeId()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID not provided")
	}

	// Check if volume capabilities are provided
	if len(req.GetVolumeCapabilities()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume capabilities not provided")
	}

	// For simplicity, assume all requested capabilities are supported
	// In a real implementation, you would check if the volume supports the requested capabilities

	// Return confirmed if capabilities are supported
	return &csi.ValidateVolumeCapabilitiesResponse{
		Confirmed: &csi.ValidateVolumeCapabilitiesResponse_Confirmed{
			VolumeCapabilities: req.GetVolumeCapabilities(),
		},
	}, nil
}
