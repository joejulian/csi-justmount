// sanity_test.go

package main_test

import (
	"context"
	"log"
	"net"
	"os"
	"testing"
	"time"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/kubernetes-csi/csi-test/v5/pkg/sanity"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc"

	"github.com/joejulian/csi-justmount/pkg/node"
	"github.com/joejulian/csi-justmount/pkg/node/nodefakes"
)

const (
	nodeEndpoint       = "/tmp/csi-justmount-node.sock"
	controllerEndpoint = "/tmp/csi-justmount-controller.sock"
)

func TestCSISanity(t *testing.T) {
	RegisterFailHandler(Fail)
	suiteConfig, reporterConfig := GinkgoConfiguration()
	// suiteConfig.FocusStrings = []string{"should fail when no node id is provided"}
	suiteConfig.SkipStrings = []string{
		// node-only driver; skip controller tests
		"\\[Controller Server\\]",

		// require CreateVolume
		"should fail when no volume capabilities are provided",
		"should return appropriate values",
		"should fail when the node does not exist",
		"should remove target path",
		"should fail when no volume capability is provided",
		"should be idempotent",
		"should work",

		// volumes can always exist
		"volume does not exist",
	}
	// suiteConfig.FailFast = true
	// reporterConfig.Verbose = true
	RunSpecs(t, "CSI Sanity Test Suite", suiteConfig, reporterConfig)
}

var (
	n          *node.Node
	ctrlServer *grpc.Server
	tempDir    string
)

// BeforeSuite to start the CSI driver
var _ = BeforeSuite(func() {
	// Start a minimal controller server so csi-test can query capabilities.
	ctrlServer = startControllerServer(controllerEndpoint)

	// Start the CSI node
	fake := newFakeMounter()
	n = node.NewNodeWithMounter("sanity-test-1", nodeEndpoint, fake)
	go func() {
		if err := n.Run(); err != nil {
			log.Fatalf("Failed to run node service: %v", err)
		}
	}()
	// Wait for the driver to initialize
	time.Sleep(2 * time.Second)
})

// AfterSuite to stop the CSI driver and clean up
var _ = AfterSuite(func() {
	// Stop the CSI driver
	if n != nil {
		n.Stop()
	}
	if ctrlServer != nil {
		ctrlServer.Stop()
	}
	// Clean up temporary directories
	if tempDir != "" {
		if err := os.RemoveAll(tempDir); err != nil {
			log.Printf("failed to remove tempDir: %v", err)
		}
	}
})

type testControllerServer struct {
	csi.UnimplementedControllerServer
	csi.UnimplementedIdentityServer
}

func (testControllerServer) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	return &csi.ControllerGetCapabilitiesResponse{
		Capabilities: []*csi.ControllerServiceCapability{
			{
				Type: &csi.ControllerServiceCapability_Rpc{
					Rpc: &csi.ControllerServiceCapability_RPC{
						Type: csi.ControllerServiceCapability_RPC_UNKNOWN,
					},
				},
			},
		},
	}, nil
}

func (testControllerServer) GetPluginCapabilities(ctx context.Context, req *csi.GetPluginCapabilitiesRequest) (*csi.GetPluginCapabilitiesResponse, error) {
	return &csi.GetPluginCapabilitiesResponse{
		Capabilities: []*csi.PluginCapability{},
	}, nil
}

func (testControllerServer) Probe(ctx context.Context, req *csi.ProbeRequest) (*csi.ProbeResponse, error) {
	return &csi.ProbeResponse{}, nil
}

func (testControllerServer) GetPluginInfo(ctx context.Context, req *csi.GetPluginInfoRequest) (*csi.GetPluginInfoResponse, error) {
	return &csi.GetPluginInfoResponse{
		Name:          "justmount.csi.driver",
		VendorVersion: "0.0.1",
	}, nil
}

func startControllerServer(endpoint string) *grpc.Server {
	if err := os.Remove(endpoint); err != nil && !os.IsNotExist(err) {
		log.Fatalf("Failed to remove controller socket: %v", err)
	}
	listener, err := net.Listen("unix", endpoint)
	if err != nil {
		log.Fatalf("Failed to listen on controller socket: %v", err)
	}
	server := grpc.NewServer()
	controller := testControllerServer{}
	csi.RegisterControllerServer(server, controller)
	csi.RegisterIdentityServer(server, controller)
	go func() {
		if err := server.Serve(listener); err != nil {
			log.Fatalf("Failed to run controller service: %v", err)
		}
	}()
	return server
}

func newFakeMounter() *nodefakes.FakeMounter {
	fake := &nodefakes.FakeMounter{}
	mounted := map[string]bool{}

	fake.MountStub = func(source, target, fstype string, flags uintptr, data string) error {
		mounted[target] = true
		return nil
	}
	fake.UnmountStub = func(target string, flags int) error {
		delete(mounted, target)
		return nil
	}
	fake.IsMountPointStub = func(path string) (bool, error) {
		return mounted[path], nil
	}
	return fake
}

// Create temporary directories before each test
var _ = BeforeEach(func() {
	var err error
	tempDir, err = os.MkdirTemp("", "csi-sanity")
	Expect(err).NotTo(HaveOccurred())
})

// Clean up temporary directories after each test
var _ = AfterEach(func() {
	if tempDir != "" {
		if err := os.RemoveAll(tempDir); err != nil {
			log.Printf("failed to remove tempDir: %v", err)
		}
		tempDir = ""
	}
})

func testConfig() *sanity.TestConfig {
	config := sanity.NewTestConfig()
	config.Address = nodeEndpoint
	config.ControllerAddress = controllerEndpoint
	return &config
}

// Register the sanity tests
var _ = sanity.GinkgoTest(testConfig())
