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
	"math"
	"strconv"
)

// GothicSharedContext is a zero-size marker type embedded in context structs.
// The CLI reads the name tag on this field to derive the context key name
// and auto-generates the BinaryKey variable (e.g. var PageCtxKey = BinaryKey[PageCtx](...)).
// Embedding it also satisfies the SharedContext constraint, which is required by
// UseContext and UseSharedState.
type GothicSharedContext struct{}

func (GothicSharedContext) isGothicSharedContext() {}

// SharedContext is the compile-time constraint for context types.
// Only structs that embed GothicSharedContext satisfy it.
type SharedContext interface{ isGothicSharedContext() }

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

// CustomKey returns a ContextKey with user-supplied encode/decode functions.
func CustomKey[T any](name string, encode func(T) string, decode func(string) T) ContextKey[T] {
	return ContextKey[T]{Name: name, encode: encode, decode: decode}
}

// BinaryKey returns a ContextKey that serializes T using a compact little-endian binary codec.
// No reflection, no encoding/json — just typed Encoder/Decoder calls.
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

// AutoKey is the zero-boilerplate context key. The CLI reads the struct definition
// from src/wasm/*.go, generates _encode_T / _decode_T, and rewrites AutoKey[T]("name")
// to BinaryKey[T]("name", _encode_T, _decode_T) before TinyGo compiles.
// Server-side this returns a stub key (ProvideContext/UseContext are no-ops anyway).
func AutoKey[T any](name string) ContextKey[T] {
	return ContextKey[T]{Name: name}
}

// SharedSignal server-side stub — same API as the WASM implementation, no-op broadcast.
type SharedSignal[T any] struct{ value T }

func (s *SharedSignal[T]) Get() T  { return s.value }
func (s *SharedSignal[T]) Set(v T) { s.value = v }

// UseSharedState returns a SharedSignal subscribed to the named context.
// Calling Set propagates the value to all WASM modules sharing the same key.
func UseSharedState[T SharedContext](key ContextKey[T], initial T) *SharedSignal[T] {
	return &SharedSignal[T]{value: initial}
}

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
func (e *Encoder) Bool(v bool)   { b := byte(0); if v { b = 1 }; e.Buf = append(e.Buf, b) }
func (e *Encoder) Bytes(v []byte)  { e.U32(uint32(len(v))); e.Buf = append(e.Buf, v...) }
func (e *Encoder) String(v string) { e.U32(uint32(len(v))); e.Buf = append(e.Buf, v...) }

// Decoder reads a little-endian binary stream (server-side stub — mirrors runtime.Decoder).
type Decoder struct {
	Buf []byte
	Pos int
	Err error
}

func (d *Decoder) need(n int) bool {
	if d.Err != nil { return false }
	if d.Pos+n > len(d.Buf) { d.Err = decErr("codec: buffer underflow"); return false }
	return true
}

type decErr string
func (e decErr) Error() string { return string(e) }

func (d *Decoder) U8() uint8 {
	if !d.need(1) { return 0 }
	v := d.Buf[d.Pos]; d.Pos++; return v
}
func (d *Decoder) U16() uint16 {
	if !d.need(2) { return 0 }
	v := uint16(d.Buf[d.Pos]) | uint16(d.Buf[d.Pos+1])<<8; d.Pos += 2; return v
}
func (d *Decoder) U32() uint32 {
	if !d.need(4) { return 0 }
	v := uint32(d.Buf[d.Pos]) | uint32(d.Buf[d.Pos+1])<<8 |
		uint32(d.Buf[d.Pos+2])<<16 | uint32(d.Buf[d.Pos+3])<<24
	d.Pos += 4; return v
}
func (d *Decoder) U64() uint64 {
	if !d.need(8) { return 0 }
	v := uint64(d.Buf[d.Pos]) | uint64(d.Buf[d.Pos+1])<<8 |
		uint64(d.Buf[d.Pos+2])<<16 | uint64(d.Buf[d.Pos+3])<<24 |
		uint64(d.Buf[d.Pos+4])<<32 | uint64(d.Buf[d.Pos+5])<<40 |
		uint64(d.Buf[d.Pos+6])<<48 | uint64(d.Buf[d.Pos+7])<<56
	d.Pos += 8; return v
}
func (d *Decoder) I32() int32   { return int32(d.U32()) }
func (d *Decoder) I64() int64   { return int64(d.U64()) }
func (d *Decoder) F32() float32 { return math.Float32frombits(d.U32()) }
func (d *Decoder) F64() float64 { return math.Float64frombits(d.U64()) }
func (d *Decoder) Bool() bool   { return d.U8() != 0 }
func (d *Decoder) Bytes() []byte {
	n := d.U32()
	if !d.need(int(n)) { return nil }
	v := d.Buf[d.Pos : d.Pos+int(n)]; d.Pos += int(n); return v
}
func (d *Decoder) String() string {
	n := d.U32()
	if !d.need(int(n)) { return "" }
	v := string(d.Buf[d.Pos : d.Pos+int(n)]); d.Pos += int(n); return v
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
	if len(s)%2 != 0 { return nil }
	dst := make([]byte, len(s)/2)
	for i := 0; i < len(s); i += 2 {
		dst[i/2] = unhex(s[i])<<4 | unhex(s[i+1])
	}
	return dst
}

func unhex(c byte) byte {
	switch {
	case c >= '0' && c <= '9': return c - '0'
	case c >= 'a' && c <= 'f': return c - 'a' + 10
	case c >= 'A' && c <= 'F': return c - 'A' + 10
	}
	return 0
}


// UseContext subscribes to the named context and returns a local signal (no-op server-side).
func UseContext[T SharedContext](key ContextKey[T], initial T) *Signal[T] {
	return &Signal[T]{value: initial}
}
