//go:build !js || !wasm

package runtime

import (
	"encoding/json"
	"strconv"
)

// GothicSharedContext is a zero-size marker type embedded in context structs.
type GothicSharedContext struct{}

func (GothicSharedContext) isGothicSharedContext() {}

// SharedContext is the compile-time constraint for context types.
type SharedContext interface{ isGothicSharedContext() }

// ContextKey is a typed context identifier that carries its own codec.
type ContextKey[T any] struct {
	Name   string
	encode func(T) string
	decode func(string) T
}

func BoolKey(name string) ContextKey[bool] {
	return ContextKey[bool]{
		Name:   name,
		encode: strconv.FormatBool,
		decode: func(s string) bool { b, _ := strconv.ParseBool(s); return b },
	}
}

func StringKey(name string) ContextKey[string] {
	return ContextKey[string]{
		Name:   name,
		encode: func(s string) string { return s },
		decode: func(s string) string { return s },
	}
}

func IntKey(name string) ContextKey[int] {
	return ContextKey[int]{
		Name:   name,
		encode: strconv.Itoa,
		decode: func(s string) int { n, _ := strconv.Atoi(s); return n },
	}
}

func Int8Key(name string) ContextKey[int8] {
	return ContextKey[int8]{
		Name:   name,
		encode: func(v int8) string { return strconv.FormatInt(int64(v), 10) },
		decode: func(s string) int8 { n, _ := strconv.ParseInt(s, 10, 8); return int8(n) },
	}
}

func Int16Key(name string) ContextKey[int16] {
	return ContextKey[int16]{
		Name:   name,
		encode: func(v int16) string { return strconv.FormatInt(int64(v), 10) },
		decode: func(s string) int16 { n, _ := strconv.ParseInt(s, 10, 16); return int16(n) },
	}
}

func Int32Key(name string) ContextKey[int32] {
	return ContextKey[int32]{
		Name:   name,
		encode: func(v int32) string { return strconv.FormatInt(int64(v), 10) },
		decode: func(s string) int32 { n, _ := strconv.ParseInt(s, 10, 32); return int32(n) },
	}
}

func Int64Key(name string) ContextKey[int64] {
	return ContextKey[int64]{
		Name:   name,
		encode: func(v int64) string { return strconv.FormatInt(v, 10) },
		decode: func(s string) int64 { n, _ := strconv.ParseInt(s, 10, 64); return n },
	}
}

func UintKey(name string) ContextKey[uint] {
	return ContextKey[uint]{
		Name:   name,
		encode: func(v uint) string { return strconv.FormatUint(uint64(v), 10) },
		decode: func(s string) uint { n, _ := strconv.ParseUint(s, 10, 64); return uint(n) },
	}
}

func Uint8Key(name string) ContextKey[uint8] {
	return ContextKey[uint8]{
		Name:   name,
		encode: func(v uint8) string { return strconv.FormatUint(uint64(v), 10) },
		decode: func(s string) uint8 { n, _ := strconv.ParseUint(s, 10, 8); return uint8(n) },
	}
}

func Uint16Key(name string) ContextKey[uint16] {
	return ContextKey[uint16]{
		Name:   name,
		encode: func(v uint16) string { return strconv.FormatUint(uint64(v), 10) },
		decode: func(s string) uint16 { n, _ := strconv.ParseUint(s, 10, 16); return uint16(n) },
	}
}

func Uint32Key(name string) ContextKey[uint32] {
	return ContextKey[uint32]{
		Name:   name,
		encode: func(v uint32) string { return strconv.FormatUint(uint64(v), 10) },
		decode: func(s string) uint32 { n, _ := strconv.ParseUint(s, 10, 32); return uint32(n) },
	}
}

func Uint64Key(name string) ContextKey[uint64] {
	return ContextKey[uint64]{
		Name:   name,
		encode: func(v uint64) string { return strconv.FormatUint(v, 10) },
		decode: func(s string) uint64 { n, _ := strconv.ParseUint(s, 10, 64); return n },
	}
}

func Float32Key(name string) ContextKey[float32] {
	return ContextKey[float32]{
		Name:   name,
		encode: func(v float32) string { return strconv.FormatFloat(float64(v), 'f', -1, 32) },
		decode: func(s string) float32 { f, _ := strconv.ParseFloat(s, 32); return float32(f) },
	}
}

func Float64Key(name string) ContextKey[float64] {
	return ContextKey[float64]{
		Name:   name,
		encode: func(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) },
		decode: func(s string) float64 { f, _ := strconv.ParseFloat(s, 64); return f },
	}
}

func RuneKey(name string) ContextKey[rune] {
	return ContextKey[rune]{
		Name:   name,
		encode: func(v rune) string { return strconv.FormatInt(int64(v), 10) },
		decode: func(s string) rune { n, _ := strconv.ParseInt(s, 10, 32); return rune(n) },
	}
}

func ByteKey(name string) ContextKey[byte] {
	return ContextKey[byte]{
		Name:   name,
		encode: func(v byte) string { return strconv.FormatUint(uint64(v), 10) },
		decode: func(s string) byte { n, _ := strconv.ParseUint(s, 10, 8); return byte(n) },
	}
}

func JsonKey[T any](name string) ContextKey[T] {
	return ContextKey[T]{
		Name: name,
		encode: func(v T) string {
			b, _ := json.Marshal(v)
			return string(b)
		},
		decode: func(s string) T {
			var v T
			_ = json.Unmarshal([]byte(s), &v)
			return v
		},
	}
}

func CustomKey[T any](name string, encode func(T) string, decode func(string) T) ContextKey[T] {
	return ContextKey[T]{Name: name, encode: encode, decode: decode}
}

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
// This stub exists so server-side code compiles without error.
func AutoKey[T any](name string) ContextKey[T] { return ContextKey[T]{Name: name} }


type ContextSignal[T any] struct{ inner *Signal[T] }

func (s *ContextSignal[T]) Get() T                 { return s.inner.value }
func (s *ContextSignal[T]) Set(v T)                { s.inner.value = v }
func (s *ContextSignal[T]) addEffect(e *Effect)    { s.inner.addEffect(e) }
func (s *ContextSignal[T]) removeEffect(e *Effect) { s.inner.removeEffect(e) }

func UseContext[T SharedContext](key ContextKey[T], initial T) *ContextSignal[T] {
	return &ContextSignal[T]{inner: &Signal[T]{value: initial}}
}
