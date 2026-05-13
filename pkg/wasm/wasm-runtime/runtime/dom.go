//go:build js && wasm

package runtime

import "syscall/js"

var document = js.Global().Get("document")

func getElementById(id string) js.Value {
	return document.Call("getElementById", id)
}

func SetText(id, value string) {
	el := getElementById(id)
	if el.IsNull() || el.IsUndefined() {
		return
	}
	el.Set("textContent", value)
}

func SetHTML(id, html string) {
	el := getElementById(id)
	if el.IsNull() || el.IsUndefined() {
		return
	}
	el.Set("innerHTML", html)
}

func SetValue(id, value string) {
	el := getElementById(id)
	if el.IsNull() || el.IsUndefined() {
		return
	}
	el.Set("value", value)
}

func GetValue(id string) string {
	el := getElementById(id)
	if el.IsNull() || el.IsUndefined() {
		return ""
	}
	return el.Get("value").String()
}

func AddClass(id, className string) {
	el := getElementById(id)
	if el.IsNull() || el.IsUndefined() {
		return
	}
	el.Get("classList").Call("add", className)
}

func RemoveClass(id, className string) {
	el := getElementById(id)
	if el.IsNull() || el.IsUndefined() {
		return
	}
	el.Get("classList").Call("remove", className)
}

func ToggleClass(id, className string) {
	el := getElementById(id)
	if el.IsNull() || el.IsUndefined() {
		return
	}
	el.Get("classList").Call("toggle", className)
}

func SetAttr(id, attr, value string) {
	el := getElementById(id)
	if el.IsNull() || el.IsUndefined() {
		return
	}
	el.Call("setAttribute", attr, value)
}

func SetStyle(id, property, value string) {
	el := getElementById(id)
	if el.IsNull() || el.IsUndefined() {
		return
	}
	el.Get("style").Set(property, value)
}
