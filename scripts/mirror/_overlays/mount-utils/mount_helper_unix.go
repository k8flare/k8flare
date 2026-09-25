//go:build js

package mount

import (
	"errors"
	"io/fs"
	"os"
)

func IsCorruptedMnt(error) bool { return false }

func PathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return false, err
}
