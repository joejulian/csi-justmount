package node

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type recordingMounter struct {
	mounted  map[string]bool
	mounts   []string
	unmounts []string
}

type recordingPVCReporter struct {
	started   []string
	completed []string
}

func (r *recordingPVCReporter) RepairStarted(ctx context.Context, req *csi.NodePublishVolumeRequest, reason, message string) error {
	r.started = append(r.started, reason)
	return nil
}

func (r *recordingPVCReporter) RepairCompleted(ctx context.Context, req *csi.NodePublishVolumeRequest, reason, message string) error {
	r.completed = append(r.completed, reason)
	return nil
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
	reporter := &recordingPVCReporter{}
	n.pvcReporter = reporter

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
	if len(reporter.started) != 1 || reporter.started[0] != "JustmountBindMountDisconnected" {
		t.Fatalf("NodePublishVolume() repair start reports = %v, want [JustmountBindMountDisconnected]", reporter.started)
	}
	if len(reporter.completed) != 1 || reporter.completed[0] != "JustmountBindMountReplaced" {
		t.Fatalf("NodePublishVolume() repair completion reports = %v, want [JustmountBindMountReplaced]", reporter.completed)
	}
}

func TestNodePublishVolumeUnstagesDisconnectedStagingAndDependentBinds(t *testing.T) {
	stagingPath := t.TempDir()
	podTarget := filepath.Join(t.TempDir(), "pod-target")
	nestedTarget := filepath.Join(podTarget, "nested")
	publishTarget := t.TempDir()

	mounter := &recordingMounter{
		mounted: map[string]bool{
			stagingPath:  true,
			podTarget:    true,
			nestedTarget: true,
		},
	}
	n := NewNodeWithMounter("node-id", "/tmp/test-csi.sock", mounter)
	reporter := &recordingPVCReporter{}
	n.pvcReporter = reporter

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

	req := &csi.NodePublishVolumeRequest{
		VolumeId:          "test-volume",
		StagingTargetPath: stagingPath,
		TargetPath:        publishTarget,
		VolumeCapability: &csi.VolumeCapability{
			AccessType: &csi.VolumeCapability_Mount{
				Mount: &csi.VolumeCapability_MountVolume{FsType: "glusterfs"},
			},
		},
	}

	if _, err := n.NodePublishVolume(context.Background(), req); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("NodePublishVolume() error code = %v, want %v; error = %v", status.Code(err), codes.FailedPrecondition, err)
	}

	wantUnmounts := []string{nestedTarget, podTarget, stagingPath}
	if len(mounter.unmounts) != len(wantUnmounts) {
		t.Fatalf("NodePublishVolume() unmounts = %v, want %v", mounter.unmounts, wantUnmounts)
	}
	for i := range wantUnmounts {
		if mounter.unmounts[i] != wantUnmounts[i] {
			t.Errorf("NodePublishVolume() unmounts[%d] = %q, want %q", i, mounter.unmounts[i], wantUnmounts[i])
		}
	}
	if len(mounter.mounts) != 0 {
		t.Fatalf("NodePublishVolume() mounts = %v, want none", mounter.mounts)
	}
	if len(reporter.started) != 1 || reporter.started[0] != "JustmountStagingMountDisconnected" {
		t.Fatalf("NodePublishVolume() repair start reports = %v, want [JustmountStagingMountDisconnected]", reporter.started)
	}
	if len(reporter.completed) != 1 || reporter.completed[0] != "JustmountStagingMountUnstaged" {
		t.Fatalf("NodePublishVolume() repair completion reports = %v, want [JustmountStagingMountUnstaged]", reporter.completed)
	}
}

func TestNodeGetVolumeStatsReportsDisconnectedVolumeCondition(t *testing.T) {
	volumePath := t.TempDir()
	n := NewNodeWithMounter("node-id", "/tmp/test-csi.sock", &recordingMounter{mounted: map[string]bool{}})

	origProbeMountPath := probeMountPath
	probeMountPath = func(path string) error {
		if path == volumePath {
			return syscall.ENOTCONN
		}
		return nil
	}
	t.Cleanup(func() { probeMountPath = origProbeMountPath })

	resp, err := n.NodeGetVolumeStats(context.Background(), &csi.NodeGetVolumeStatsRequest{
		VolumeId:   "test-volume",
		VolumePath: volumePath,
	})
	if err != nil {
		t.Fatalf("NodeGetVolumeStats() error = %v, want nil", err)
	}
	if resp.GetVolumeCondition() == nil {
		t.Fatalf("NodeGetVolumeStats() volume condition = nil, want abnormal condition")
	}
	if !resp.GetVolumeCondition().GetAbnormal() {
		t.Fatalf("NodeGetVolumeStats() abnormal = false, want true")
	}
	if resp.GetVolumeCondition().GetMessage() == "" {
		t.Fatalf("NodeGetVolumeStats() message = empty, want diagnostic message")
	}
}

func TestNodeGetVolumeStatsReportsHealthyVolumeCondition(t *testing.T) {
	volumePath := t.TempDir()
	n := NewNodeWithMounter("node-id", "/tmp/test-csi.sock", &recordingMounter{mounted: map[string]bool{}})

	resp, err := n.NodeGetVolumeStats(context.Background(), &csi.NodeGetVolumeStatsRequest{
		VolumeId:   "test-volume",
		VolumePath: volumePath,
	})
	if err != nil {
		t.Fatalf("NodeGetVolumeStats() error = %v, want nil", err)
	}
	if resp.GetVolumeCondition() == nil {
		t.Fatalf("NodeGetVolumeStats() volume condition = nil, want healthy condition")
	}
	if resp.GetVolumeCondition().GetAbnormal() {
		t.Fatalf("NodeGetVolumeStats() abnormal = true, want false")
	}
	if len(resp.GetUsage()) == 0 {
		t.Fatalf("NodeGetVolumeStats() usage entries = 0, want at least one")
	}
}
