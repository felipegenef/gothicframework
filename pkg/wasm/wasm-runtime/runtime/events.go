//go:build js && wasm

package runtime

import (
	"os"
	"strings"
	"syscall/js"
)

var keep []js.Func
var jsTrue = js.ValueOf(true)

// cachedModuleID is captured once at package init. Two channels are checked:
//
//  1. Go.argv — the deterministic channel. The bootstrap script in
//     pkg/helpers/routes/wasm_bootstrap.go sets
//     go.argv = ['gothic', 'GOTHIC_SCOPE=<id>'] BEFORE go.run, and
//     TinyGo's wasm_exec populates os.Args from argv synchronously, so this
//     read is race-free even when several bootstraps run concurrently.
//
//  2. window.__gothicCurrentModule — legacy global, kept as a fallback for
//     anyone hand-rolling a bootstrap or running the runtime outside the
//     standard envelope. Susceptible to a multi-IIFE race when multiple
//     bootstraps interleave, so the argv channel takes precedence.
//
// If neither channel yields a value the runtime falls back to "_default"
// and DOM helpers behave document-wide. Tests and non-WASM callers rely on
// this fallback.
var cachedModuleID = func() string {
	for _, arg := range os.Args {
		if strings.HasPrefix(arg, "GOTHIC_SCOPE=") {
			return arg[len("GOTHIC_SCOPE="):]
		}
	}
	v := js.Global().Get("__gothicCurrentModule")
	if v.IsUndefined() || v.IsNull() {
		return "_default"
	}
	return v.String()
}()

func moduleID() string {
	return cachedModuleID
}

func ensureRegistry() js.Value {
	reg := js.Global().Get("__gothic_registry")
	if reg.IsUndefined() {
		reg = js.Global().Get("Object").New()
		js.Global().Set("__gothic_registry", reg)
	}
	return reg
}

func moduleRegistry() js.Value {
	reg := ensureRegistry()
	modID := moduleID()
	modReg := reg.Get(modID)
	if modReg.IsUndefined() {
		modReg = js.Global().Get("Object").New()
		reg.Set(modID, modReg)
	}
	return modReg
}

func ensureProxied() js.Value {
	p := js.Global().Get("__gothic_proxied")
	if p.IsUndefined() {
		p = js.Global().Get("Object").New()
		js.Global().Set("__gothic_proxied", p)
	}
	return p
}

// findScope walks up from window.event.target to find the nearest [data-gothic-scope].
func findScope() string {
	event := js.Global().Get("event")
	if event.IsUndefined() || event.IsNull() {
		return ""
	}
	target := event.Get("target")
	if target.IsUndefined() || target.IsNull() {
		return ""
	}
	el := target.Call("closest", "[data-gothic-scope]")
	if el.IsNull() || el.IsUndefined() {
		return ""
	}
	return el.Get("dataset").Get("gothicScope").String()
}

// dispatch / dispatchVoid / dispatchString / dispatchBool are intra-module
// helpers for user-registered callbacks invoked via the global proxy. They
// are NOT part of the cross-module context broadcast path (ListenCtxEvent
// et al.) — args here come from a direct fn.Invoke() in the same
// synchronous turn, not from a CustomEvent dispatched across modules.
//
// dispatch invokes the named function in the current scope's module registry
// (or any module registry as fallback), forwarding args to the JS function.
//
// Direct Go unit tests are not practical for this function because it is
// compiled only under the js && wasm build tag. The existing Playwright
// suite (codec.spec.ts, components.spec.ts) covers this code path end-to-end.
func dispatch(name string, args ...interface{}) {
	registry := ensureRegistry()
	if scopeID := findScope(); scopeID != "" {
		if mod := registry.Get(scopeID); !mod.IsUndefined() {
			if fn := mod.Get(name); !fn.IsUndefined() {
				fn.Invoke(args...)
				return
			}
		}
	}
	// fallback: first module that has this function
	keys := js.Global().Get("Object").Call("keys", registry)
	for i := 0; i < keys.Length(); i++ {
		mod := registry.Get(keys.Index(i).String())
		if mod.IsUndefined() {
			continue
		}
		if fn := mod.Get(name); !fn.IsUndefined() {
			fn.Invoke(args...)
			return
		}
	}
}

func dispatchVoid(name string)           { dispatch(name) }
func dispatchString(name, val string)    { dispatch(name, val) }
func dispatchBool(name string, val bool) { dispatch(name, val) }

// installProxy ensures a single global proxy is installed on window for name
// the first time it's encountered; subsequent calls short-circuit. The proxy
// is built lazily by makeProxy and retained in keep so the Go GC won't reclaim it.
func installProxy(name string, makeProxy func() js.Func) {
	proxied := ensureProxied()
	if !proxied.Get(name).IsUndefined() {
		return
	}
	proxy := makeProxy()
	keep = append(keep, proxy)
	js.Global().Set(name, proxy)
	proxied.Set(name, jsTrue)
}

// registerLocal stores impl in this module's slot of the global registry and
// retains it in keep.
func registerLocal(name string, impl js.Func) {
	keep = append(keep, impl)
	moduleRegistry().Set(name, impl)
}

func CreateWasmFunc(name string, fn func()) {
	registerLocal(name, js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		fn()
		return nil
	}))
	nameCopy := name
	installProxy(name, func() js.Func {
		return js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			dispatchVoid(nameCopy)
			return nil
		})
	})
}

func CreateWasmStringFunc(name string, fn func(string)) {
	registerLocal(name, js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		val := ""
		if len(args) > 0 {
			val = args[0].String()
		}
		fn(val)
		return nil
	}))
	nameCopy := name
	installProxy(name, func() js.Func {
		return js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			val := ""
			if len(args) > 0 {
				val = args[0].String()
			}
			dispatchString(nameCopy, val)
			return nil
		})
	})
}

func CreateWasmBoolFunc(name string, fn func(bool)) {
	registerLocal(name, js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		val := false
		if len(args) > 0 {
			val = args[0].Bool()
		}
		fn(val)
		return nil
	}))
	nameCopy := name
	installProxy(name, func() js.Func {
		return js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			val := false
			if len(args) > 0 {
				val = args[0].Bool()
			}
			dispatchBool(nameCopy, val)
			return nil
		})
	})
}
