//go:build js && wasm

package runtime

import "syscall/js"

var keep []js.Func
var jsTrue = js.ValueOf(true)

// cachedModuleID is read once at package init — synchronously during go.run() startup,
// before other async bootstrap IIFEs can overwrite window.__gothicCurrentModule.
var cachedModuleID = func() string {
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

func dispatchVoid(name string) {
	registry := ensureRegistry()
	if scopeID := findScope(); scopeID != "" {
		if mod := registry.Get(scopeID); !mod.IsUndefined() {
			if fn := mod.Get(name); !fn.IsUndefined() {
				fn.Invoke()
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
			fn.Invoke()
			return
		}
	}
}

func dispatchString(name, val string) {
	registry := ensureRegistry()
	if scopeID := findScope(); scopeID != "" {
		if mod := registry.Get(scopeID); !mod.IsUndefined() {
			if fn := mod.Get(name); !fn.IsUndefined() {
				fn.Invoke(val)
				return
			}
		}
	}
	keys := js.Global().Get("Object").Call("keys", registry)
	for i := 0; i < keys.Length(); i++ {
		mod := registry.Get(keys.Index(i).String())
		if mod.IsUndefined() {
			continue
		}
		if fn := mod.Get(name); !fn.IsUndefined() {
			fn.Invoke(val)
			return
		}
	}
}

func dispatchBool(name string, val bool) {
	registry := ensureRegistry()
	if scopeID := findScope(); scopeID != "" {
		if mod := registry.Get(scopeID); !mod.IsUndefined() {
			if fn := mod.Get(name); !fn.IsUndefined() {
				fn.Invoke(val)
				return
			}
		}
	}
	keys := js.Global().Get("Object").Call("keys", registry)
	for i := 0; i < keys.Length(); i++ {
		mod := registry.Get(keys.Index(i).String())
		if mod.IsUndefined() {
			continue
		}
		if fn := mod.Get(name); !fn.IsUndefined() {
			fn.Invoke(val)
			return
		}
	}
}

func CreateWasmFunc(name string, fn func()) {
	impl := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		fn()
		return nil
	})
	keep = append(keep, impl)
	moduleRegistry().Set(name, impl)

	proxied := ensureProxied()
	if proxied.Get(name).IsUndefined() {
		nameCopy := name
		proxy := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			dispatchVoid(nameCopy)
			return nil
		})
		keep = append(keep, proxy)
		js.Global().Set(nameCopy, proxy)
		proxied.Set(nameCopy, jsTrue)
	}
}

func CreateWasmStringFunc(name string, fn func(string)) {
	impl := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		val := ""
		if len(args) > 0 {
			val = args[0].String()
		}
		fn(val)
		return nil
	})
	keep = append(keep, impl)
	moduleRegistry().Set(name, impl)

	proxied := ensureProxied()
	if proxied.Get(name).IsUndefined() {
		nameCopy := name
		proxy := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			val := ""
			if len(args) > 0 {
				val = args[0].String()
			}
			dispatchString(nameCopy, val)
			return nil
		})
		keep = append(keep, proxy)
		js.Global().Set(nameCopy, proxy)
		proxied.Set(nameCopy, jsTrue)
	}
}

func CreateWasmBoolFunc(name string, fn func(bool)) {
	impl := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		val := false
		if len(args) > 0 {
			val = args[0].Bool()
		}
		fn(val)
		return nil
	})
	keep = append(keep, impl)
	moduleRegistry().Set(name, impl)

	proxied := ensureProxied()
	if proxied.Get(name).IsUndefined() {
		nameCopy := name
		proxy := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			val := false
			if len(args) > 0 {
				val = args[0].Bool()
			}
			dispatchBool(nameCopy, val)
			return nil
		})
		keep = append(keep, proxy)
		js.Global().Set(nameCopy, proxy)
		proxied.Set(nameCopy, jsTrue)
	}
}
