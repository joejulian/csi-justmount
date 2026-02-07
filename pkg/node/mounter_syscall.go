package node

import (
	"syscall"

	"github.com/joejulian/csi-justmount/pkg/util"
)

// SyscallMounter is the production mounter implementation.
type SyscallMounter struct{}

func (SyscallMounter) Mount(source, target, fstype string, flags uintptr, data string) error {
	return syscall.Mount(source, target, fstype, flags, data)
}

func (SyscallMounter) Unmount(target string, flags int) error {
	return syscall.Unmount(target, flags)
}

func (SyscallMounter) IsMountPoint(path string) (bool, error) {
	return util.IsMountPoint(path)
}
