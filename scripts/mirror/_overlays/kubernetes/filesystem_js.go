//go:build js

package filesystem

import (
	"fmt"
	"os"
	"path/filepath"
)

func IsUnixDomainSocket(filePath string) (bool, error) {
	return false, fmt.Errorf("unix domain sockets are not available: %s", filePath)
}

func Chmod(name string, mode os.FileMode) error {
	return os.Chmod(name, mode)
}

func MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

func IsAbs(path string) bool {
	return filepath.IsAbs(path)
}
