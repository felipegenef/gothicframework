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
func UseEffect(fn func()) *Effect { return &Effect{} }

// UseEffectWithCleanup is like UseEffect with a cleanup function.
func UseEffectWithCleanup(fn func() func()) *Effect { return &Effect{} }

// Stop deactivates an effect (no-op server-side).
func (e *Effect) Stop() {}

// Computed creates a derived read-only signal.
func Computed[T any](fn func() T) *Signal[T] { return &Signal[T]{value: fn()} }

// Memo is an alias for Computed.
func Memo[T any](fn func() T) *Signal[T] { return &Signal[T]{value: fn()} }

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
