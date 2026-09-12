//go:build js && wasm

package cloudflare

import (
	"fmt"
	"sync"
	"time"
)

var (
	instanceOnce sync.Once
	instanceID   string
)

// InstanceID names this Go instance. The design assumes one resident
// controller per component, and the Controllers DO holds exactly one
// entrypoint per name -- but two `controllerManager: run starting` lines
// have been seen against a single `dynamic worker up`
// (docs/platform-verification.md S59), which can only happen if two Go
// instances exist. Without a name on each one, every other measurement
// silently averages them together.
func InstanceID() string {
	instanceOnce.Do(func() {
		instanceID = fmt.Sprintf("%x", time.Now().UnixNano()&0xffffff)
	})
	return instanceID
}
