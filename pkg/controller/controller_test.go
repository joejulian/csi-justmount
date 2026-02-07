package controller_test

import (
	"context"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/joejulian/csi-justmount/pkg/controller" // Update to your actual package path
)

func TestRun(t *testing.T) {
	t.Run("Run successfully starts server and creates socket", func(t *testing.T) {
		endpoint := "/tmp/test-csi.sock"
		_ = os.Remove(endpoint) // Clean up any existing file before the test

		ctrl := controller.NewController(endpoint, false)

		// Run the controller in a separate goroutine
		go func() {
			err := ctrl.Run()
			assert.NoError(t, err)
		}()

		// Wait briefly for the server to start
		time.Sleep(500 * time.Millisecond)

		// Verify that the socket file was created
		_, err := os.Stat(endpoint)
		assert.NoError(t, err, "Socket file should be created at the endpoint")

		// Verify that the gRPC server is listening on the socket
		conn, err := net.Dial("unix", endpoint)
		assert.NoError(t, err, "Should be able to connect to the gRPC server on the socket")
		assert.NotNil(t, conn, "Connection should not be nil")
		if conn != nil {
			conn.Close()
		}

		// Stop the server
		ctrl.Stop()
		_ = os.Remove(endpoint) // Clean up after test
	})

	t.Run("Run removes existing socket file", func(t *testing.T) {
		endpoint := "/tmp/test-csi.sock"

		// Create a dummy socket file before starting the server
		_, err := os.Create(endpoint)
		assert.NoError(t, err, "Should be able to create a dummy socket file")

		// Confirm the dummy socket file exists
		_, err = os.Stat(endpoint)
		assert.NoError(t, err, "Dummy socket file should exist before starting the server")

		ctrl := controller.NewController(endpoint, false)

		// Run the controller in a separate goroutine
		go func() {
			err := ctrl.Run()
			assert.NoError(t, err)
		}()

		// Wait briefly for the server to start
		time.Sleep(500 * time.Millisecond)

		// Verify that the socket file was recreated by the server
		_, err = os.Stat(endpoint)
		assert.NoError(t, err, "Socket file should be recreated by the server")

		// Verify that we can connect to the gRPC server on the new socket
		conn, err := net.Dial("unix", endpoint)
		assert.NoError(t, err, "Should be able to connect to the gRPC server on the new socket")
		assert.NotNil(t, conn, "Connection should not be nil")
		if conn != nil {
			conn.Close()
		}

		// Stop the server
		ctrl.Stop()
		_ = os.Remove(endpoint) // Clean up after test
	})

	t.Run("Run fails on invalid socket directory", func(t *testing.T) {
		// Set an endpoint in the non-writable /sys directory
		endpoint := "/sys/test-csi.sock"

		ctrl := controller.NewController(endpoint, false)
		err := ctrl.Run()

		// Expect an error because the directory is not writable
		assert.Error(t, err, "Expected error due to invalid socket directory")
		assert.True(t, strings.Contains(err.Error(), "permission denied") || strings.Contains(err.Error(), "operation not permitted"))
	})
}

// TestControllerPublishVolume tests the ControllerPublishVolume method
func TestControllerPublishVolume(t *testing.T) {
	controller := controller.NewController("/tmp/test-csi.sock", false)
	go func() {
		_ = controller.Run()
	}()
	defer controller.Stop()

	tests := []struct {
		name             string
		nodeID           string
		volumeID         string
		volumeCapability *csi.VolumeCapability
		expectErrorCode  codes.Code
	}{
		{
			name:     "Valid nodeID",
			nodeID:   "valid-node",
			volumeID: "valid-volume",
			volumeCapability: &csi.VolumeCapability{
				AccessType: &csi.VolumeCapability_Mount{
					Mount: &csi.VolumeCapability_MountVolume{},
				},
				AccessMode: &csi.VolumeCapability_AccessMode{
					Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
				},
			},
			expectErrorCode: codes.OK,
		},
		// Add other test cases as needed...
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := &csi.ControllerPublishVolumeRequest{
				NodeId:           tc.nodeID,
				VolumeId:         tc.volumeID,
				VolumeCapability: tc.volumeCapability,
			}
			resp, err := controller.ControllerPublishVolume(context.Background(), req)

			if err != nil {
				st, _ := status.FromError(err)
				assert.Equal(t, tc.expectErrorCode, st.Code())
			} else {
				assert.NotNil(t, resp)
			}
		})
	}
}
