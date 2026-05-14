//go:build !js || !wasm

package runtime

var batchDepth int
var pendingSubscriptions []*Subscription

func addPendingSubscription(_ *Subscription) {}

