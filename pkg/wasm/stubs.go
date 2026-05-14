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
	"math"
)

// GothicSharedContext is a zero-size marker type embedded in context structs.
// The CLI reads the name tag on this field to derive the context key name
// and auto-generates the BinaryKey variable (e.g. var PageCtxKey = BinaryKey[PageCtx](...)).
// Embedding it also satisfies the SharedContext constraint, which is required by the context API.
type GothicSharedContext struct{}

func (GothicSharedContext) isGothicSharedContext() {}

// SharedContext is the compile-time constraint for context types.
// Only structs that embed GothicSharedContext satisfy it.
type SharedContext interface{ isGothicSharedContext() }

// Observable is a typed reactive state container (server-side no-op).
// Similar to useState in React — holds a value and notifies observers on change.
type Observable[T any] struct{ value T }

// Subscription is a reactive computation (server-side no-op).
type Subscription struct{}

// CreateObservable creates an Observable with the given initial value.
// It is the Gothic equivalent of React's useState hook.
//
// Example:
//
//	count := CreateObservable(0)
//	label := CreateObservable("hello")
//
//	count.Set(count.Get() + 1)  // triggers all Observe callbacks that depend on count
func CreateObservable[T any](initial T) *Observable[T] { return &Observable[T]{value: initial} }

// Get returns the current observable value.
func (s *Observable[T]) Get() T { return s.value }

// Set updates the observable value.
func (s *Observable[T]) Set(v T) { s.value = v }

// Observe runs fn immediately and re-runs it whenever a listed dep changes.
// It is the Gothic equivalent of React's useEffect hook.
// Pass no deps to run fn exactly once with no reactive subscription.
//
// Example:
//
//	count := CreateObservable(0)
//
//	Observe(func() {
//	    SetText("counter", fmt.Sprintf("%d", count.Get()))
//	}, count)
func Observe(fn func(), deps ...any) *Subscription { return &Subscription{} }

// ObserveWithCleanup is like Observe with a cleanup function.
func ObserveWithCleanup(fn func() func(), deps ...any) *Subscription { return &Subscription{} }

// Stop deactivates an effect (no-op server-side).
func (e *Subscription) Stop() {}

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

func CreateWasmFunc(name string, fn func())            {}
func CreateWasmStringFunc(name string, fn func(string)) {}
func CreateWasmBoolFunc(name string, fn func(bool))    {}

// ── Context infrastructure (generated code only — not part of the user API) ───

// ContextKey is a typed key used by the auto-generated context system.
// Users never construct these directly — the CLI generates them from src/context/*.go.
type ContextKey[T any] struct {
	Name   string
	encode func(T) string
	decode func(string) T
}

// BinaryKey is used exclusively by CLI-generated code in src/context/context_gen.go.
func BinaryKey[T any](name string, encode func(T, *Encoder), decode func(*Decoder) T) ContextKey[T] {
	return ContextKey[T]{
		Name: name,
		encode: func(v T) string {
			e := NewEncoder(64)
			encode(v, e)
			return hexEncode(e.Buf)
		},
		decode: func(s string) T {
			d := &Decoder{Buf: hexDecode(s)}
			return decode(d)
		},
	}
}

// AutoKey is rewritten to BinaryKey by the CLI before TinyGo compiles.
// Server-side this is a no-op stub so the code compiles.
func AutoKey[T any](name string) ContextKey[T] { return ContextKey[T]{Name: name} }

// Encoder writes a little-endian binary stream (server-side stub — mirrors runtime.Encoder).
type Encoder struct{ Buf []byte }

func NewEncoder(cap int) *Encoder { return &Encoder{Buf: make([]byte, 0, cap)} }
func (e *Encoder) U8(v uint8)     { e.Buf = append(e.Buf, v) }
func (e *Encoder) U16(v uint16)   { e.Buf = append(e.Buf, byte(v), byte(v>>8)) }
func (e *Encoder) U32(v uint32) {
	e.Buf = append(e.Buf, byte(v), byte(v>>8), byte(v>>16), byte(v>>24))
}
func (e *Encoder) U64(v uint64) {
	e.Buf = append(e.Buf, byte(v), byte(v>>8), byte(v>>16), byte(v>>24),
		byte(v>>32), byte(v>>40), byte(v>>48), byte(v>>56))
}
func (e *Encoder) I32(v int32)   { e.U32(uint32(v)) }
func (e *Encoder) I64(v int64)   { e.U64(uint64(v)) }
func (e *Encoder) F32(v float32) { e.U32(math.Float32bits(v)) }
func (e *Encoder) F64(v float64) { e.U64(math.Float64bits(v)) }
func (e *Encoder) Bool(v bool) {
	b := byte(0)
	if v {
		b = 1
	}
	e.Buf = append(e.Buf, b)
}
func (e *Encoder) Bytes(v []byte)  { e.U32(uint32(len(v))); e.Buf = append(e.Buf, v...) }
func (e *Encoder) String(v string) { e.U32(uint32(len(v))); e.Buf = append(e.Buf, v...) }

// Decoder reads a little-endian binary stream (server-side stub — mirrors runtime.Decoder).
type Decoder struct {
	Buf []byte
	Pos int
	Err error
}

func (d *Decoder) need(n int) bool {
	if d.Err != nil {
		return false
	}
	if d.Pos+n > len(d.Buf) {
		d.Err = decErr("codec: buffer underflow")
		return false
	}
	return true
}

type decErr string

func (e decErr) Error() string { return string(e) }

func (d *Decoder) U8() uint8 {
	if !d.need(1) {
		return 0
	}
	v := d.Buf[d.Pos]
	d.Pos++
	return v
}
func (d *Decoder) U16() uint16 {
	if !d.need(2) {
		return 0
	}
	v := uint16(d.Buf[d.Pos]) | uint16(d.Buf[d.Pos+1])<<8
	d.Pos += 2
	return v
}
func (d *Decoder) U32() uint32 {
	if !d.need(4) {
		return 0
	}
	v := uint32(d.Buf[d.Pos]) | uint32(d.Buf[d.Pos+1])<<8 |
		uint32(d.Buf[d.Pos+2])<<16 | uint32(d.Buf[d.Pos+3])<<24
	d.Pos += 4
	return v
}
func (d *Decoder) U64() uint64 {
	if !d.need(8) {
		return 0
	}
	v := uint64(d.Buf[d.Pos]) | uint64(d.Buf[d.Pos+1])<<8 |
		uint64(d.Buf[d.Pos+2])<<16 | uint64(d.Buf[d.Pos+3])<<24 |
		uint64(d.Buf[d.Pos+4])<<32 | uint64(d.Buf[d.Pos+5])<<40 |
		uint64(d.Buf[d.Pos+6])<<48 | uint64(d.Buf[d.Pos+7])<<56
	d.Pos += 8
	return v
}
func (d *Decoder) I32() int32   { return int32(d.U32()) }
func (d *Decoder) I64() int64   { return int64(d.U64()) }
func (d *Decoder) F32() float32 { return math.Float32frombits(d.U32()) }
func (d *Decoder) F64() float64 { return math.Float64frombits(d.U64()) }
func (d *Decoder) Bool() bool   { return d.U8() != 0 }
func (d *Decoder) Bytes() []byte {
	n := d.U32()
	if !d.need(int(n)) {
		return nil
	}
	v := d.Buf[d.Pos : d.Pos+int(n)]
	d.Pos += int(n)
	return v
}
func (d *Decoder) String() string {
	n := d.U32()
	if !d.need(int(n)) {
		return ""
	}
	v := string(d.Buf[d.Pos : d.Pos+int(n)])
	d.Pos += int(n)
	return v
}

const hextable = "0123456789abcdef"

func hexEncode(src []byte) string {
	dst := make([]byte, len(src)*2)
	for i, b := range src {
		dst[i*2] = hextable[b>>4]
		dst[i*2+1] = hextable[b&0xf]
	}
	return string(dst)
}

func hexDecode(s string) []byte {
	if len(s)%2 != 0 {
		return nil
	}
	dst := make([]byte, len(s)/2)
	for i := 0; i < len(s); i += 2 {
		dst[i/2] = unhex(s[i])<<4 | unhex(s[i+1])
	}
	return dst
}

func unhex(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	}
	return 0
}

// SharedCtxObservable is the internal type backing auto-generated context constructors.
// Users access shared context via the generated e.g. PageCtxContext() — not directly.
type SharedCtxObservable[T any] struct{ value T }

func (s *SharedCtxObservable[T]) Get() T  { return s.value }
func (s *SharedCtxObservable[T]) Set(v T) { s.value = v }

// ObservableField is a per-field reactive observable for a generated context struct.
// Server-side stub — no broadcast, no effect tracking.
type ObservableField[T any] struct{ sig *Observable[T] }

// NewObservableField creates a ContextField with the given initial value.
func NewObservableField[T any](initial T) *ObservableField[T] {
	return &ObservableField[T]{sig: &Observable[T]{value: initial}}
}
func (f *ObservableField[T]) SetBroadcast(fn func()) {}
func (f *ObservableField[T]) Get() T                 { return f.sig.Get() }
func (f *ObservableField[T]) Peek() T                { return f.sig.value }
func (f *ObservableField[T]) Set(v T)                { f.sig.value = v }
func (f *ObservableField[T]) ApplyExternal(v T)      { f.sig.Set(v) }

