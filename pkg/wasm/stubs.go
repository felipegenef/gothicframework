// Package wasm provides server-side stubs for the WASM reactive runtime.
// Import this package with a dot import in page files so the state function
// compiles server-side:
//
//	import . "github.com/felipegenef/gothicframework/pkg/wasm"
//
// At WASM compile time the framework substitutes the real TinyGo implementation
// (signal tracking, DOM manipulation, JS event registration) from the embedded
// wasm-runtime module.  On the server these are all no-ops.
package wasm

import (
	"encoding/json"
	"strconv"
)

// Signal is a typed reactive state container (server-side no-op).
type Signal[T any] struct{ value T }

// Effect is a reactive computation (server-side no-op).
type Effect struct{}

// UseState creates a Signal with the given initial value.
func UseState[T any](initial T) *Signal[T] { return &Signal[T]{value: initial} }

// Get returns the current signal value.
func (s *Signal[T]) Get() T { return s.value }

// Set updates the signal value.
func (s *Signal[T]) Set(v T) { s.value = v }

// UseEffect runs fn immediately (server-side no-op — fn is not called).
// Pass deps to re-run fn when those signals change; pass none to run once.
func UseEffect(fn func(), deps ...any) *Effect { return &Effect{} }

// UseEffectWithCleanup is like UseEffect with a cleanup function.
func UseEffectWithCleanup(fn func() func(), deps ...any) *Effect { return &Effect{} }

// Stop deactivates an effect (no-op server-side).
func (e *Effect) Stop() {}

// Batch defers effect re-executions (server-side runs fn immediately).
func Batch(fn func()) { fn() }

// DOM helpers — all no-ops on the server.

func SetText(id, value string)            {}
func SetHTML(id, html string)             {}
func SetValue(id, value string)           {}
func GetValue(id string) string           { return "" }
func AddClass(id, className string)       {}
func RemoveClass(id, className string)    {}
func ToggleClass(id, className string)    {}
func SetAttr(id, attr, value string)      {}
func SetStyle(id, property, value string) {}

// Event registration — no-ops on the server.

func Register(name string, fn func())            {}
func RegisterInput(name string, fn func(string)) {}
func RegisterBool(name string, fn func(bool))    {}

// Context — no-ops on the server.

// ContextKey is a typed context identifier that carries its own codec.
// Construct via the factory functions (IntKey, StringKey, JsonKey, etc.).
type ContextKey[T any] struct {
	Name   string
	encode func(T) string
	decode func(string) T
}

func BoolKey(name string) ContextKey[bool] {
	return ContextKey[bool]{Name: name, encode: strconv.FormatBool, decode: func(s string) bool { b, _ := strconv.ParseBool(s); return b }}
}
func StringKey(name string) ContextKey[string] {
	return ContextKey[string]{Name: name, encode: func(s string) string { return s }, decode: func(s string) string { return s }}
}
func IntKey(name string) ContextKey[int] {
	return ContextKey[int]{Name: name, encode: strconv.Itoa, decode: func(s string) int { n, _ := strconv.Atoi(s); return n }}
}
func Int8Key(name string) ContextKey[int8] {
	return ContextKey[int8]{Name: name, encode: func(v int8) string { return strconv.FormatInt(int64(v), 10) }, decode: func(s string) int8 { n, _ := strconv.ParseInt(s, 10, 8); return int8(n) }}
}
func Int16Key(name string) ContextKey[int16] {
	return ContextKey[int16]{Name: name, encode: func(v int16) string { return strconv.FormatInt(int64(v), 10) }, decode: func(s string) int16 { n, _ := strconv.ParseInt(s, 10, 16); return int16(n) }}
}
func Int32Key(name string) ContextKey[int32] {
	return ContextKey[int32]{Name: name, encode: func(v int32) string { return strconv.FormatInt(int64(v), 10) }, decode: func(s string) int32 { n, _ := strconv.ParseInt(s, 10, 32); return int32(n) }}
}
func Int64Key(name string) ContextKey[int64] {
	return ContextKey[int64]{Name: name, encode: func(v int64) string { return strconv.FormatInt(v, 10) }, decode: func(s string) int64 { n, _ := strconv.ParseInt(s, 10, 64); return n }}
}
func UintKey(name string) ContextKey[uint] {
	return ContextKey[uint]{Name: name, encode: func(v uint) string { return strconv.FormatUint(uint64(v), 10) }, decode: func(s string) uint { n, _ := strconv.ParseUint(s, 10, 64); return uint(n) }}
}
func Uint8Key(name string) ContextKey[uint8] {
	return ContextKey[uint8]{Name: name, encode: func(v uint8) string { return strconv.FormatUint(uint64(v), 10) }, decode: func(s string) uint8 { n, _ := strconv.ParseUint(s, 10, 8); return uint8(n) }}
}
func Uint16Key(name string) ContextKey[uint16] {
	return ContextKey[uint16]{Name: name, encode: func(v uint16) string { return strconv.FormatUint(uint64(v), 10) }, decode: func(s string) uint16 { n, _ := strconv.ParseUint(s, 10, 16); return uint16(n) }}
}
func Uint32Key(name string) ContextKey[uint32] {
	return ContextKey[uint32]{Name: name, encode: func(v uint32) string { return strconv.FormatUint(uint64(v), 10) }, decode: func(s string) uint32 { n, _ := strconv.ParseUint(s, 10, 32); return uint32(n) }}
}
func Uint64Key(name string) ContextKey[uint64] {
	return ContextKey[uint64]{Name: name, encode: func(v uint64) string { return strconv.FormatUint(v, 10) }, decode: func(s string) uint64 { n, _ := strconv.ParseUint(s, 10, 64); return n }}
}
func Float32Key(name string) ContextKey[float32] {
	return ContextKey[float32]{Name: name, encode: func(v float32) string { return strconv.FormatFloat(float64(v), 'f', -1, 32) }, decode: func(s string) float32 { f, _ := strconv.ParseFloat(s, 32); return float32(f) }}
}
func Float64Key(name string) ContextKey[float64] {
	return ContextKey[float64]{Name: name, encode: func(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }, decode: func(s string) float64 { f, _ := strconv.ParseFloat(s, 64); return f }}
}
func RuneKey(name string) ContextKey[rune] {
	return ContextKey[rune]{Name: name, encode: func(v rune) string { return strconv.FormatInt(int64(v), 10) }, decode: func(s string) rune { n, _ := strconv.ParseInt(s, 10, 32); return rune(n) }}
}
func ByteKey(name string) ContextKey[byte] {
	return ContextKey[byte]{Name: name, encode: func(v byte) string { return strconv.FormatUint(uint64(v), 10) }, decode: func(s string) byte { n, _ := strconv.ParseUint(s, 10, 8); return byte(n) }}
}

// JsonKey returns a ContextKey for any struct or slice type, serialized as JSON.
func JsonKey[T any](name string) ContextKey[T] {
	return ContextKey[T]{
		Name: name,
		encode: func(v T) string { b, _ := json.Marshal(v); return string(b) },
		decode: func(s string) T { var v T; _ = json.Unmarshal([]byte(s), &v); return v },
	}
}

// ProvideContext makes signal the source of truth for the named context (no-op server-side).
func ProvideContext[T any](key ContextKey[T], signal *Signal[T]) {}

// UseContext subscribes to the named context and returns a local signal (no-op server-side).
func UseContext[T any](key ContextKey[T], initial T) *Signal[T] {
	return &Signal[T]{value: initial}
}
