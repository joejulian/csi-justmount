package node_test

import (
	"context"
	"os"
	"testing"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/joejulian/csi-justmount/pkg/node"
	"github.com/joejulian/csi-justmount/pkg/node/nodefakes"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestNodePublishVolume(t *testing.T) {
	fake := newFakeMounter()
	n := node.NewNodeWithMounter("node-id", "/tmp/test-csi.sock", fake)

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
			volumeContext:   map[string]string{"fileMode": "0755", "source": "tmpfs"},
			expectErrorCode: codes.OK,
		},
		{
			name:             "Missing volume ID",
			volumeID:         "",
			stagingPath:      "",
			targetPath:       true,
			volumeCapability: nil,
			volumeContext:    map[string]string{"fileMode": "0755", "source": "tmpfs"},
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
			volumeContext:   map[string]string{"fileMode": "0755", "source": "tmpfs"},
			targetPath:      false,
			expectErrorCode: codes.InvalidArgument,
		},
		{
			name:             "Missing volume capability",
			volumeID:         "test-volume",
			targetPath:       true,
			volumeCapability: nil,
			volumeContext:    map[string]string{"fileMode": "0755", "source": "tmpfs"},
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
			t.Cleanup(func() {
				if err := os.RemoveAll(stagingPath); err != nil {
					t.Fatalf("cleanup staging path: %v", err)
				}
			})

			targetPath, err := os.MkdirTemp("", "csi-publish-")
			assert.NoError(t, err, "Failed to create temp publish directory")
			t.Cleanup(func() {
				if err := os.RemoveAll(targetPath); err != nil {
					t.Fatalf("cleanup target path: %v", err)
				}
			})

			if !tc.targetPath {
				targetPath = ""
			}

			// If expecting success, record staging as mounted
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
				isMounted, err := fake.IsMountPoint(targetPath)
				assert.NoError(t, err)
				assert.True(t, isMounted, "The target path should be a bind mount point")
			} else {
				st, _ := status.FromError(err)
				assert.Equal(t, tc.expectErrorCode, st.Code())
			}

			if err == nil {
				assert.NotNil(t, resp)
			}

			// Final cleanup: Unmount if mounted
			_ = fake.Unmount(targetPath, 0)
			_ = fake.Unmount(stagingPath, 0)
		})
	}
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
