//go:build !js || !wasm

package runtime

var batchDepth int
var pendingEffects []*Effect

func addPending(_ *Effect) {}

func Batch(fn func()) { fn() }

func Computed[T any](fn func() T) *Signal[T] { return &Signal[T]{value: fn()} }
func Memo[T any](fn func() T) *Signal[T]     { return Computed(fn) }
