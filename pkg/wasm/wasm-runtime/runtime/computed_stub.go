//go:build !js || !wasm

package runtime

var batchDepth int
var pendingEffects []*Effect

func addPending(_ *Effect) {}

func Batch(fn func()) { fn() }

