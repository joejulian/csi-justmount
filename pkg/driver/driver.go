package driver

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/url"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/wrapperspb"
	mount "k8s.io/mount-utils"
)

type JustMountDriver struct {
	Endpoint string
	mounter  mount.Interface
}

func NewJustMountDriver(endpoint string) *JustMountDriver {
	mounter := mount.NewWithoutSystemd("/bin/mount")
	return &JustMountDriver{Endpoint: endpoint, mounter: mounter}
}

func NewFakeJustMountDriver(endpoint string) *JustMountDriver {
	var mountPoints []mount.MountPoint
	mounter := mount.NewFakeMounter(mountPoints)
	return &JustMountDriver{Endpoint: endpoint, mounter: mounter}
}

func (d *JustMountDriver) Run() error {
	network, address, err := parseURI(d.Endpoint)
	if err != nil {
		return fmt.Errorf("failed to parse endpoint: %q: %v", d.Endpoint, err)
	}

	listener, err := net.Listen(network, address)
	if err != nil {
		return fmt.Errorf("failed to listen: %v", err)
	}

	server := grpc.NewServer()
	csi.RegisterNodeServer(server, d)
	csi.RegisterIdentityServer(server, d)
	log.Printf("Starting JustMount driver at %s", d.Endpoint)
	return server.Serve(listener)
}

func (d *JustMountDriver) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	source := req.GetVolumeContext()["source"]
	target := req.GetTargetPath()
	fsType := req.GetVolumeContext()["fsType"]
	options := strings.Split(req.GetVolumeContext()["options"], ",")
	sensitiveOptions := strings.Split(req.GetVolumeContext()["sensitiveOptions"], ",")

	if err := d.mounter.MountSensitive(source, target, fsType, options, sensitiveOptions); err != nil {
		return nil, err
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func (d *JustMountDriver) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	target := req.GetTargetPath()
	err := d.mounter.Unmount(target)
	if err != nil {
		return nil, err
	}

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

// IdentityServer methods
func (d *JustMountDriver) GetPluginInfo(ctx context.Context, req *csi.GetPluginInfoRequest) (*csi.GetPluginInfoResponse, error) {
	return &csi.GetPluginInfoResponse{
		Name:          "justmount.csi.k8s.io",
		VendorVersion: "1.0.0",
	}, nil
}

func (d *JustMountDriver) GetPluginCapabilities(ctx context.Context, req *csi.GetPluginCapabilitiesRequest) (*csi.GetPluginCapabilitiesResponse, error) {
	return &csi.GetPluginCapabilitiesResponse{
		Capabilities: []*csi.PluginCapability{
			{
				Type: &csi.PluginCapability_Service_{
					Service: &csi.PluginCapability_Service{
						Type: csi.PluginCapability_Service_CONTROLLER_SERVICE,
					},
				},
			},
		},
	}, nil
}

func (d *JustMountDriver) Probe(ctx context.Context, req *csi.ProbeRequest) (*csi.ProbeResponse, error) {
	return &csi.ProbeResponse{
		Ready: wrapperspb.Bool(true),
	}, nil
}

// NodeServer methods

func (d *JustMountDriver) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	return &csi.NodeGetInfoResponse{
		NodeId: "justmount-node",
	}, nil
}

func (d *JustMountDriver) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
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

func (d *JustMountDriver) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	// This implementation does not require staging volumes.
	return &csi.NodeStageVolumeResponse{}, nil
}

func (d *JustMountDriver) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	// This implementation does not require unstaging volumes.
	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (d *JustMountDriver) NodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	return nil, fmt.Errorf("NodeGetVolumeStats not implemented")
}

func (d *JustMountDriver) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	return nil, fmt.Errorf("NodeExpandVolume not implemented")
}

func parseURI(uri string) (network, address string, err error) {
	parsedURL, err := url.Parse(uri)
	if err != nil {
		return "", "", fmt.Errorf("invalid URI: %w", err)
	}

	network = parsedURL.Scheme
	switch network {
	case "unix":
		address = parsedURL.Path
	case "tcp":
		address = parsedURL.Host
	default:
		return "", "", fmt.Errorf("unsupported network scheme: %s", network)
	}

	return network, address, nil
}
