package node_test

import (
	"context"
	"os"
	"syscall"
	"testing"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/joejulian/csi-justmount/pkg/node"
	"github.com/joejulian/csi-justmount/pkg/util"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestNodePublishVolume(t *testing.T) {
	n := node.NewNode("/tmp/test-csi.sock", "node-id")
	go n.Run()
	defer n.Stop()

	tests := []struct {
		name             string
		volumeID         string
		stagingPath      string
		targetPath       bool
		volumeCapability *csi.VolumeCapability
		volumeContext    map[string]string
		expectErrorCode  codes.Code
	}{
		{
			name:       "Valid publish request with bind mount",
			volumeID:   "test-volume",
			targetPath: true,
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
			volumeContext:   map[string]string{"fileMode": "0755"},
			expectErrorCode: codes.OK,
		},
		{
			name:             "Missing volume ID",
			volumeID:         "",
			stagingPath:      "",
			targetPath:       true,
			volumeCapability: nil,
			volumeContext:    map[string]string{"fileMode": "0755"},
			expectErrorCode:  codes.InvalidArgument,
		},
		{
			name:     "Missing target path",
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
			volumeContext:   map[string]string{"fileMode": "0755"},
			targetPath:      false,
			expectErrorCode: codes.InvalidArgument,
		},
		{
			name:             "Missing volume capability",
			volumeID:         "test-volume",
			targetPath:       true,
			volumeCapability: nil,
			volumeContext:    map[string]string{"fileMode": "0755"},
			expectErrorCode:  codes.InvalidArgument,
		},
		{
			name:       "Missing fileMode",
			volumeID:   "test-volume",
			targetPath: true,
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
			expectErrorCode: codes.FailedPrecondition,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create temporary staging and publish directories
			stagingPath, err := os.MkdirTemp("", "csi-staging-")
			assert.NoError(t, err, "Failed to create temp staging directory")
			defer os.RemoveAll(stagingPath)

			targetPath, err := os.MkdirTemp("", "csi-publish-")
			assert.NoError(t, err, "Failed to create temp publish directory")
			defer os.RemoveAll(targetPath)

			if !tc.targetPath {
				targetPath = ""
			}

			// If expecting success, create and mount tmpfs to staging path
			if tc.expectErrorCode == codes.OK {
				err := os.MkdirAll(stagingPath, 0755)
				assert.NoError(t, err, "Failed to create staging directory")

				stageReq := &csi.NodeStageVolumeRequest{
					VolumeId:          tc.volumeID,
					StagingTargetPath: stagingPath,
					VolumeCapability:  tc.volumeCapability,
					VolumeContext:     tc.volumeContext,
				}
				_, err = n.NodeStageVolume(context.Background(), stageReq)
				assert.NoError(t, err, "Failed to stage volume")
				tc.stagingPath = stagingPath
			} else if tc.stagingPath == "" {
				tc.stagingPath = stagingPath
			}

			// Ensure target path exists if expecting success
			if tc.expectErrorCode == codes.OK {
				err = os.MkdirAll(targetPath, 0755)
				assert.NoError(t, err, "Failed to create target directory")
			}

			// Create the NodePublishVolumeRequest
			req := &csi.NodePublishVolumeRequest{
				VolumeId:          tc.volumeID,
				StagingTargetPath: tc.stagingPath,
				TargetPath:        targetPath,
				VolumeCapability:  tc.volumeCapability,
				VolumeContext:     tc.volumeContext,
			}

			// Run NodePublishVolume
			resp, err := n.NodePublishVolume(context.Background(), req)
			if tc.expectErrorCode == codes.OK {
				assert.NoError(t, err)

				// Verify if the target path is bind-mounted
				isMounted, err := util.IsMountPoint(targetPath)
				assert.NoError(t, err)
				assert.True(t, isMounted, "The target path should be a bind mount point")

				// Unmount after the test
				err = syscall.Unmount(targetPath, 0)
				assert.NoError(t, err, "Failed to unmount target path after test")
			} else {
				st, _ := status.FromError(err)
				assert.Equal(t, tc.expectErrorCode, st.Code())
			}

			if err == nil {
				assert.NotNil(t, resp)
			}

			// Final cleanup: Unmount if mounted
			syscall.Unmount(targetPath, 0)
			syscall.Unmount(stagingPath, 0)
		})
	}
}
