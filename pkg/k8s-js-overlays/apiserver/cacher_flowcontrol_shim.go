// js/wasm overlay: utilflowcontrol.WatchInitialized drags the whole APF
// controller (and flowcontrol informers the lean client-go doesn't ship)
// into the build. On js there is no APF; the notification is a no-op.
package cacher

import "context"

func watchInitializedShim(ctx context.Context) {}
