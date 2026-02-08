package util

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// Helper function to check if a path is a mount point
func IsMountPoint(path string) (bool, error) {
	cleaned := filepath.Clean(path)
	if mounted, err := isMountPointFromMountInfo(cleaned); err == nil {
		return mounted, nil
	}

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

var mountInfoPath = "/proc/self/mountinfo"

func isMountPointFromMountInfo(path string) (bool, error) {
	data, err := os.ReadFile(mountInfoPath)
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.SplitN(line, " - ", 2)
		if len(parts) < 1 {
			continue
		}
		fields := strings.Fields(parts[0])
		if len(fields) < 5 {
			continue
		}
		mountPoint := unescapeMountPath(fields[4])
		if filepath.Clean(mountPoint) == path {
			return true, nil
		}
	}
	return false, nil
}

func unescapeMountPath(path string) string {
	// mountinfo uses octal escapes for special chars.
	replacer := strings.NewReplacer(
		"\\040", " ",
		"\\011", "\t",
		"\\012", "\n",
		"\\134", "\\",
	)
	return replacer.Replace(path)
}
