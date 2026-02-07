package node_test

import (
	"context"
	"os"
	"strconv"
	"syscall"
	"testing"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/joejulian/csi-justmount/pkg/node"
	"github.com/joejulian/csi-justmount/pkg/util"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestNodeStageVolume(t *testing.T) {
	n := node.NewNode("/tmp/test-csi.sock", "node-id")
	go func() {
		_ = n.Run()
	}()
	defer n.Stop()
	// Create a temporary staging directory
	stagingPath, err := os.MkdirTemp("", "csi-staging-")
	assert.NoError(t, err, "Failed to create temp staging directory")

	defer os.RemoveAll(stagingPath)

	tests := []struct {
		name             string
		volumeID         string
		volumeCapability *csi.VolumeCapability
		fsType           string
		fileMode         string
		source           string
		mountOptions     string
		expectErrorCode  codes.Code
		stagingPath      string
	}{
		{
			name:     "Valid request with tmpfs filesystem and fileMode",
			volumeID: "test-volume",
			volumeCapability: &csi.VolumeCapability{
				AccessType: &csi.VolumeCapability_Mount{
					Mount: &csi.VolumeCapability_MountVolume{
						FsType: "tmpfs",
					},
				},
				AccessMode: &csi.VolumeCapability_AccessMode{
					Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
				},
			},
			fsType:          "tmpfs",
			fileMode:        "0755",
			source:          "tmpfs",
			mountOptions:    "rw",
			expectErrorCode: codes.OK,
			stagingPath:     stagingPath,
		},
		{
			name:     "Missing fileMode in VolumeContext",
			volumeID: "test-volume",
			volumeCapability: &csi.VolumeCapability{
				AccessType: &csi.VolumeCapability_Mount{
					Mount: &csi.VolumeCapability_MountVolume{
						FsType: "tmpfs",
					},
				},
				AccessMode: &csi.VolumeCapability_AccessMode{
					Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
				},
			},
			fsType:          "tmpfs",
			fileMode:        "", // Missing fileMode should trigger an error
			source:          "tmpfs",
			expectErrorCode: codes.InvalidArgument,
			stagingPath:     stagingPath,
		},
		{
			name:     "Invalid fileMode in VolumeContext",
			volumeID: "test-volume",
			volumeCapability: &csi.VolumeCapability{
				AccessType: &csi.VolumeCapability_Mount{
					Mount: &csi.VolumeCapability_MountVolume{
						FsType: "tmpfs",
					},
				},
				AccessMode: &csi.VolumeCapability_AccessMode{
					Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
				},
			},
			fsType:          "tmpfs",
			fileMode:        "invalid", // Invalid fileMode format should trigger an error
			source:          "tmpfs",
			expectErrorCode: codes.InvalidArgument,
			stagingPath:     stagingPath,
		},
		{
			name:     "Missing staging path",
			volumeID: "test-volume",
			volumeCapability: &csi.VolumeCapability{
				AccessType: &csi.VolumeCapability_Mount{
					Mount: &csi.VolumeCapability_MountVolume{
						FsType: "tmpfs",
					},
				},
				AccessMode: &csi.VolumeCapability_AccessMode{
					Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
				},
			},
			fsType:          "tmpfs",
			fileMode:        "0755",
			source:          "tmpfs",
			expectErrorCode: codes.InvalidArgument,
			stagingPath:     "",
		},
		{
			name:     "Missing source in VolumeContext",
			volumeID: "test-volume",
			volumeCapability: &csi.VolumeCapability{
				AccessType: &csi.VolumeCapability_Mount{
					Mount: &csi.VolumeCapability_MountVolume{
						FsType: "tmpfs",
					},
				},
				AccessMode: &csi.VolumeCapability_AccessMode{
					Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
				},
			},
			fsType:          "tmpfs",
			fileMode:        "0755",
			source:          "",
			expectErrorCode: codes.InvalidArgument,
			stagingPath:     stagingPath,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Prepare the NodeStageVolumeRequest with VolumeContext containing fileMode
			req := &csi.NodeStageVolumeRequest{
				VolumeId:          tc.volumeID,
				StagingTargetPath: tc.stagingPath,
				VolumeCapability:  tc.volumeCapability,
				VolumeContext: map[string]string{
					"fileMode": tc.fileMode,
					"source":   tc.source,
					"fsType":   tc.fsType,
					"mountOptions": tc.mountOptions,
				},
			}

			// Run NodeStageVolume
			resp, err := n.NodeStageVolume(context.Background(), req)
			if tc.expectErrorCode == codes.OK {
				assert.NoError(t, err)

				// Verify the mount point and permissions
				isMounted, err := util.IsMountPoint(stagingPath)
				assert.NoError(t, err)
				assert.True(t, isMounted, "the volume path should be a mount point")

				// Check the file mode if fileMode is valid
				if tc.fileMode != "" {
					mode, _ := strconv.ParseUint(tc.fileMode, 8, 32)
					info, err := os.Stat(stagingPath)
					assert.NoError(t, err)
					assert.Equal(t, os.FileMode(mode), info.Mode().Perm(), "file mode should match specified fileMode")
				}

				// Unmount after the test
				err = syscall.Unmount(stagingPath, 0)
				assert.NoError(t, err, "Failed to unmount volume path after test")
			} else {
				st, _ := status.FromError(err)
				assert.Equal(t, tc.expectErrorCode, st.Code())
			}

			if err == nil {
				assert.NotNil(t, resp)
			}
		})
	}
}

func TestNodeUnstageVolume(t *testing.T) {
	n := node.NewNode("/tmp/test-csi.sock", "node-id")
	go func() {
		_ = n.Run()
	}()
	defer n.Stop()

	// Create a temporary staging directory
	stagingPath, err := os.MkdirTemp("", "csi-staging-")
	assert.NoError(t, err, "Failed to create temp staging directory")

	defer os.RemoveAll(stagingPath)

	tests := []struct {
		name              string
		volumeID          string
		stagingTargetPath string
		expectErrorCode   codes.Code
	}{
		{
			name:              "Valid unstage request",
			volumeID:          "test-volume",
			stagingTargetPath: stagingPath, // Temp directory will be created dynamically
			expectErrorCode:   codes.OK,
		},
		{
			name:              "Missing volumeID",
			volumeID:          "",
			stagingTargetPath: stagingPath, // Temp directory will be created dynamically
			expectErrorCode:   codes.InvalidArgument,
		},
		{
			name:              "Missing staging target path",
			volumeID:          "test-volume",
			stagingTargetPath: "", // Empty to simulate missing path
			expectErrorCode:   codes.InvalidArgument,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {

			// Simulate mounting for the valid test case
			if tc.expectErrorCode == codes.OK {
				err = syscall.Mount("tmpfs", stagingPath, "tmpfs", 0, "")
				assert.NoError(t, err, "Failed to mount tmpfs for testing unstage")
				tc.stagingTargetPath = stagingPath
			}

			req := &csi.NodeUnstageVolumeRequest{
				VolumeId:          tc.volumeID,
				StagingTargetPath: tc.stagingTargetPath,
			}

			resp, err := n.NodeUnstageVolume(context.Background(), req)
			if tc.expectErrorCode == codes.OK {
				assert.NoError(t, err)

				// Check if the mount has been removed
				isMounted, err := util.IsMountPoint(stagingPath)
				assert.NoError(t, err)
				assert.False(t, isMounted, "The volume mount path should be unmounted")
			} else {
				st, _ := status.FromError(err)
				assert.Equal(t, tc.expectErrorCode, st.Code())
			}

			if err == nil {
				assert.NotNil(t, resp)
			}

			// Final cleanup: Attempt to unmount if still mounted
			_ = syscall.Unmount(stagingPath, 0)
		})
	}
}
