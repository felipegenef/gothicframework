//go:build !js || !wasm

package runtime

// ContextKey is a typed identifier for a shared context value.
type ContextKey[T any] struct {
	Name string
}

func ProvideContext[T any](key ContextKey[T], signal *Signal[T], encode func(T) string) {}

func UseContext[T any](key ContextKey[T], initial T, decode func(string) T) *Signal[T] {
	return &Signal[T]{value: initial}
}
