package node

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go.uber.org/zap"
)

type mountInfoEntry struct {
	mountPoint string
	device     string
}

var readMountInfo = func() ([]byte, error) {
	return os.ReadFile("/proc/self/mountinfo")
}

func (n *Node) unmountDependentMounts(ctx context.Context, stagingPath string) error {
	stagingEntry, ok, err := mountInfoEntryForPath(stagingPath)
	if err != nil {
		return fmt.Errorf("read mountinfo: %w", err)
	}
	if !ok {
		return nil
	}

	dependents, err := dependentMounts(stagingEntry)
	if err != nil {
		return fmt.Errorf("read dependent mounts: %w", err)
	}
	for i, target := range dependents {
		Logger(ctx).Warn("unmounting dependent bind mount for disconnected staging mount",
			zap.String("staging_target_path", stagingPath),
			zap.String("target_path", target),
		)
		if err := n.unmountOnce(ctx, target, i+1); err != nil {
			return fmt.Errorf("unmount dependent bind mount %q: %w", target, err)
		}
	}
	return nil
}

func mountInfoEntryForPath(path string) (mountInfoEntry, bool, error) {
	cleaned := filepath.Clean(path)
	entries, err := mountInfoEntries()
	if err != nil {
		return mountInfoEntry{}, false, err
	}
	for _, entry := range entries {
		if filepath.Clean(entry.mountPoint) == cleaned {
			return entry, true, nil
		}
	}
	return mountInfoEntry{}, false, nil
}

func dependentMounts(stagingEntry mountInfoEntry) ([]string, error) {
	entries, err := mountInfoEntries()
	if err != nil {
		return nil, err
	}

	cleanedStagingPath := filepath.Clean(stagingEntry.mountPoint)
	var mounts []string
	for _, entry := range entries {
		cleanedMountPoint := filepath.Clean(entry.mountPoint)
		if entry.device != stagingEntry.device || cleanedMountPoint == cleanedStagingPath {
			continue
		}
		mounts = append(mounts, cleanedMountPoint)
	}

	sort.Slice(mounts, func(i, j int) bool {
		return len(mounts[i]) > len(mounts[j])
	})
	return mounts, nil
}

func mountInfoEntries() ([]mountInfoEntry, error) {
	data, err := readMountInfo()
	if err != nil {
		return nil, err
	}

	var entries []mountInfoEntry
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.SplitN(line, " - ", 2)
		fields := strings.Fields(parts[0])
		if len(fields) < 5 {
			continue
		}
		entries = append(entries, mountInfoEntry{
			mountPoint: unescapeMountPath(fields[4]),
			device:     fields[2],
		})
	}
	return entries, nil
}
