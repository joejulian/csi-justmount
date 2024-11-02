package util

import (
	"path/filepath"
	"syscall"
)

// Helper function to check if a path is a mount point
func IsMountPoint(path string) (bool, error) {
	var stat syscall.Stat_t
	if err := syscall.Stat(path, &stat); err != nil {
		return false, err
	}

	// Get the parent directory of the path
	parent := filepath.Dir(path)
	var parentStat syscall.Stat_t
	if err := syscall.Stat(parent, &parentStat); err != nil {
		return false, err
	}

	// Compare device IDs; if they differ, the path is a mount point
	return stat.Dev != parentStat.Dev, nil
}
