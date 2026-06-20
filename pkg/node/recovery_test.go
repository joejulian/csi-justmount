package node

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
)

type recordingMounter struct {
	mounted  map[string]bool
	mounts   []string
	unmounts []string
}

func (m *recordingMounter) Mount(source, target, fstype string, flags uintptr, data string) error {
	m.mounted[target] = true
	m.mounts = append(m.mounts, target)
	return nil
}

func (m *recordingMounter) Unmount(target string, flags int) error {
	delete(m.mounted, target)
	m.unmounts = append(m.unmounts, target)
	return nil
}

func (m *recordingMounter) IsMountPoint(path string) (bool, error) {
	return m.mounted[path], nil
}

func TestNodeStageVolumeReplacesDisconnectedStagingAndDependentBinds(t *testing.T) {
	stagingPath := t.TempDir()
	podTarget := filepath.Join(t.TempDir(), "pod-target")
	nestedTarget := filepath.Join(podTarget, "nested")

	mounter := &recordingMounter{
		mounted: map[string]bool{
			stagingPath:  true,
			podTarget:    true,
			nestedTarget: true,
		},
	}
	n := NewNodeWithMounter("node-id", "/tmp/test-csi.sock", mounter)

	origProbeMountPath := probeMountPath
	probeMountPath = func(path string) error {
		if path == stagingPath {
			return syscall.ENOTCONN
		}
		return nil
	}
	t.Cleanup(func() { probeMountPath = origProbeMountPath })

	origReadMountInfo := readMountInfo
	readMountInfo = func() ([]byte, error) {
		return []byte(
			"1 0 0:42 / " + stagingPath + " rw - fuse.glusterfs gluster:media rw\n" +
				"2 0 0:42 / " + podTarget + " rw - fuse.glusterfs gluster:media rw\n" +
				"3 0 0:42 / " + nestedTarget + " rw - fuse.glusterfs gluster:media rw\n",
		), nil
	}
	t.Cleanup(func() { readMountInfo = origReadMountInfo })

	req := &csi.NodeStageVolumeRequest{
		VolumeId:          "test-volume",
		StagingTargetPath: stagingPath,
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

	if _, err := n.NodeStageVolume(context.Background(), req); err != nil {
		t.Fatalf("NodeStageVolume() error = %v, want nil", err)
	}

	wantUnmounts := []string{nestedTarget, podTarget, stagingPath}
	if len(mounter.unmounts) != len(wantUnmounts) {
		t.Fatalf("NodeStageVolume() unmounts = %v, want %v", mounter.unmounts, wantUnmounts)
	}
	for i := range wantUnmounts {
		if mounter.unmounts[i] != wantUnmounts[i] {
			t.Errorf("NodeStageVolume() unmounts[%d] = %q, want %q", i, mounter.unmounts[i], wantUnmounts[i])
		}
	}
	if len(mounter.mounts) != 1 || mounter.mounts[0] != stagingPath {
		t.Fatalf("NodeStageVolume() mounts = %v, want [%s]", mounter.mounts, stagingPath)
	}
	if !mounter.mounted[stagingPath] {
		t.Fatalf("NodeStageVolume() left staging mounted = false, want true")
	}
}

func TestNodePublishVolumeReplacesDisconnectedTargetBindWithoutRemovingDirectory(t *testing.T) {
	stagingPath := t.TempDir()
	targetPath := t.TempDir()
	child := filepath.Join(targetPath, "child")
	if err := os.WriteFile(child, []byte("data"), 0644); err != nil {
		t.Fatalf("write child: %v", err)
	}

	mounter := &recordingMounter{
		mounted: map[string]bool{
			stagingPath: true,
			targetPath:  true,
		},
	}
	n := NewNodeWithMounter("node-id", "/tmp/test-csi.sock", mounter)

	origProbeMountPath := probeMountPath
	probeMountPath = func(path string) error {
		if path == targetPath {
			return syscall.ENOTCONN
		}
		return nil
	}
	t.Cleanup(func() { probeMountPath = origProbeMountPath })

	req := &csi.NodePublishVolumeRequest{
		VolumeId:          "test-volume",
		StagingTargetPath: stagingPath,
		TargetPath:        targetPath,
		VolumeCapability: &csi.VolumeCapability{
			AccessType: &csi.VolumeCapability_Mount{
				Mount: &csi.VolumeCapability_MountVolume{FsType: "glusterfs"},
			},
		},
	}

	if _, err := n.NodePublishVolume(context.Background(), req); err != nil {
		t.Fatalf("NodePublishVolume() error = %v, want nil", err)
	}
	if len(mounter.unmounts) != 1 || mounter.unmounts[0] != targetPath {
		t.Fatalf("NodePublishVolume() unmounts = %v, want [%s]", mounter.unmounts, targetPath)
	}
	if len(mounter.mounts) != 1 || mounter.mounts[0] != targetPath {
		t.Fatalf("NodePublishVolume() mounts = %v, want [%s]", mounter.mounts, targetPath)
	}
	if _, err := os.Stat(child); err != nil {
		t.Fatalf("NodePublishVolume() removed target child, stat error = %v", err)
	}
}
