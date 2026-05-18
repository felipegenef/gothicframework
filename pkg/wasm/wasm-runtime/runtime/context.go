//go:build js && wasm

package runtime

import (
	"encoding/json"
	"strconv"
	"syscall/js"
	"time"
	"unsafe"
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
//
// All 16 primitive ContextKey factories share the same shape: build a
// ContextKey[T] with a strconv-based encode and a strconv-based decode. The
// helper `newPrimitiveKey` removes the boilerplate. Since the existing
// factories already returned generic types, TinyGo monomorphizes each
// instantiation just as it did before (no `interface{}`, no reflection).

// newPrimitiveKey builds a ContextKey[T] for a primitive value using the
// provided strconv-based encode and decode functions.
func newPrimitiveKey[T any](name string, encode func(T) string, decode func(string) T) ContextKey[T] {
	return ContextKey[T]{Name: name, encode: encode, decode: decode}
}

func BoolKey(name string) ContextKey[bool] {
	return newPrimitiveKey(name,
		strconv.FormatBool,
		func(s string) bool { b, _ := strconv.ParseBool(s); return b },
	)
}

func StringKey(name string) ContextKey[string] {
	return newPrimitiveKey(name,
		func(s string) string { return s },
		func(s string) string { return s },
	)
}

func IntKey(name string) ContextKey[int] {
	return newPrimitiveKey(name,
		strconv.Itoa,
		func(s string) int { n, _ := strconv.Atoi(s); return n },
	)
}

func Int8Key(name string) ContextKey[int8] {
	return newPrimitiveKey(name,
		func(v int8) string { return strconv.FormatInt(int64(v), 10) },
		func(s string) int8 { n, _ := strconv.ParseInt(s, 10, 8); return int8(n) },
	)
}

func Int16Key(name string) ContextKey[int16] {
	return newPrimitiveKey(name,
		func(v int16) string { return strconv.FormatInt(int64(v), 10) },
		func(s string) int16 { n, _ := strconv.ParseInt(s, 10, 16); return int16(n) },
	)
}

func Int32Key(name string) ContextKey[int32] {
	return newPrimitiveKey(name,
		func(v int32) string { return strconv.FormatInt(int64(v), 10) },
		func(s string) int32 { n, _ := strconv.ParseInt(s, 10, 32); return int32(n) },
	)
}

func Int64Key(name string) ContextKey[int64] {
	return newPrimitiveKey(name,
		func(v int64) string { return strconv.FormatInt(v, 10) },
		func(s string) int64 { n, _ := strconv.ParseInt(s, 10, 64); return n },
	)
}

func UintKey(name string) ContextKey[uint] {
	return newPrimitiveKey(name,
		func(v uint) string { return strconv.FormatUint(uint64(v), 10) },
		func(s string) uint { n, _ := strconv.ParseUint(s, 10, 64); return uint(n) },
	)
}

func Uint8Key(name string) ContextKey[uint8] {
	return newPrimitiveKey(name,
		func(v uint8) string { return strconv.FormatUint(uint64(v), 10) },
		func(s string) uint8 { n, _ := strconv.ParseUint(s, 10, 8); return uint8(n) },
	)
}

func Uint16Key(name string) ContextKey[uint16] {
	return newPrimitiveKey(name,
		func(v uint16) string { return strconv.FormatUint(uint64(v), 10) },
		func(s string) uint16 { n, _ := strconv.ParseUint(s, 10, 16); return uint16(n) },
	)
}

func Uint32Key(name string) ContextKey[uint32] {
	return newPrimitiveKey(name,
		func(v uint32) string { return strconv.FormatUint(uint64(v), 10) },
		func(s string) uint32 { n, _ := strconv.ParseUint(s, 10, 32); return uint32(n) },
	)
}

func Uint64Key(name string) ContextKey[uint64] {
	return newPrimitiveKey(name,
		func(v uint64) string { return strconv.FormatUint(v, 10) },
		func(s string) uint64 { n, _ := strconv.ParseUint(s, 10, 64); return n },
	)
}

func Float32Key(name string) ContextKey[float32] {
	return newPrimitiveKey(name,
		func(v float32) string { return strconv.FormatFloat(float64(v), 'f', -1, 32) },
		func(s string) float32 { f, _ := strconv.ParseFloat(s, 32); return float32(f) },
	)
}

func Float64Key(name string) ContextKey[float64] {
	return newPrimitiveKey(name,
		func(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) },
		func(s string) float64 { f, _ := strconv.ParseFloat(s, 64); return f },
	)
}

// RuneKey is IntKey for rune (= int32).
func RuneKey(name string) ContextKey[rune] {
	return newPrimitiveKey(name,
		func(v rune) string { return strconv.FormatInt(int64(v), 10) },
		func(s string) rune { n, _ := strconv.ParseInt(s, 10, 32); return rune(n) },
	)
}

// ByteKey is UintKey for byte (= uint8).
func ByteKey(name string) ContextKey[byte] {
	return newPrimitiveKey(name,
		func(v byte) string { return strconv.FormatUint(uint64(v), 10) },
		func(s string) byte { n, _ := strconv.ParseUint(s, 10, 8); return byte(n) },
	)
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

// ── Direct-memory payload dispatch ──────────────────────────────────────────

// dispatchHold keeps each key's payload slice alive until the next dispatch on
// the same key overwrites it. The queueMicrotask callback in Gothic's bootstrap
// JS reads directly from WASM linear memory via the raw pointer; the slice must
// not be GC'd before that microtask fires.
var dispatchHold = map[string][]byte{}

// dispatchDirect writes encoded into a Go-owned buffer, passes the raw WASM
// memory offset to __gothic_ctx.set (Bootstrap JS reads the bytes directly
// from instance.exports.memory.buffer — no js.CopyBytesToJS, no Uint8Array
// allocation, no _values[] entry for the payload), then fires an async event
// so document listeners still receive it.
func dispatchDirect(keyName, eventPrefix string, encoded []byte) {
	buf := make([]byte, len(encoded))
	copy(buf, encoded)
	dispatchHold[eventPrefix+keyName] = buf

	ptr := int32(uintptr(unsafe.Pointer(unsafe.SliceData(buf))))
	js.Global().Get("__gothic_set").Get(moduleID()).Invoke(
		js.ValueOf(eventPrefix+keyName),
		js.ValueOf(ptr),
		js.ValueOf(len(buf)),
	)
	js.Global().Call("__gothicDispatchAsync", js.ValueOf(eventPrefix+keyName))
}

// ── SharedCtxObservable ───────────────────────────────────────────────────────

// SharedCtxObservable is a reactive Observable bound to a shared context key.
// Get/Set work like a regular Observable, but Set also broadcasts the new value
// to every other WASM module sharing the same key.
// Used internally by the auto-generated context constructors (e.g. PageCtxContext()).
type SharedCtxObservable[T any] struct {
	inner *Observable[T]
	key   ContextKey[T]
}

func (s *SharedCtxObservable[T]) Get() T { return s.inner.Get() }

func (s *SharedCtxObservable[T]) Set(v T) {
	s.inner.value = v
	s.inner.notifyAll()
	encoded := s.key.encode(v)
	ensureContextStore().Set(s.key.Name, encoded)
	dispatchDirect(s.key.Name, "gothic:context:", []byte(encoded))
}

func (s *SharedCtxObservable[T]) addEffect(e *Subscription)    { s.inner.addEffect(e) }
func (s *SharedCtxObservable[T]) removeEffect(e *Subscription) { s.inner.removeEffect(e) }

// AutoKey is rewritten to BinaryKey by the CLI before TinyGo compiles.
// This stub exists so server-side code compiles; WASM code never calls it directly.
func AutoKey[T any](name string) ContextKey[T] { return ContextKey[T]{Name: name} }

// ── Per-field context signals ─────────────────────────────────────────────────

// ObservableField is a reactive Observable bound to one field of a shared context struct.
// It behaves like *Observable[T] but Set also broadcasts the full context to other modules.
// Pass *ObservableField as a dep in Observe to react to individual property changes.
type ObservableField[T any] struct {
	sig       *Observable[T]
	broadcast func()
}

// NewObservableField creates an ObservableField with the given initial value.
func NewObservableField[T any](initial T) *ObservableField[T] {
	return &ObservableField[T]{sig: &Observable[T]{value: initial}}
}

// SetBroadcast wires the broadcast callback called whenever Set updates this field.
func (f *ObservableField[T]) SetBroadcast(fn func()) { f.broadcast = fn }

// Get returns the current value, auto-registering as a dep of any running effect.
func (f *ObservableField[T]) Get() T { return f.sig.Get() }

// Peek returns the current value without registering as an effect dependency.
// Used internally by broadcast closures to read sibling field values safely.
func (f *ObservableField[T]) Peek() T { return f.sig.value }

// Set sends a set-request to the context manager WASM.
// The local value is silently updated so Peek() returns the correct value during
// encoding, but subscribers are NOT notified until the manager broadcasts back.
func (f *ObservableField[T]) Set(v T) {
	f.sig.value = v
	f.broadcast()
}

// ApplyExternal updates value and notifies subscribers without triggering broadcast.
// Used by generated context listeners and Set-all methods to avoid redundant events.
func (f *ObservableField[T]) ApplyExternal(v T) {
	f.sig.value = v
	f.sig.notifyAll()
}

func (f *ObservableField[T]) addEffect(e *Subscription)    { f.sig.addEffect(e) }
func (f *ObservableField[T]) removeEffect(e *Subscription) { f.sig.removeEffect(e) }

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
	dispatchDirect(keyName, "gothic:context:", []byte(encoded))
}

// ListenCtxEvent registers a cross-module listener for context updates.
func ListenCtxEvent(keyName string, fn func(string)) {
	fullKey := "gothic:context:" + keyName
	listener := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		data := js.Global().Get("__gothic_ctx").Call("get", js.ValueOf(fullKey))
		if data.IsNull() || data.IsUndefined() {
			return nil
		}
		n := data.Get("byteLength").Int()
		if n == 0 {
			return nil
		}
		dst := make([]byte, n)
		js.CopyBytesToGo(dst, data)
		fn(string(dst))
		return nil
	})
	keep = append(keep, listener)
	js.Global().Get("document").Call("addEventListener", fullKey, listener)
}

// RequestCtxSet dispatches a set-request to the context manager WASM for this key.
// The manager is the sole writer: it applies the update and broadcasts back.
func RequestCtxSet(keyName, encoded string) {
	dispatchDirect(keyName, "gothic:ctx-req:", []byte(encoded))
}

// ListenCtxSetReq registers a handler for incoming set-requests on a context manager WASM.
func ListenCtxSetReq(keyName string, fn func(string)) {
	fullKey := "gothic:ctx-req:" + keyName
	listener := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		data := js.Global().Get("__gothic_ctx").Call("get", js.ValueOf(fullKey))
		if data.IsNull() || data.IsUndefined() {
			return nil
		}
		n := data.Get("byteLength").Int()
		if n == 0 {
			return nil
		}
		dst := make([]byte, n)
		js.CopyBytesToGo(dst, data)
		fn(string(dst))
		return nil
	})
	keep = append(keep, listener)
	js.Global().Get("document").Call("addEventListener", fullKey, listener)
}

// pingEvents caches one CustomEvent JS object per keyName so we don't allocate
// a new JS value (and a permanent TinyGo bridge slot) on every ping.
var pingEvents = map[string]js.Value{}

// PingCtxManager dispatches a ping to the context manager asking for an online ack.
func PingCtxManager(keyName string) {
	evt, ok := pingEvents[keyName]
	if !ok {
		evt = js.Global().Get("CustomEvent").New("gothic:ctx-ping:" + keyName)
		pingEvents[keyName] = evt
	}
	js.Global().Get("document").Call("dispatchEvent", evt)
}

// ListenCtxOnline registers a handler that receives the manager's online ack with current state.
// Fires once on manager startup and on every ping response.
func ListenCtxOnline(keyName string, fn func(string)) {
	fullKey := "gothic:ctx-online:" + keyName
	listener := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		data := js.Global().Get("__gothic_ctx").Call("get", js.ValueOf(fullKey))
		if data.IsNull() || data.IsUndefined() {
			return nil
		}
		n := data.Get("byteLength").Int()
		if n == 0 {
			return nil
		}
		dst := make([]byte, n)
		js.CopyBytesToGo(dst, data)
		fn(string(dst))
		return nil
	})
	keep = append(keep, listener)
	js.Global().Get("document").Call("addEventListener", fullKey, listener)
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
	ensureContextStore().Set(keyName, encoded)
	dispatchDirect(keyName, "gothic:ctx-online:", []byte(encoded))
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
