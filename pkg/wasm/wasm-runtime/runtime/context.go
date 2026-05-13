//go:build js && wasm

package runtime

import (
	"encoding/json"
	"strconv"
	"syscall/js"
)

// GothicSharedContext is a zero-size marker type embedded in context structs.
// The CLI reads the name tag on this field to derive the context key name.
// Embedding it also satisfies the SharedContext constraint.
type GothicSharedContext struct{}

func (GothicSharedContext) isGothicSharedContext() {}

// SharedContext is the compile-time constraint for context types.
type SharedContext interface{ isGothicSharedContext() }

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

// ── UseContext / UseSharedState ───────────────────────────────────────────────

// UseContext subscribes to the named context and returns a local *Signal[T]
// that mirrors the provider's value. Reads the current value from the JS store
// at startup (handles components that load after the provider already ran), then
// listens for CustomEvents to update reactively. Use as a dep in UseEffect like
// any other signal.
func UseContext[T SharedContext](key ContextKey[T], initial T) *Signal[T] {
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

// ── SharedSignal / UseSharedState ─────────────────────────────────────────────

// SharedSignal is a reactive signal whose Set broadcasts to all WASM modules
// sharing the same ContextKey. Any module calling UseSharedState with the same
// key can both read and write the value — there is no separate provider/consumer.
type SharedSignal[T any] struct {
	inner *Signal[T]
	key   ContextKey[T]
}

func (s *SharedSignal[T]) Get() T { return s.inner.Get() }

// Set updates the local value, notifies local effects, writes the new value to
// the JS context store, and dispatches a CustomEvent so every other WASM module
// that called UseSharedState with the same key receives the update.
func (s *SharedSignal[T]) Set(v T) {
	s.inner.value = v
	s.inner.notifyAll()
	encoded := s.key.encode(v)
	ensureContextStore().Set(s.key.Name, encoded)
	init := js.Global().Get("Object").New()
	init.Set("detail", encoded)
	event := js.Global().Get("CustomEvent").New("gothic:context:"+s.key.Name, init)
	js.Global().Get("document").Call("dispatchEvent", event)
}

// Implements dependency so *SharedSignal[T] works as a UseEffect dep.
func (s *SharedSignal[T]) addEffect(e *Effect)    { s.inner.addEffect(e) }
func (s *SharedSignal[T]) removeEffect(e *Effect) { s.inner.removeEffect(e) }

// UseSharedState subscribes to the named context and returns a *SharedSignal[T].
// Unlike UseContext, calling Set on the returned signal propagates the new value
// to all other WASM modules sharing the same key.
// The module that calls Set first becomes the effective source of truth for that update.
//
// Example (any module — page or component):
//
//	cart := UseSharedState(CartItemsKey, []CartItem{})
//
//	UseEffect(func() {
//	    SetText("cart-count", strconv.Itoa(len(cart.Get())))
//	}, cart)
//
//	Register("clearCart", func() {
//	    cart.Set([]CartItem{})  // propagates to every other module instantly
//	})
func UseSharedState[T SharedContext](key ContextKey[T], initial T) *SharedSignal[T] {
	current := initial
	v := ensureContextStore().Get(key.Name)
	if !v.IsUndefined() && !v.IsNull() {
		current = key.decode(v.String())
	}

	ss := &SharedSignal[T]{
		inner: &Signal[T]{value: current},
		key:   key,
	}

	// Listen for writes from other modules.
	listener := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) > 0 {
			detail := args[0].Get("detail")
			if !detail.IsUndefined() && !detail.IsNull() {
				decoded := key.decode(detail.String())
				// Update local value and notify local effects without re-broadcasting.
				ss.inner.value = decoded
				ss.inner.notifyAll()
			}
		}
		return nil
	})
	keep = append(keep, listener)
	js.Global().Get("document").Call("addEventListener", "gothic:context:"+key.Name, listener)

	return ss
}

// AutoKey is rewritten to BinaryKey by the CLI before TinyGo compiles.
// This stub exists so server-side code compiles; WASM code never calls it directly.
func AutoKey[T any](name string) ContextKey[T] { return ContextKey[T]{Name: name} }

// CustomKey returns a ContextKey with user-supplied encode/decode functions.
// Use this to avoid the encoding/json dependency when JsonKey's binary size is a concern.
func CustomKey[T any](name string, encode func(T) string, decode func(string) T) ContextKey[T] {
	return ContextKey[T]{Name: name, encode: encode, decode: decode}
}

// BinaryKey returns a ContextKey that serializes T using a compact little-endian binary
// codec instead of JSON. No reflection, no encoding/json — just typed Encoder/Decoder calls.
// The encode function writes fields onto e; the decode function reads them back and returns T.
// Field order must match between encode and decode.
//
// Example:
//
//	BinaryKey[PageCtx]("page-ctx",
//	    func(v PageCtx, e *Encoder) {
//	        e.I32(int32(v.Pings))
//	        e.String(v.Label)
//	        e.String(v.Theme)
//	    },
//	    func(d *Decoder) PageCtx {
//	        return PageCtx{Pings: int(d.I32()), Label: d.String(), Theme: d.String()}
//	    },
//	)
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
