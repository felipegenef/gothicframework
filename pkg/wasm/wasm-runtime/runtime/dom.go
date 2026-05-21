//go:build js && wasm

package runtime

import (
	"errors"
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

	onLoad := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		arrayBuffer := reader.Get("result")
		uint8Array := js.Global().Get("Uint8Array").New(arrayBuffer)
		data := make([]byte, uint8Array.Get("length").Int())
		js.CopyBytesToGo(data, uint8Array)
		ch <- result{data: data, ok: true}
		return nil
	})
	onError := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
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

	thenFn := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		resp := args[0]
		textThen := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			ch <- result{body: args[0].String()}
			return nil
		})
		textCatch := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			ch <- result{err: errors.New(args[0].String())}
			return nil
		})
		resp.Call("text").Call("then", textThen).Call("catch", textCatch)
		return nil
	})
	catchFn := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
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

	thenFn := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		resp := args[0]
		// Use arrayBuffer() instead of text() to avoid UTF-8 corruption
		bufThen := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			uint8Array := js.Global().Get("Uint8Array").New(args[0])
			data := make([]byte, uint8Array.Get("length").Int())
			js.CopyBytesToGo(data, uint8Array)
			ch <- result{data: data}
			return nil
		})
		bufCatch := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			ch <- result{err: errors.New(args[0].String())}
			return nil
		})
		resp.Call("arrayBuffer").Call("then", bufThen).Call("catch", bufCatch)
		return nil
	})
	catchFn := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		ch <- result{err: errors.New(args[0].String())}
		return nil
	})

	js.Global().Call("fetch", url, init).Call("then", thenFn).Call("catch", catchFn)

	r := <-ch
	return r.data, r.err
}
