//go:build js && wasm

package runtime

import (
	"strconv"
	"syscall/js"
	"time"
	"unsafe"
)

// _gothicKeyRegistry maps topic key names to their TopicKey (stored as any).
// Populated by generated init() calls in the compiled WASM main.

// TopicKey is a typed topic identifier that carries its own codec.
// T encodes the value type — provider and consumer must use the same key.
// Construct via the factory functions (IntKey, StringKey, BinaryKey, etc.),
// not as a struct literal.
type TopicKey[T any] struct {
	Name   string
	encode func(T) string
	decode func(string) T
}

// ── Primitive key factories ──────────────────────────────────────────────────
//
// All 16 primitive TopicKey factories share the same shape: build a
// TopicKey[T] with a strconv-based encode and a strconv-based decode. The
// helper `newPrimitiveKey` removes the boilerplate. Since the existing
// factories already returned generic types, TinyGo monomorphizes each
// instantiation just as it did before (no `interface{}`, no reflection).

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

// ── JS topic store ───────────────────────────────────────────────────────────

func ensureTopicStore() js.Value {
	store := js.Global().Get("__gothic_topic_store")
	if store.IsUndefined() {
		store = js.Global().Get("Object").New()
		js.Global().Set("__gothic_topic_store", store)
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
// memory offset to __gothic_topic.set (Bootstrap JS reads the bytes directly
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

// ── SharedTopicObservable ───────────────────────────────────────────────────────

// SharedTopicObservable is a reactive Observable bound to a shared topic key.
// Get/Set work like a regular Observable, but Set also broadcasts the new value
// to every other WASM module sharing the same key.
// Used internally by the auto-generated topic constructors (e.g. PageTopic()).
type SharedTopicObservable[T any] struct {
	inner *Observable[T]
	key   TopicKey[T]
}

func (s *SharedTopicObservable[T]) Get() T { return s.inner.Get() }

func (s *SharedTopicObservable[T]) Set(v T) {
	s.inner.value = v
	s.inner.notifyAll()
	encoded := s.key.encode(v)
	ensureTopicStore().Set(s.key.Name, encoded)
	dispatchDirect(s.key.Name, "gothic:topic:", []byte(encoded))
}

func (s *SharedTopicObservable[T]) addEffect(e *Subscription)    { s.inner.addEffect(e) }
func (s *SharedTopicObservable[T]) removeEffect(e *Subscription) { s.inner.removeEffect(e) }

// AutoKey is rewritten to BinaryKey by the CLI before TinyGo compiles.
// This stub exists so server-side code compiles; WASM code never calls it directly.
func AutoKey[T any](name string) TopicKey[T] { return TopicKey[T]{Name: name} }

// ── Per-field topic signals ───────────────────────────────────────────────────

// ObservableField is a reactive Observable bound to one field of a shared topic struct.
// It behaves like *Observable[T] but Set also broadcasts the full topic to other modules.
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

// Set sends a set-request to the topic manager WASM.
// The local value is silently updated so Peek() returns the correct value during
// encoding, but subscribers are NOT notified until the manager broadcasts back.
func (f *ObservableField[T]) Set(v T) {
	f.sig.value = v
	f.broadcast()
}

// ApplyExternal updates value and notifies subscribers without triggering broadcast.
// Used by generated topic listeners and Set-all methods to avoid redundant events.
func (f *ObservableField[T]) ApplyExternal(v T) {
	f.sig.value = v
	f.sig.notifyAll()
}

func (f *ObservableField[T]) addEffect(e *Subscription)    { f.sig.addEffect(e) }
func (f *ObservableField[T]) removeEffect(e *Subscription) { f.sig.removeEffect(e) }

// ── Cross-module topic helpers ────────────────────────────────────────────────

// ReadTopicStore reads the encoded topic value from the shared JS store.
func ReadTopicStore(keyName string) (string, bool) {
	v := ensureTopicStore().Get(keyName)
	if v.IsUndefined() || v.IsNull() {
		return "", false
	}
	return v.String(), true
}

// BroadcastTopicEncoded writes encoded to the JS store and dispatches a CustomEvent.
func BroadcastTopicEncoded(keyName, encoded string) {
	ensureTopicStore().Set(keyName, encoded)
	dispatchDirect(keyName, "gothic:topic:", []byte(encoded))
}

// ListenTopicEvent registers a cross-module listener for topic updates.
func ListenTopicEvent(keyName string, fn func(string)) {
	fullKey := "gothic:topic:" + keyName
	listener := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		data := js.Global().Get("__gothic_topic").Call("get", js.ValueOf(fullKey))
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

// RequestTopicSet dispatches a set-request to the topic manager WASM for this key.
// The manager is the sole writer: it applies the update and broadcasts back.
func RequestTopicSet(keyName, encoded string) {
	dispatchDirect(keyName, "gothic:topic-req:", []byte(encoded))
}

// ListenTopicSetReq registers a handler for incoming set-requests on a topic manager WASM.
func ListenTopicSetReq(keyName string, fn func(string)) {
	fullKey := "gothic:topic-req:" + keyName
	listener := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		data := js.Global().Get("__gothic_topic").Call("get", js.ValueOf(fullKey))
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

// BroadcastTopicEncodedField broadcasts an already-encoded single field value.
// Event name: "gothic:topic:<keyName>:<fieldName>"
func BroadcastTopicEncodedField(keyName, fieldName, encoded string) {
	dispatchDirect(keyName+":"+fieldName, "gothic:topic:", []byte(encoded))
}

// RequestTopicSetField sends a per-field set-request to the manager.
// Event name: "gothic:topic-req:<keyName>:<fieldName>"
func RequestTopicSetField(keyName, fieldName, encoded string) {
	dispatchDirect(keyName+":"+fieldName, "gothic:topic-req:", []byte(encoded))
}

// ListenTopicEventField subscribes to per-field broadcasts from the manager.
func ListenTopicEventField(keyName, fieldName string, fn func(string)) {
	fullKey := "gothic:topic:" + keyName + ":" + fieldName
	listener := js.FuncOf(func(_ js.Value, _ []js.Value) interface{} {
		data := js.Global().Get("__gothic_topic").Call("get", js.ValueOf(fullKey))
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

// ListenTopicSetReqField subscribes to per-field set-requests (used by the manager).
func ListenTopicSetReqField(keyName, fieldName string, fn func(string)) {
	fullKey := "gothic:topic-req:" + keyName + ":" + fieldName
	listener := js.FuncOf(func(_ js.Value, _ []js.Value) interface{} {
		data := js.Global().Get("__gothic_topic").Call("get", js.ValueOf(fullKey))
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

// PingTopicManager dispatches a ping to the topic manager asking for an online ack.
func PingTopicManager(keyName string) {
	evt, ok := pingEvents[keyName]
	if !ok {
		evt = js.Global().Get("CustomEvent").New("gothic:topic-ping:" + keyName)
		pingEvents[keyName] = evt
	}
	js.Global().Get("document").Call("dispatchEvent", evt)
}

// ListenTopicOnline registers a handler that receives the manager's online ack with current state.
// Fires once on manager startup and on every ping response.
func ListenTopicOnline(keyName string, fn func(string)) {
	fullKey := "gothic:topic-online:" + keyName
	listener := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		data := js.Global().Get("__gothic_topic").Call("get", js.ValueOf(fullKey))
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

// ListenTopicPing registers a handler for incoming pings on the topic manager WASM.
func ListenTopicPing(keyName string, fn func()) {
	listener := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		fn()
		return nil
	})
	keep = append(keep, listener)
	js.Global().Get("document").Call("addEventListener", "gothic:topic-ping:"+keyName, listener)
}

// BroadcastTopicOnline dispatches the online ack to all consumer WASMs for this key.
func BroadcastTopicOnline(keyName, encoded string) {
	ensureTopicStore().Set(keyName, encoded)
	dispatchDirect(keyName, "gothic:topic-online:", []byte(encoded))
}

// UpdateTopicOnlineStore updates the JS-side topic store so that late-joining
// consumers see fresh data via ReadTopicStore, WITHOUT dispatching the
// gothic:topic-online event. Use this from ListenTopicSetReq to fix the startup
// race (T5) without triggering ListenTopicOnline scans in already-running consumers.
func UpdateTopicOnlineStore(keyName string, encoded []byte) {
	ensureTopicStore().Set(keyName, string(encoded))
}

// PingUntilOnline retries PingTopicManager every 50 ms until isOnline returns true.
// Runs in its own goroutine so it doesn't block the caller.
func PingUntilOnline(keyName string, isOnline func() bool) {
	go func() {
		for !isOnline() {
			PingTopicManager(keyName)
			time.Sleep(50 * time.Millisecond)
		}
	}()
}

// CustomKey returns a TopicKey with user-supplied encode/decode functions.
func CustomKey[T any](name string, encode func(T) string, decode func(string) T) TopicKey[T] {
	return TopicKey[T]{Name: name, encode: encode, decode: decode}
}

// BinaryKey returns a TopicKey that serializes T using a compact little-endian binary
// codec. No reflection, no encoding/json — just typed Encoder/Decoder calls.
// The encode function writes fields onto e; the decode function reads them back and returns T.
// Field order must match between encode and decode.
//
// Example:
//
//	BinaryKey[Page]("page",
//	    func(v Page, e *Encoder) {
//	        e.I64(int64(v.Pings))
//	        e.String(v.Label)
//	        e.String(v.Theme)
//	    },
//	    func(d *Decoder) PageCtx {
//	        return PageCtx{Pings: int(d.I32()), Label: d.String(), Theme: d.String()}
//	    },
//	)
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

// Compression is the compression algorithm used for a topic's WASM payload.
type Compression int

const (
	GZIP   Compression = iota // default
	BROTLI Compression = iota
)

// TopicConfig holds per-topic configuration.
type TopicConfig struct {
	Name             string
	Compression      Compression // GZIP (default) or BROTLI
	SubscriberFnName string      // overrides generated accessor func name (default: <StructName>Topic)
	ComponentFnName  string      // overrides generated mount component func name (default: Add<StructName>Topic)
}

// CreateTopic declares a topic. The CLI AST scanner detects this call and
// generates the concrete typed accessor. At runtime this returns a no-op.
func CreateTopic[T any](zero T, cfg TopicConfig) func() interface{} {
	return func() interface{} { return nil }
}
