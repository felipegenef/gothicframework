//go:build js && wasm

package runtime

import (
	"errors"
	"strconv"
	"strings"
	"syscall/js"
)

// Scope-aware DOM helpers.
//
// The naive implementation calls document.getElementById(id), which returns
// the FIRST element with that id in document order. When the same component
// is rendered twice on a page (e.g. two PingMirror instances), both modules
// call SetText("pm-local-count", ...) and both writes land on instance #1's
// text node — instance #2's local counter appears frozen.
//
// Instead we route every per-id lookup through the calling module's own
// [data-gothic-scope="<id>"] container, found via the scope id Go reads at
// boot (see events.go). The wrapper is the element the bootstrap script
// stamps with data-gothic-scope, so a scoped querySelector cannot stray
// into a sibling component.
//
// Non-WASM callers and the rare full-page case where no bootstrap ran
// continue to work because scopeRoot() falls back to `document` when the
// module id is "_default".

var document = js.Global().Get("document")

// scopeRoot returns the DOM container Go should query inside. For a module
// loaded through the WASM bootstrap, this is the [data-gothic-scope="<id>"]
// element. For pure-Go / non-bootstrap contexts (moduleID == "_default")
// we fall back to the full document so behaviour outside the WASM path is
// unchanged.
//
// This is intentionally NOT memoised: the user may re-render the wrapper
// (HTMX swap, programmatic DOM replacement), so we resolve the root on
// every call. The cost is a single querySelector — negligible compared to
// the JS<->WASM bridge overhead the call already pays.
func scopeRoot() js.Value {
	id := moduleID()
	if id == "" || id == "_default" {
		return document
	}
	sel := `[data-gothic-scope="` + escapeAttr(id) + `"]`
	root := document.Call("querySelector", sel)
	if root.IsNull() || root.IsUndefined() {
		// Defensive fallback: if the scope wrapper was removed (e.g. an
		// HTMX swap killed the container before the module saw the event)
		// we degrade to document rather than silently no-op'ing every
		// helper. Calls land on the first id match — which is the best we
		// can do once our own wrapper has gone.
		return document
	}
	return root
}

// queryByIdInScope returns the first element with the given id INSIDE the
// calling module's scope. We use [id="..."] (attribute selector) instead of
// the `#id` form so ids containing colons or other CSS-special characters
// continue to work.
func queryByIdInScope(id string) js.Value {
	root := scopeRoot()
	if root.IsNull() || root.IsUndefined() {
		return js.Null()
	}
	sel := `[id="` + escapeAttr(id) + `"]`
	return root.Call("querySelector", sel)
}

// escapeAttr backslash-escapes characters that would break out of a
// double-quoted CSS attribute selector value. Component ids in the
// generated templates are alnum + `-` so this is defence-in-depth, but it
// keeps the helpers safe if a user-supplied id ever flows in.
func escapeAttr(s string) string {
	if !strings.ContainsAny(s, `"\`) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 4)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '"' || c == '\\' {
			b.WriteByte('\\')
		}
		b.WriteByte(c)
	}
	return b.String()
}

func SetText(id, value string) {
	el := queryByIdInScope(id)
	if el.IsNull() || el.IsUndefined() {
		return
	}
	el.Set("textContent", value)
}

func SetHTML(id, html string) {
	el := queryByIdInScope(id)
	if el.IsNull() || el.IsUndefined() {
		return
	}
	el.Set("innerHTML", html)
}

func SetValue(id, value string) {
	el := queryByIdInScope(id)
	if el.IsNull() || el.IsUndefined() {
		return
	}
	el.Set("value", value)
}

func GetValue(id string) string {
	el := queryByIdInScope(id)
	if el.IsNull() || el.IsUndefined() {
		return ""
	}
	return el.Get("value").String()
}

// GetFileBytes reads the contents of the first selected file from a <input type="file"> element.
// Blocks until the FileReader completes. Returns nil on error or if no file is selected.
func GetFileBytes(id string) []byte {
	el := queryByIdInScope(id)
	if el.IsNull() || el.IsUndefined() {
		return nil
	}
	files := el.Get("files")
	if files.IsNull() || files.IsUndefined() || files.Get("length").Int() == 0 {
		return nil
	}
	file := files.Index(0)

	type result struct {
		data []byte
		ok   bool
	}
	ch := make(chan result, 1)

	reader := js.Global().Get("FileReader").New()

	var onLoad, onError js.Func
	onLoad = js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		onLoad.Release()
		onError.Release()
		arrayBuffer := reader.Get("result")
		uint8Array := js.Global().Get("Uint8Array").New(arrayBuffer)
		data := make([]byte, uint8Array.Get("length").Int())
		js.CopyBytesToGo(data, uint8Array)
		ch <- result{data: data, ok: true}
		return nil
	})
	onError = js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		onLoad.Release()
		onError.Release()
		ch <- result{ok: false}
		return nil
	})

	reader.Set("onload", onLoad)
	reader.Set("onerror", onError)
	reader.Call("readAsArrayBuffer", file)

	r := <-ch
	if !r.ok {
		return nil
	}
	return r.data
}

func AddClass(id, className string) {
	el := queryByIdInScope(id)
	if el.IsNull() || el.IsUndefined() {
		return
	}
	el.Get("classList").Call("add", className)
}

func RemoveClass(id, className string) {
	el := queryByIdInScope(id)
	if el.IsNull() || el.IsUndefined() {
		return
	}
	el.Get("classList").Call("remove", className)
}

func ToggleClass(id, className string) {
	el := queryByIdInScope(id)
	if el.IsNull() || el.IsUndefined() {
		return
	}
	el.Get("classList").Call("toggle", className)
}

func SetAttr(id, attr, value string) {
	el := queryByIdInScope(id)
	if el.IsNull() || el.IsUndefined() {
		return
	}
	el.Call("setAttribute", attr, value)
}

func SetStyle(id, property, value string) {
	el := queryByIdInScope(id)
	if el.IsNull() || el.IsUndefined() {
		return
	}
	el.Get("style").Set(property, value)
}

// FetchConfig configures an HTTP request made via Fetch.
type FetchConfig struct {
	Method    string
	Headers   map[string]string
	Body      string
	BodyBytes []byte
	Query     map[string]string
}

// Fetch makes an HTTP request using the browser's fetch API and blocks until complete.
func Fetch(url string, config ...FetchConfig) (string, error) {
	var cfg FetchConfig
	if len(config) > 0 {
		cfg = config[0]
	}
	if cfg.Method == "" {
		cfg.Method = "GET"
	}

	// Build URL with query parameters
	if len(cfg.Query) > 0 {
		sep := "?"
		if strings.Contains(url, "?") {
			sep = "&"
		}
		for k, v := range cfg.Query {
			url += sep + js.Global().Get("encodeURIComponent").Invoke(k).String() +
				"=" + js.Global().Get("encodeURIComponent").Invoke(v).String()
			sep = "&"
		}
	}

	// Build fetch init object
	init := js.Global().Get("Object").New()
	init.Set("method", cfg.Method)

	if len(cfg.Headers) > 0 {
		headers := js.Global().Get("Object").New()
		for k, v := range cfg.Headers {
			headers.Set(k, v)
		}
		init.Set("headers", headers)
	}

	if cfg.Body != "" {
		init.Set("body", cfg.Body)
	} else if len(cfg.BodyBytes) > 0 {
		uint8Array := js.Global().Get("Uint8Array").New(len(cfg.BodyBytes))
		js.CopyBytesToJS(uint8Array, cfg.BodyBytes)
		init.Set("body", uint8Array)
	}

	type result struct {
		body string
		err  error
	}
	ch := make(chan result, 1)

	var thenFn, catchFn js.Func
	thenFn = js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		thenFn.Release()
		catchFn.Release()
		resp := args[0]
		var textThen, textCatch js.Func
		textThen = js.FuncOf(func(_ js.Value, a []js.Value) interface{} {
			textThen.Release()
			textCatch.Release()
			ch <- result{body: a[0].String()}
			return nil
		})
		textCatch = js.FuncOf(func(_ js.Value, a []js.Value) interface{} {
			textThen.Release()
			textCatch.Release()
			ch <- result{err: errors.New(a[0].String())}
			return nil
		})
		resp.Call("text").Call("then", textThen).Call("catch", textCatch)
		return nil
	})
	catchFn = js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		thenFn.Release()
		catchFn.Release()
		ch <- result{err: errors.New(args[0].String())}
		return nil
	})

	js.Global().Call("fetch", url, init).Call("then", thenFn).Call("catch", catchFn)

	r := <-ch
	return r.body, r.err
}

// FetchBytes makes an HTTP request and returns the response as raw bytes.
// Use for binary responses (images, PDFs, ZIPs) where Fetch's text() decoding would corrupt data.
func FetchBytes(url string, config ...FetchConfig) ([]byte, error) {
	var cfg FetchConfig
	if len(config) > 0 {
		cfg = config[0]
	}
	if cfg.Method == "" {
		cfg.Method = "GET"
	}

	// Build URL with query parameters (identical to Fetch)
	if len(cfg.Query) > 0 {
		sep := "?"
		if strings.Contains(url, "?") {
			sep = "&"
		}
		for k, v := range cfg.Query {
			url += sep + js.Global().Get("encodeURIComponent").Invoke(k).String() +
				"=" + js.Global().Get("encodeURIComponent").Invoke(v).String()
			sep = "&"
		}
	}

	// Build fetch init object (identical to Fetch)
	init := js.Global().Get("Object").New()
	init.Set("method", cfg.Method)
	if len(cfg.Headers) > 0 {
		headers := js.Global().Get("Object").New()
		for k, v := range cfg.Headers {
			headers.Set(k, v)
		}
		init.Set("headers", headers)
	}
	if cfg.Body != "" {
		init.Set("body", cfg.Body)
	} else if len(cfg.BodyBytes) > 0 {
		uint8Array := js.Global().Get("Uint8Array").New(len(cfg.BodyBytes))
		js.CopyBytesToJS(uint8Array, cfg.BodyBytes)
		init.Set("body", uint8Array)
	}

	type result struct {
		data []byte
		err  error
	}
	ch := make(chan result, 1)

	var thenFn, catchFn js.Func
	thenFn = js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		thenFn.Release()
		catchFn.Release()
		resp := args[0]
		// Use arrayBuffer() instead of text() to avoid UTF-8 corruption
		var bufThen, bufCatch js.Func
		bufThen = js.FuncOf(func(_ js.Value, a []js.Value) interface{} {
			bufThen.Release()
			bufCatch.Release()
			uint8Array := js.Global().Get("Uint8Array").New(a[0])
			data := make([]byte, uint8Array.Get("length").Int())
			js.CopyBytesToGo(data, uint8Array)
			ch <- result{data: data}
			return nil
		})
		bufCatch = js.FuncOf(func(_ js.Value, a []js.Value) interface{} {
			bufThen.Release()
			bufCatch.Release()
			ch <- result{err: errors.New(a[0].String())}
			return nil
		})
		resp.Call("arrayBuffer").Call("then", bufThen).Call("catch", bufCatch)
		return nil
	})
	catchFn = js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		thenFn.Release()
		catchFn.Release()
		ch <- result{err: errors.New(args[0].String())}
		return nil
	})

	js.Global().Call("fetch", url, init).Call("then", thenFn).Call("catch", catchFn)

	r := <-ch
	return r.data, r.err
}

// JSValue wraps js.Value. Never create as a literal; use JS(), Document(), etc.
type JSValue struct{ v js.Value }

func JS() JSValue       { return JSValue{js.Global()} }
func Window() JSValue   { return JS() }
func Document() JSValue { return JSValue{js.Global().Get("document")} }

func ConsoleLog(args ...any) { js.Global().Get("console").Call("log", toJSArgs(args)...) }

func GetElementById(id string) JSValue    { return JSValue{js.Global().Get("document").Call("getElementById", id)} }
func CreateElement(tag string) JSValue    { return JSValue{js.Global().Get("document").Call("createElement", tag)} }
func QuerySelector(sel string) JSValue    { return JSValue{js.Global().Get("document").Call("querySelector", sel)} }
func QuerySelectorAll(sel string) JSValue { return JSValue{js.Global().Get("document").Call("querySelectorAll", sel)} }

func (v JSValue) Get(key string) JSValue                  { return JSValue{v.v.Get(key)} }
func (v JSValue) Set(key string, val any)                 { v.v.Set(key, toJSVal(val)) }
func (v JSValue) Call(method string, args ...any) JSValue { return JSValue{v.v.Call(method, toJSArgs(args)...)} }
func (v JSValue) New(args ...any) JSValue                 { return JSValue{v.v.New(toJSArgs(args)...)} }
func (v JSValue) String() string                          { return v.v.String() }
func (v JSValue) Int() int                                { return v.v.Int() }
func (v JSValue) Float() float64                          { return v.v.Float() }
func (v JSValue) Bool() bool                              { return v.v.Bool() }
func (v JSValue) IsNull() bool                            { return v.v.IsNull() }
func (v JSValue) IsUndefined() bool                       { return v.v.IsUndefined() }
func (v JSValue) Truthy() bool                            { return v.v.Truthy() }
func (v JSValue) Index(i int) JSValue                     { return JSValue{v.v.Index(i)} }
func (v JSValue) SetIndex(i int, val any)                 { v.v.SetIndex(i, toJSVal(val)) }
func (v JSValue) Length() int                             { return v.v.Length() }

func CopyBytesToJS(dst JSValue, src []byte) int { return js.CopyBytesToJS(dst.v, src) }
func CopyBytesToGo(dst []byte, src JSValue) int { return js.CopyBytesToGo(dst, src.v) }

func toJSVal(v any) any {
	switch x := v.(type) {
	case JSValue:
		return x.v
	default:
		return js.ValueOf(x)
	}
}

func toJSArgs(args []any) []any {
	out := make([]any, len(args))
	for i, a := range args {
		out[i] = toJSVal(a)
	}
	return out
}

// TriggerDownload prompts the browser to download `data` as a file named `filename` with the given MIME type.
func TriggerDownload(filename string, data []byte, mimeType string) {
	uint8Array := js.Global().Get("Uint8Array").New(len(data))
	js.CopyBytesToJS(uint8Array, data)
	blobParts := js.Global().Get("Array").New(uint8Array)
	opts := js.Global().Get("Object").New()
	opts.Set("type", mimeType)
	blob := js.Global().Get("Blob").New(blobParts, opts)
	url := js.Global().Get("URL").Call("createObjectURL", blob)
	a := js.Global().Get("document").Call("createElement", "a")
	a.Set("href", url)
	a.Set("download", filename)
	js.Global().Get("document").Get("body").Call("appendChild", a)
	a.Call("click")
	js.Global().Get("document").Get("body").Call("removeChild", a)
	js.Global().Get("URL").Call("revokeObjectURL", url)
}

// Element tree helpers.
func AppendChild(parent, child JSValue) { parent.v.Call("appendChild", child.v) }
func RemoveElement(el JSValue)          { el.v.Call("remove") }
func ClickElement(el JSValue)           { el.v.Call("click") }

// WriteClipboard writes the given text to the system clipboard via navigator.clipboard.writeText.
func WriteClipboard(text string) {
	js.Global().Get("navigator").Get("clipboard").Call("writeText", text)
}

// ExecJS executes the given script string in the global scope.
func ExecJS(script string) {
	js.Global().Call("eval", script)
}

// Navigation helpers.
func Navigate(url string)         { js.Global().Get("location").Set("href", url) }
func Reload()                     { js.Global().Get("location").Call("reload") }
func PushState(url, title string) { js.Global().Get("history").Call("pushState", js.Null(), title, url) }
func GoBack()                     { js.Global().Get("history").Call("back") }

// LocalStorage helpers
func LocalStorageSet(key, value string) {
	js.Global().Get("localStorage").Call("setItem", key, value)
}

func LocalStorageGet(key string) string {
	v := js.Global().Get("localStorage").Call("getItem", key)
	if v.IsNull() || v.IsUndefined() {
		return ""
	}
	return v.String()
}

func LocalStorageRemove(key string) {
	js.Global().Get("localStorage").Call("removeItem", key)
}

// SessionStorage helpers
func SessionStorageSet(key, value string) {
	js.Global().Get("sessionStorage").Call("setItem", key, value)
}

func SessionStorageGet(key string) string {
	v := js.Global().Get("sessionStorage").Call("getItem", key)
	if v.IsNull() || v.IsUndefined() {
		return ""
	}
	return v.String()
}

func SessionStorageRemove(key string) {
	js.Global().Get("sessionStorage").Call("removeItem", key)
}

// CookieOptions configures CookieSet behaviour.
type CookieOptions struct {
	MaxAge   int    // seconds; 0 = session cookie
	Path     string // defaults to "/"
	SameSite string // "Strict", "Lax", or "None"
	Secure   bool
}

// CookieSet writes a cookie to document.cookie.
func CookieSet(key, value string, opts ...CookieOptions) {
	path := "/"
	maxAge := 0
	sameSite := ""
	secure := false
	if len(opts) > 0 {
		o := opts[0]
		if o.Path != "" {
			path = o.Path
		}
		maxAge = o.MaxAge
		sameSite = o.SameSite
		secure = o.Secure
	}
	cookie := key + "=" + value + "; Path=" + path
	if maxAge != 0 {
		cookie += "; Max-Age=" + strconv.Itoa(maxAge)
	}
	if sameSite != "" {
		cookie += "; SameSite=" + sameSite
	}
	if secure {
		cookie += "; Secure"
	}
	js.Global().Get("document").Set("cookie", cookie)
}

// CookieGet reads a cookie value from document.cookie.
// Returns "" for missing or HttpOnly cookies.
func CookieGet(key string) string {
	raw := js.Global().Get("document").Get("cookie").String()
	for _, part := range strings.Split(raw, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, key+"=") {
			return part[len(key)+1:]
		}
	}
	return ""
}

// CookieDelete expires a cookie immediately.
func CookieDelete(key string) {
	CookieSet(key, "", CookieOptions{MaxAge: -1})
}
