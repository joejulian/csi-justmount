package util

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsMountPointFromMountInfo(t *testing.T) {
	tmp := t.TempDir()
	mountInfo := filepath.Join(tmp, "mountinfo")
	data := []byte(`36 25 0:32 / /var/lib/kubelet/plugins/justmount.csi.driver/vol/globalmount rw,relatime - fuse.glusterfs gluster:media rw
37 25 0:33 / /var/lib/kubelet/plugins/justmount.csi.driver/vol/globalmount\040with\040space rw,relatime - fuse.sshfs sshfs#host:/ rw
`)
	if err := os.WriteFile(mountInfo, data, 0644); err != nil {
		t.Fatalf("write mountinfo: %v", err)
	}

	orig := mountInfoPath
	mountInfoPath = mountInfo
	t.Cleanup(func() { mountInfoPath = orig })

	tests := []struct {
		path string
		want bool
	}{
		{"/var/lib/kubelet/plugins/justmount.csi.driver/vol/globalmount", true},
		{"/var/lib/kubelet/plugins/justmount.csi.driver/vol/globalmount with space", true},
		{"/var/lib/kubelet/plugins/justmount.csi.driver/vol/other", false},
	}

	for _, tc := range tests {
		got, err := isMountPointFromMountInfo(tc.path)
		if err != nil {
			t.Fatalf("isMountPointFromMountInfo error for %q: %v", tc.path, err)
		}
		if got != tc.want {
			t.Fatalf("isMountPointFromMountInfo(%q)=%v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestIsMountPointUsesMountInfo(t *testing.T) {
	tmp := t.TempDir()
	mountInfo := filepath.Join(tmp, "mountinfo")
	data := []byte(`36 25 0:32 / /mnt/test rw,relatime - fuse.sshfs sshfs#host:/ rw
`)
	if err := os.WriteFile(mountInfo, data, 0644); err != nil {
		t.Fatalf("write mountinfo: %v", err)
	}

	orig := mountInfoPath
	mountInfoPath = mountInfo
	t.Cleanup(func() { mountInfoPath = orig })

	got, err := IsMountPoint("/mnt/test")
	if err != nil {
		t.Fatalf("IsMountPoint error: %v", err)
	}
	if !got {
		t.Fatalf("IsMountPoint should return true when mountinfo contains the path")
	}
}
