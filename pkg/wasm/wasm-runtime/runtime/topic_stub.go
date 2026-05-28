//go:build !js || !wasm

package runtime

import (
	"strconv"
)

// Compression is the compression algorithm used for a topic's WASM payload.
type Compression int

const (
	GZIP   Compression = iota // default
	BROTLI Compression = iota
)

// WasmCompiler selects the WASM build toolchain for a topic.
type WasmCompiler int

const (
	GothicTinyGo WasmCompiler = iota // default: embedded TinyGo binary
	LocalTinyGo                      // system tinygo binary in PATH
	Golang                           // GOOS=js GOARCH=wasm standard Go compiler
)

// TopicConfig holds per-topic configuration.
type TopicConfig struct {
	Name             string
	Compression      Compression  // GZIP (default) or BROTLI
	Compiler         WasmCompiler // GothicTinyGo (default), LocalTinyGo, or Golang
	SubscriberFnName string       // overrides generated accessor func name (default: <StructName>Topic)
	ComponentFnName  string       // overrides generated mount component func name (default: Add<StructName>Topic)
}

// CreateTopic declares a topic. The CLI AST scanner detects this call and
// generates the concrete typed accessor. At runtime this returns a no-op.
func CreateTopic[T any](zero T, cfg TopicConfig) func() interface{} {
	return func() interface{} { return nil }
}

// TopicKey is a typed topic identifier that carries its own codec.
type TopicKey[T any] struct {
	Name   string
	encode func(T) string
	decode func(string) T
}

// ── Primitive key factories ──────────────────────────────────────────────────
//
// All 16 primitive TopicKey factories share the same shape: build a
// TopicKey[T] with a strconv-based encode and a strconv-based decode. The
// helper `newPrimitiveKey` removes the boilerplate. This file mirrors
// topic.go's structure so the two cannot drift apart.

// newPrimitiveKey builds a TopicKey[T] for a primitive value using the
// provided strconv-based encode and decode functions.
func newPrimitiveKey[T any](name string, encode func(T) string, decode func(string) T) TopicKey[T] {
	return TopicKey[T]{Name: name, encode: encode, decode: decode}
}

func BoolKey(name string) TopicKey[bool] {
	return newPrimitiveKey(name,
		strconv.FormatBool,
		func(s string) bool { b, _ := strconv.ParseBool(s); return b },
	)
}

func StringKey(name string) TopicKey[string] {
	return newPrimitiveKey(name,
		func(s string) string { return s },
		func(s string) string { return s },
	)
}

func IntKey(name string) TopicKey[int] {
	return newPrimitiveKey(name,
		strconv.Itoa,
		func(s string) int { n, _ := strconv.Atoi(s); return n },
	)
}

func Int8Key(name string) TopicKey[int8] {
	return newPrimitiveKey(name,
		func(v int8) string { return strconv.FormatInt(int64(v), 10) },
		func(s string) int8 { n, _ := strconv.ParseInt(s, 10, 8); return int8(n) },
	)
}

func Int16Key(name string) TopicKey[int16] {
	return newPrimitiveKey(name,
		func(v int16) string { return strconv.FormatInt(int64(v), 10) },
		func(s string) int16 { n, _ := strconv.ParseInt(s, 10, 16); return int16(n) },
	)
}

func Int32Key(name string) TopicKey[int32] {
	return newPrimitiveKey(name,
		func(v int32) string { return strconv.FormatInt(int64(v), 10) },
		func(s string) int32 { n, _ := strconv.ParseInt(s, 10, 32); return int32(n) },
	)
}

func Int64Key(name string) TopicKey[int64] {
	return newPrimitiveKey(name,
		func(v int64) string { return strconv.FormatInt(v, 10) },
		func(s string) int64 { n, _ := strconv.ParseInt(s, 10, 64); return n },
	)
}

func UintKey(name string) TopicKey[uint] {
	return newPrimitiveKey(name,
		func(v uint) string { return strconv.FormatUint(uint64(v), 10) },
		func(s string) uint { n, _ := strconv.ParseUint(s, 10, 64); return uint(n) },
	)
}

func Uint8Key(name string) TopicKey[uint8] {
	return newPrimitiveKey(name,
		func(v uint8) string { return strconv.FormatUint(uint64(v), 10) },
		func(s string) uint8 { n, _ := strconv.ParseUint(s, 10, 8); return uint8(n) },
	)
}

func Uint16Key(name string) TopicKey[uint16] {
	return newPrimitiveKey(name,
		func(v uint16) string { return strconv.FormatUint(uint64(v), 10) },
		func(s string) uint16 { n, _ := strconv.ParseUint(s, 10, 16); return uint16(n) },
	)
}

func Uint32Key(name string) TopicKey[uint32] {
	return newPrimitiveKey(name,
		func(v uint32) string { return strconv.FormatUint(uint64(v), 10) },
		func(s string) uint32 { n, _ := strconv.ParseUint(s, 10, 32); return uint32(n) },
	)
}

func Uint64Key(name string) TopicKey[uint64] {
	return newPrimitiveKey(name,
		func(v uint64) string { return strconv.FormatUint(v, 10) },
		func(s string) uint64 { n, _ := strconv.ParseUint(s, 10, 64); return n },
	)
}

func Float32Key(name string) TopicKey[float32] {
	return newPrimitiveKey(name,
		func(v float32) string { return strconv.FormatFloat(float64(v), 'f', -1, 32) },
		func(s string) float32 { f, _ := strconv.ParseFloat(s, 32); return float32(f) },
	)
}

func Float64Key(name string) TopicKey[float64] {
	return newPrimitiveKey(name,
		func(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) },
		func(s string) float64 { f, _ := strconv.ParseFloat(s, 64); return f },
	)
}

// RuneKey is IntKey for rune (= int32).
func RuneKey(name string) TopicKey[rune] {
	return newPrimitiveKey(name,
		func(v rune) string { return strconv.FormatInt(int64(v), 10) },
		func(s string) rune { n, _ := strconv.ParseInt(s, 10, 32); return rune(n) },
	)
}

// ByteKey is UintKey for byte (= uint8).
func ByteKey(name string) TopicKey[byte] {
	return newPrimitiveKey(name,
		func(v byte) string { return strconv.FormatUint(uint64(v), 10) },
		func(s string) byte { n, _ := strconv.ParseUint(s, 10, 8); return byte(n) },
	)
}

func CustomKey[T any](name string, encode func(T) string, decode func(string) T) TopicKey[T] {
	return TopicKey[T]{Name: name, encode: encode, decode: decode}
}

func BinaryKey[T any](name string, encode func(T, *Encoder), decode func(*Decoder) T) TopicKey[T] {
	return TopicKey[T]{
		Name: name,
		encode: func(v T) string {
			e := NewEncoder(64)
			encode(v, e)
			return HexEncode(e.Buf)
		},
		decode: func(s string) T {
			d := &Decoder{Buf: HexDecode(s)}
			return decode(d)
		},
	}
}

// AutoKey is rewritten to BinaryKey by the CLI before TinyGo compiles.
// This stub exists so server-side code compiles without error.
func AutoKey[T any](name string) TopicKey[T] { return TopicKey[T]{Name: name} }

type SharedTopicObservable[T any] struct{ inner *Observable[T] }

func (s *SharedTopicObservable[T]) Get() T                 { return s.inner.value }
func (s *SharedTopicObservable[T]) Set(v T)                { s.inner.value = v }
func (s *SharedTopicObservable[T]) addEffect(e *Subscription)    { s.inner.addEffect(e) }
func (s *SharedTopicObservable[T]) removeEffect(e *Subscription) { s.inner.removeEffect(e) }

func RequestTopicSet(keyName, encoded string)                {}
func ListenTopicSetReq(keyName string, fn func(string))      {}
func PingTopicManager(keyName string)                        {}
func ListenTopicOnline(keyName string, fn func(string))      {}
func ListenTopicPing(keyName string, fn func())              {}
func BroadcastTopicOnline(keyName, encoded string)           {}
func UpdateTopicOnlineStore(keyName string, encoded []byte)  {}
func PingUntilOnline(keyName string, isOnline func() bool) {}
func BroadcastTopicEncoded(keyName, encoded string)          {}
func ListenTopicEvent(keyName string, fn func(string))       {}

func BroadcastTopicEncodedField(keyName, fieldName, encoded string)             {}
func RequestTopicSetField(keyName, fieldName, encoded string)                   {}
func ListenTopicEventField(keyName, fieldName string, fn func(string))          {}
func ListenTopicSetReqField(keyName, fieldName string, fn func(string))         {}

// ObservableField stub — no-op broadcast and tracking for server-side compilation.
type ObservableField[T any] struct{ sig *Observable[T] }

func NewObservableField[T any](initial T) *ObservableField[T] {
	return &ObservableField[T]{sig: &Observable[T]{value: initial}}
}
func (f *ObservableField[T]) SetBroadcast(fn func())  {}
func (f *ObservableField[T]) Get() T                  { return f.sig.Get() }
func (f *ObservableField[T]) Peek() T                 { return f.sig.value }
func (f *ObservableField[T]) Set(v T)                 { f.sig.value = v }
func (f *ObservableField[T]) ApplyExternal(v T)       { f.sig.Set(v) }
func (f *ObservableField[T]) addEffect(_ *Subscription)     {}
func (f *ObservableField[T]) removeEffect(_ *Subscription)  {}
