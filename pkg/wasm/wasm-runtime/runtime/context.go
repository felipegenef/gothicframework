//go:build js && wasm

package runtime

import (
	"encoding/json"
	"strconv"
	"syscall/js"
)

// ContextKey is a typed context identifier that carries its own codec.
// T encodes the value type — provider and consumer must use the same key.
// Construct via the factory functions (IntKey, StringKey, JsonKey, etc.),
// not as a struct literal.
type ContextKey[T any] struct {
	Name   string
	encode func(T) string
	decode func(string) T
}

// ── Primitive key factories ──────────────────────────────────────────────────

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

// RuneKey is IntKey for rune (= int32).
func RuneKey(name string) ContextKey[rune] {
	return ContextKey[rune]{
		Name:   name,
		encode: func(v rune) string { return strconv.FormatInt(int64(v), 10) },
		decode: func(s string) rune { n, _ := strconv.ParseInt(s, 10, 32); return rune(n) },
	}
}

// ByteKey is UintKey for byte (= uint8).
func ByteKey(name string) ContextKey[byte] {
	return ContextKey[byte]{
		Name:   name,
		encode: func(v byte) string { return strconv.FormatUint(uint64(v), 10) },
		decode: func(s string) byte { n, _ := strconv.ParseUint(s, 10, 8); return byte(n) },
	}
}

// JsonKey returns a ContextKey for any struct or slice type, serialized as JSON.
// T must be JSON-serializable (exported fields, no channels or functions).
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

// ── JS context store ─────────────────────────────────────────────────────────

func ensureContextStore() js.Value {
	store := js.Global().Get("__gothic_context")
	if store.IsUndefined() {
		store = js.Global().Get("Object").New()
		js.Global().Set("__gothic_context", store)
	}
	return store
}

// ── ProvideContext / UseContext ───────────────────────────────────────────────

// ProvideContext makes signal the source of truth for the named context.
// Whenever signal changes the encoded value is written to window.__gothic_context
// and broadcast via CustomEvent so all UseContext consumers update reactively.
func ProvideContext[T any](key ContextKey[T], signal *Signal[T]) {
	e := &Effect{active: true} // explicitDeps:false → auto-tracks signal via Get()
	e.fn = func() {
		encoded := key.encode(signal.Get())
		ensureContextStore().Set(key.Name, encoded)
		init := js.Global().Get("Object").New()
		init.Set("detail", encoded)
		event := js.Global().Get("CustomEvent").New("gothic:context:"+key.Name, init)
		js.Global().Get("document").Call("dispatchEvent", event)
	}
	e.run()
}

// UseContext subscribes to the named context and returns a local *Signal[T]
// that mirrors the provider's value. Reads the current value from the JS store
// at startup (handles components that load after the provider already ran), then
// listens for CustomEvents to update reactively. Use as a dep in UseEffect like
// any other signal.
func UseContext[T any](key ContextKey[T], initial T) *Signal[T] {
	current := initial
	v := ensureContextStore().Get(key.Name)
	if !v.IsUndefined() && !v.IsNull() {
		current = key.decode(v.String())
	}

	s := &Signal[T]{value: current}

	listener := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) > 0 {
			detail := args[0].Get("detail")
			if !detail.IsUndefined() && !detail.IsNull() {
				s.Set(key.decode(detail.String()))
			}
		}
		return nil
	})
	keep = append(keep, listener)
	js.Global().Get("document").Call("addEventListener", "gothic:context:"+key.Name, listener)

	return s
}
