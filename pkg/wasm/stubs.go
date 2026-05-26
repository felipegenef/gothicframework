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

// GetFileBytes reads the contents of the first file selected in a <input type="file"> element.
// Returns nil if the element is not found, no file is selected, or reading fails.
func GetFileBytes(id string) []byte { return nil }

func AddClass(id, className string)       {}
func RemoveClass(id, className string)    {}
func ToggleClass(id, className string)    {}
func SetAttr(id, attr, value string)      {}
func SetStyle(id, property, value string) {}

// FetchConfig configures an HTTP request made via Fetch.
type FetchConfig struct {
	Method    string            // "GET", "POST", "PUT", "DELETE" — default: "GET"
	Headers   map[string]string // request headers
	Body      string            // request body (for POST/PUT) — text body
	BodyBytes []byte            // binary body — used when Body is empty
	Query     map[string]string // query parameters appended to the URL
}

// Fetch makes an HTTP request using the browser's fetch API and blocks until complete.
// Config is optional — omit for a simple GET request.
// Must be called from inside a goroutine or CreateWasmFunc handler.
//
// Example:
//
//	body, err := Fetch("https://api.example.com/todos/1")
//
//	body, err := Fetch("https://api.example.com/todos", FetchConfig{
//	    Method:  "POST",
//	    Headers: map[string]string{"Content-Type": "application/json"},
//	    Body:    `{"title":"foo"}`,
//	})
func Fetch(url string, config ...FetchConfig) (string, error) { return "", nil }

// FetchBytes makes an HTTP request and returns the response as raw bytes.
// Use this instead of Fetch when the response is binary (images, PDFs, ZIPs, etc.).
// Config is optional — omit for a simple GET.
// Must be called from inside a goroutine or CreateWasmFunc handler.
func FetchBytes(url string, config ...FetchConfig) ([]byte, error) { return nil, nil }

// JSValue is a server-side stub for syscall/js.Value.
// All methods are no-ops; the real implementation lives in the WASM runtime.
type JSValue struct{}

func JS() JSValue       { return JSValue{} }
func Window() JSValue   { return JSValue{} }
func Document() JSValue { return JSValue{} }

func ConsoleLog(args ...any) {}

func GetElementById(id string) JSValue    { return JSValue{} }
func CreateElement(tag string) JSValue    { return JSValue{} }
func QuerySelector(sel string) JSValue    { return JSValue{} }
func QuerySelectorAll(sel string) JSValue { return JSValue{} }

func (v JSValue) Get(key string) JSValue                  { return JSValue{} }
func (v JSValue) Set(key string, val any)                 {}
func (v JSValue) Call(method string, args ...any) JSValue { return JSValue{} }
func (v JSValue) New(args ...any) JSValue                 { return JSValue{} }
func (v JSValue) String() string                          { return "" }
func (v JSValue) Int() int                                { return 0 }
func (v JSValue) Float() float64                          { return 0 }
func (v JSValue) Bool() bool                              { return false }
func (v JSValue) IsNull() bool                            { return true }
func (v JSValue) IsUndefined() bool                       { return true }
func (v JSValue) Truthy() bool                            { return false }
func (v JSValue) Index(i int) JSValue                     { return JSValue{} }
func (v JSValue) SetIndex(i int, val any)                 {}
func (v JSValue) Length() int                             { return 0 }

func CopyBytesToJS(dst JSValue, src []byte) int { return 0 }
func CopyBytesToGo(dst []byte, src JSValue) int { return 0 }

// TriggerDownload prompts the browser to download `data` as a file named `filename` with the given MIME type.
// Server-side no-op.
func TriggerDownload(filename string, data []byte, mimeType string) {}

// AddEventListenerWithEvent attaches a persistent event listener to el for the given event name.
// fn receives the browser Event object as a JSValue, giving access to event properties and methods.
// Use this when you need to inspect or interact with the event itself — call preventDefault,
// read event.target, event.key, event.clientX, event.detail, etc.
// The listener stays alive for the lifetime of the page — it is never removed automatically.
//
// Example:
//
//	AddEventListenerWithEvent(form, "submit", func(e JSValue) {
//	    e.Call("preventDefault")               // stop the default form submission
//	    val := e.Get("target").Get("value").String()
//	})
//
//	AddEventListenerWithEvent(Document(), "keydown", func(e JSValue) {
//	    if e.Get("key").String() == "Escape" {
//	        // close modal, etc.
//	    }
//	})
func AddEventListenerWithEvent(el JSValue, event string, fn func(JSValue)) {}

// AddEventListener attaches a persistent event listener to el for the given event name.
// fn is called with no arguments each time the event fires.
// The listener stays alive for the lifetime of the page — it is never removed automatically.
//
// Common use cases: reacting to browser events (click, input, toggle) or framework
// events (htmx:afterSwap, htmx:beforeSwap) on any JSValue element including Document()
// and Window().
//
// Example:
//
//	body := Document().Get("body")
//	AddEventListener(body, "htmx:afterSwap", func() {
//	    // re-sync DOM after HTMX swaps content
//	})
//
//	details := QuerySelector("details#menu")
//	AddEventListener(details, "toggle", func() {
//	    // react to open/close state changes
//	})
func AddEventListener(el JSValue, event string, fn func()) {}

// Element tree helpers — server-side no-ops.
func AppendChild(parent, child JSValue) {}
func RemoveElement(el JSValue)          {}
func ClickElement(el JSValue)           {}

// WriteClipboard writes text to the system clipboard. Server-side no-op.
func WriteClipboard(text string) {}

// ExecJS executes a JavaScript snippet in the browser. Server-side no-op.
func ExecJS(script string) {}

// Navigation helpers — server-side no-ops.
func Navigate(url string)         {}
func Reload()                     {}
func PushState(url, title string) {}
func GoBack()                     {}

// Event registration — no-ops on the server.

func CreateWasmFunc(name string, fn func())            {}
func CreateWasmStringFunc(name string, fn func(string)) {}
func CreateWasmBoolFunc(name string, fn func(bool))    {}

// ── Topic infrastructure (generated code only — not part of the user API) ────

// TopicKey is a typed key used by the auto-generated topic system.
// Users never construct these directly — the CLI generates them from src/topics/*.go.
type TopicKey[T any] struct {
	Name   string
	encode func(T) string
	decode func(string) T
}

// BinaryKey is used exclusively by CLI-generated code in src/topics/topic_gen.go.
func BinaryKey[T any](name string, encode func(T, *Encoder), decode func(*Decoder) T) TopicKey[T] {
	return TopicKey[T]{
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
func AutoKey[T any](name string) TopicKey[T] { return TopicKey[T]{Name: name} }

// Compression is the compression algorithm used for a topic's WASM payload.
type Compression int

const (
	GZIP   Compression = iota // default
	BROTLI Compression = iota
)

// TopicConfig holds per-topic configuration. The CLI AST scanner reads the
// Name and Compression fields from CreateTopic call sites to drive code
// generation.
type TopicConfig struct {
	Name             string
	Compression      Compression // GZIP (default) or BROTLI
	SubscriberFnName string      // overrides generated accessor func name (default: <StructName>Topic)
	ComponentFnName  string      // overrides generated mount component func name (default: Add<StructName>Topic)
}

// CreateTopic declares a topic. The CLI AST scanner detects this call and
// generates the concrete typed accessor. Server-side this is a no-op stub.
func CreateTopic[T any](zero T, cfg TopicConfig) func() interface{} {
	return func() interface{} { return nil }
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

// SharedTopicObservable is the internal type backing auto-generated topic constructors.
// Users access shared topic state via the generated accessor e.g. PageTopic() — not directly.
type SharedTopicObservable[T any] struct{ value T }

func (s *SharedTopicObservable[T]) Get() T  { return s.value }
func (s *SharedTopicObservable[T]) Set(v T) { s.value = v }

// ObservableField is a per-field reactive observable for a generated topic struct.
// Server-side stub — no broadcast, no effect tracking.
type ObservableField[T any] struct{ sig *Observable[T] }

// NewObservableField creates an ObservableField with the given initial value.
func NewObservableField[T any](initial T) *ObservableField[T] {
	return &ObservableField[T]{sig: &Observable[T]{value: initial}}
}
func (f *ObservableField[T]) SetBroadcast(fn func()) {}
func (f *ObservableField[T]) Get() T                 { return f.sig.Get() }
func (f *ObservableField[T]) Peek() T                { return f.sig.value }
func (f *ObservableField[T]) Set(v T)                { f.sig.value = v }
func (f *ObservableField[T]) ApplyExternal(v T)      { f.sig.Set(v) }

// LocalStorage helpers — server-side no-ops.
func LocalStorageSet(key, value string) {}
func LocalStorageGet(key string) string { return "" }
func LocalStorageRemove(key string)     {}

// SessionStorage helpers — server-side no-ops.
func SessionStorageSet(key, value string) {}
func SessionStorageGet(key string) string { return "" }
func SessionStorageRemove(key string)     {}

// CookieOptions configures CookieSet behaviour.
type CookieOptions struct {
	MaxAge   int    // seconds; 0 = session cookie
	Path     string // defaults to "/"
	SameSite string // "Strict", "Lax", or "None"
	Secure   bool
}

// Cookie helpers — server-side no-ops.
func CookieSet(key, value string, opts ...CookieOptions) {}
func CookieGet(key string) string                        { return "" }
func CookieDelete(key string)                            {}

