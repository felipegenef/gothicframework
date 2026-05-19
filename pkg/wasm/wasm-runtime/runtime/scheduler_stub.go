//go:build !js || !wasm

package runtime

var batchDepth int
var pendingSubscriptions []*Subscription

func addPendingSubscription(_ *Subscription) {}

// BeginBatch is a no-op in non-WASM builds.
func BeginBatch() {}

// EndBatch is a no-op in non-WASM builds.
func EndBatch() {}
