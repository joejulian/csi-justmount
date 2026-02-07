package node

import (
	"context"
	"errors"
	"path/filepath"
	"syscall"
	"testing"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
)

type stubMounter struct {
	mountErr error
}

func (s stubMounter) Mount(source, target, fstype string, flags uintptr, data string) error {
	return s.mountErr
}

func (s stubMounter) Unmount(target string, flags int) error {
	return nil
}

func (s stubMounter) IsMountPoint(path string) (bool, error) {
	return false, nil
}

func TestNodeStageVolumeExecFallback(t *testing.T) {
	ctx := context.Background()
	stagePath := filepath.Join(t.TempDir(), "stage")

	var gotType, gotSource, gotTarget, gotOpts string
	origHelper := mountHelper
	mountHelper = func(fsType, source, target, opts string) error {
		gotType = fsType
		gotSource = source
		gotTarget = target
		gotOpts = opts
		return nil
	}
	t.Cleanup(func() { mountHelper = origHelper })

	n := NewNodeWithMounter("node-1", "endpoint", stubMounter{mountErr: syscall.ENODEV})
	req := &csi.NodeStageVolumeRequest{
		VolumeId:          "vol-1",
		StagingTargetPath: stagePath,
		VolumeCapability: &csi.VolumeCapability{
			AccessType: &csi.VolumeCapability_Mount{
				Mount: &csi.VolumeCapability_MountVolume{FsType: "glusterfs"},
			},
		},
		VolumeContext: map[string]string{
			"fileMode":     "0755",
			"source":       "gluster:media",
			"mountOptions": "rw,allow_other",
		},
	}

	if _, err := n.NodeStageVolume(ctx, req); err != nil {
		t.Fatalf("NodeStageVolume failed: %v", err)
	}

	if gotType != "glusterfs" || gotSource != "gluster:media" || gotTarget != stagePath || gotOpts != "rw,allow_other" {
		t.Fatalf("unexpected helper args: type=%q source=%q target=%q opts=%q", gotType, gotSource, gotTarget, gotOpts)
	}
}

func TestNodeStageVolumeNoExecFallbackOnOtherError(t *testing.T) {
	ctx := context.Background()
	stagePath := filepath.Join(t.TempDir(), "stage")

	origHelper := mountHelper
	called := false
	mountHelper = func(fsType, source, target, opts string) error {
		called = true
		return nil
	}
	t.Cleanup(func() { mountHelper = origHelper })

	n := NewNodeWithMounter("node-1", "endpoint", stubMounter{mountErr: errors.New("boom")})
	req := &csi.NodeStageVolumeRequest{
		VolumeId:          "vol-1",
		StagingTargetPath: stagePath,
		VolumeCapability: &csi.VolumeCapability{
			AccessType: &csi.VolumeCapability_Mount{
				Mount: &csi.VolumeCapability_MountVolume{FsType: "glusterfs"},
			},
		},
		VolumeContext: map[string]string{
			"fileMode": "0755",
			"source":   "gluster:media",
		},
	}

	if _, err := n.NodeStageVolume(ctx, req); err == nil {
		t.Fatalf("expected error, got nil")
	}
	if called {
		t.Fatalf("mount helper should not be called for non-ENODEV errors")
	}
}
