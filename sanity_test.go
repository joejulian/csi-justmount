// sanity_test.go

package main_test

import (
	"log"
	"os"
	"testing"
	"time"

	"github.com/kubernetes-csi/csi-test/v5/pkg/sanity"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/joejulian/csi-justmount/pkg/node"
)

const (
	nodeEndpoint       = "/tmp/csi-justmount-node.sock"
)

func TestCSISanity(t *testing.T) {
	RegisterFailHandler(Fail)
	suiteConfig, reporterConfig := GinkgoConfiguration()
	// suiteConfig.FocusStrings = []string{"should fail when no node id is provided"}
	suiteConfig.SkipStrings = []string{
		// node-only driver; skip all controller tests
		"[Controller Server]",

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
	n       *node.Node
	tempDir string
)

// BeforeSuite to start the CSI driver
var _ = BeforeSuite(func() {
	// Start the CSI node
	n = node.NewNode("sanity-test-1", nodeEndpoint)
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
	n.Stop()
	// Clean up temporary directories
	if tempDir != "" {
		os.RemoveAll(tempDir)
	}
})

// Create temporary directories before each test
var _ = BeforeEach(func() {
	var err error
	tempDir, err = os.MkdirTemp("", "csi-sanity")
	Expect(err).NotTo(HaveOccurred())
})

// Clean up temporary directories after each test
var _ = AfterEach(func() {
	if tempDir != "" {
		os.RemoveAll(tempDir)
		tempDir = ""
	}
})

func testConfig() *sanity.TestConfig {
	config := sanity.NewTestConfig()
	config.Address = nodeEndpoint
	return &config
}

// Register the sanity tests
var _ = sanity.GinkgoTest(testConfig())
