package node

import (
	"errors"
	"os"
	"strings"
	"syscall"
)

var probeMountPath = func(path string) error {
	_, err := os.Stat(path)
	return err
}

func isDisconnectedMountError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, syscall.ENOTCONN) ||
		errors.Is(err, syscall.EIO) ||
		errors.Is(err, syscall.ESTALE) {
		return true
	}

	message := strings.ToLower(err.Error())
	return strings.Contains(message, "transport endpoint is not connected") ||
		strings.Contains(message, "input/output error") ||
		strings.Contains(message, "stale file handle")
}
