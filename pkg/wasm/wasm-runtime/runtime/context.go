//go:build js && wasm

package runtime

import (
	"encoding/json"
	"strconv"
	"syscall/js"
	"time"
)

// GothicSharedContext is a zero-size marker type embedded in context structs.
// The CLI reads the name tag on this field to derive the context key name.
// Embedding it also satisfies the SharedContext constraint.
type GothicSharedContext struct{}

func (GothicSharedContext) isGothicSharedContext() {}

// SharedContext is the compile-time constraint for context types.
type SharedContext interface{ isGothicSharedContext() }

// _gothicKeyRegistry maps context key names to their ContextKey (stored as any).
// Populated by generated init() calls in the compiled WASM main.

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
// ContextSignal is a reactive signal bound to a shared context key.
// Get/Set work like a regular signal, but Set also broadcasts the new value
// to every other WASM module that called UseContext with the same key.
type ContextSignal[T any] struct {
	inner *Signal[T]
	key   ContextKey[T]
}

func (s *ContextSignal[T]) Get() T { return s.inner.Get() }

func (s *ContextSignal[T]) Set(v T) {
	s.inner.value = v
	s.inner.notifyAll()
	encoded := s.key.encode(v)
	ensureContextStore().Set(s.key.Name, encoded)
	init := js.Global().Get("Object").New()
	init.Set("detail", encoded)
	event := js.Global().Get("CustomEvent").New("gothic:context:"+s.key.Name, init)
	js.Global().Get("document").Call("dispatchEvent", event)
}

func (s *ContextSignal[T]) addEffect(e *Effect)    { s.inner.addEffect(e) }
func (s *ContextSignal[T]) removeEffect(e *Effect) { s.inner.removeEffect(e) }

// UseContext subscribes to the named shared context and returns a *ContextSignal[T].
// Reading the current value and reacting to updates from other modules is automatic.
// Calling Set broadcasts the new value to every other WASM module sharing the same key.
func UseContext[T SharedContext](key ContextKey[T], initial T) *ContextSignal[T] {
	current := initial
	v := ensureContextStore().Get(key.Name)
	if !v.IsUndefined() && !v.IsNull() {
		current = key.decode(v.String())
	}

	cs := &ContextSignal[T]{
		inner: &Signal[T]{value: current},
		key:   key,
	}

	listener := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) > 0 {
			detail := args[0].Get("detail")
			if !detail.IsUndefined() && !detail.IsNull() {
				decoded := key.decode(detail.String())
				cs.inner.value = decoded
				cs.inner.notifyAll()
			}
		}
		return nil
	})
	keep = append(keep, listener)
	js.Global().Get("document").Call("addEventListener", "gothic:context:"+key.Name, listener)

	return cs
}

// AutoKey is rewritten to BinaryKey by the CLI before TinyGo compiles.
// This stub exists so server-side code compiles; WASM code never calls it directly.
func AutoKey[T any](name string) ContextKey[T] { return ContextKey[T]{Name: name} }

// ── Per-field context signals ─────────────────────────────────────────────────

// ContextField is a reactive signal bound to one field of a shared context struct.
// It behaves like *Signal[T] but Set also broadcasts the full context to other modules.
// Pass *ContextField as a dep in UseEffect to react to individual property changes.
type ContextField[T any] struct {
	sig       *Signal[T]
	broadcast func()
}

// NewContextField creates a ContextField with the given initial value.
func NewContextField[T any](initial T) *ContextField[T] {
	return &ContextField[T]{sig: &Signal[T]{value: initial}}
}

// SetBroadcast wires the broadcast callback called whenever Set updates this field.
func (f *ContextField[T]) SetBroadcast(fn func()) { f.broadcast = fn }

// Get returns the current value, auto-registering as a dep of any running effect.
func (f *ContextField[T]) Get() T { return f.sig.Get() }

// Peek returns the current value without registering as an effect dependency.
// Used internally by broadcast closures to read sibling field values safely.
func (f *ContextField[T]) Peek() T { return f.sig.value }

// Set sends a set-request to the context manager WASM.
// The local value is silently updated so Peek() returns the correct value during
// encoding, but subscribers are NOT notified until the manager broadcasts back.
func (f *ContextField[T]) Set(v T) {
	f.sig.value = v
	f.broadcast()
}

// ApplyExternal updates value and notifies subscribers without triggering broadcast.
// Used by generated context listeners and Set-all methods to avoid redundant events.
func (f *ContextField[T]) ApplyExternal(v T) {
	f.sig.value = v
	f.sig.notifyAll()
}

func (f *ContextField[T]) addEffect(e *Effect)    { f.sig.addEffect(e) }
func (f *ContextField[T]) removeEffect(e *Effect) { f.sig.removeEffect(e) }

// ── Cross-module context helpers ──────────────────────────────────────────────

// ReadCtxStore reads the encoded context value from the shared JS store.
func ReadCtxStore(keyName string) (string, bool) {
	v := ensureContextStore().Get(keyName)
	if v.IsUndefined() || v.IsNull() {
		return "", false
	}
	return v.String(), true
}

// BroadcastCtxEncoded writes encoded to the JS store and dispatches a CustomEvent.
func BroadcastCtxEncoded(keyName, encoded string) {
	ensureContextStore().Set(keyName, encoded)
	init := js.Global().Get("Object").New()
	init.Set("detail", encoded)
	event := js.Global().Get("CustomEvent").New("gothic:context:"+keyName, init)
	js.Global().Get("document").Call("dispatchEvent", event)
}

// ListenCtxEvent registers a cross-module listener for context updates.
func ListenCtxEvent(keyName string, fn func(string)) {
	listener := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) > 0 {
			detail := args[0].Get("detail")
			if !detail.IsUndefined() && !detail.IsNull() {
				fn(detail.String())
			}
		}
		return nil
	})
	keep = append(keep, listener)
	js.Global().Get("document").Call("addEventListener", "gothic:context:"+keyName, listener)
}

// RequestCtxSet dispatches a set-request to the context manager WASM for this key.
// The manager is the sole writer: it applies the update and broadcasts back.
func RequestCtxSet(keyName, encoded string) {
	init := js.Global().Get("Object").New()
	init.Set("detail", encoded)
	event := js.Global().Get("CustomEvent").New("gothic:ctx-req:"+keyName, init)
	js.Global().Get("document").Call("dispatchEvent", event)
}

// ListenCtxSetReq registers a handler for incoming set-requests on a context manager WASM.
func ListenCtxSetReq(keyName string, fn func(string)) {
	listener := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) > 0 {
			detail := args[0].Get("detail")
			if !detail.IsUndefined() && !detail.IsNull() {
				fn(detail.String())
			}
		}
		return nil
	})
	keep = append(keep, listener)
	js.Global().Get("document").Call("addEventListener", "gothic:ctx-req:"+keyName, listener)
}

// PingCtxManager dispatches a ping to the context manager asking for an online ack.
func PingCtxManager(keyName string) {
	init := js.Global().Get("Object").New()
	event := js.Global().Get("CustomEvent").New("gothic:ctx-ping:"+keyName, init)
	js.Global().Get("document").Call("dispatchEvent", event)
}

// ListenCtxOnline registers a handler that receives the manager's online ack with current state.
// Fires once on manager startup and on every ping response.
func ListenCtxOnline(keyName string, fn func(string)) {
	listener := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) > 0 {
			detail := args[0].Get("detail")
			if !detail.IsUndefined() && !detail.IsNull() {
				fn(detail.String())
			}
		}
		return nil
	})
	keep = append(keep, listener)
	js.Global().Get("document").Call("addEventListener", "gothic:ctx-online:"+keyName, listener)
}

// ListenCtxPing registers a handler for incoming pings on the context manager WASM.
func ListenCtxPing(keyName string, fn func()) {
	listener := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		fn()
		return nil
	})
	keep = append(keep, listener)
	js.Global().Get("document").Call("addEventListener", "gothic:ctx-ping:"+keyName, listener)
}

// BroadcastCtxOnline dispatches the online ack to all consumer WASMs for this key.
func BroadcastCtxOnline(keyName, encoded string) {
	init := js.Global().Get("Object").New()
	init.Set("detail", encoded)
	event := js.Global().Get("CustomEvent").New("gothic:ctx-online:"+keyName, init)
	js.Global().Get("document").Call("dispatchEvent", event)
}

// PingUntilOnline retries PingCtxManager every 50 ms until isOnline returns true.
// Runs in its own goroutine so it doesn't block the caller.
func PingUntilOnline(keyName string, isOnline func() bool) {
	go func() {
		for !isOnline() {
			PingCtxManager(keyName)
			time.Sleep(50 * time.Millisecond)
		}
	}()
}

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
			return HexEncode(e.Buf)
		},
		decode: func(s string) T {
			d := &Decoder{Buf: HexDecode(s)}
			return decode(d)
		},
	}
}
