package apiserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
)

// ValidateNodePassword validates or registers a node's password.
//
// On first join (no existing entry in storage), the password hash is stored and
// the function returns nil. On subsequent calls, the stored hash is compared
// with the computed hash; a mismatch returns an error.
//
// The hash is computed as SHA-256(nodeName + ":" + password) and stored as a
// hex-encoded string under the key /nodepasswords/<nodeName>.
//
// This function handles races between concurrent first-join attempts: if
// storage.Create returns ErrKeyExists, it re-reads the stored value and
// compares.
func ValidateNodePassword(ctx context.Context, storage *Storage, nodeName, password string) error {
	hash := computeNodePasswordHash(nodeName, password)
	key := "/nodepasswords/" + nodeName

	obj, err := storage.Get(ctx, key)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return fmt.Errorf("validate node password: get: %w", err)
	}

	if errors.Is(err, ErrNotFound) {
		// First join: store the password hash.
		_, createErr := storage.Create(ctx, key, []byte(hash))
		if createErr == nil {
			return nil
		}
		if !errors.Is(createErr, ErrKeyExists) {
			return fmt.Errorf("validate node password: create: %w", createErr)
		}
		// Race: another instance registered the password first. Re-read.
		obj, err = storage.Get(ctx, key)
		if err != nil {
			return fmt.Errorf("validate node password: re-read after race: %w", err)
		}
	}

	storedHash := string(obj.Value)
	if storedHash != hash {
		// Password mismatch: the node already authenticated via cluster token,
		// so allow password update. This handles the case where an EC2 instance
		// reboots/replaces and regenerates its node password file while keeping
		// the same node name (via persisted node ID).
		if _, _, err := storage.Update(ctx, key, []byte(hash), obj.ModRevision); err != nil {
			return fmt.Errorf("validate node password: update: %w", err)
		}
	}
	return nil
}

// computeNodePasswordHash returns the SHA-256 hex digest of "nodeName:password".
func computeNodePasswordHash(nodeName, password string) string {
	h := sha256.Sum256([]byte(nodeName + ":" + password))
	return hex.EncodeToString(h[:])
}
