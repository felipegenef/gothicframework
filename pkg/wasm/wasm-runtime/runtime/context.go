//go:build js && wasm

package runtime

import "syscall/js"

// ContextKey is a typed identifier for a shared context value.
// T encodes the type — provider and consumer must use the same T and Name.
type ContextKey[T any] struct {
	Name string
}

func ensureContextStore() js.Value {
	store := js.Global().Get("__gothic_context")
	if store.IsUndefined() {
		store = js.Global().Get("Object").New()
		js.Global().Set("__gothic_context", store)
	}
	return store
}

// ProvideContext makes signal the source of truth for a named context.
// Whenever signal changes the new value is written to the JS context store
// and broadcast via a CustomEvent so all UseContext consumers update.
func ProvideContext[T any](key ContextKey[T], signal *Signal[T], encode func(T) string) {
	e := &Effect{active: true} // explicitDeps:false → auto-tracking via signal.Get()
	e.fn = func() {
		encoded := encode(signal.Get())
		ensureContextStore().Set(key.Name, encoded)
		init := js.Global().Get("Object").New()
		init.Set("detail", encoded)
		event := js.Global().Get("CustomEvent").New("gothic:context:"+key.Name, init)
		js.Global().Get("document").Call("dispatchEvent", event)
	}
	e.run()
}

// UseContext subscribes to a named context and returns a local *Signal[T] that
// mirrors the provider's value. It can be used as a dep in UseEffect like any
// other signal.
func UseContext[T any](key ContextKey[T], initial T, decode func(string) T) *Signal[T] {
	current := initial
	v := ensureContextStore().Get(key.Name)
	if !v.IsUndefined() && !v.IsNull() {
		current = decode(v.String())
	}

	s := &Signal[T]{value: current}

	listener := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) > 0 {
			detail := args[0].Get("detail")
			if !detail.IsUndefined() && !detail.IsNull() {
				s.Set(decode(detail.String()))
			}
		}
		return nil
	})
	keep = append(keep, listener)
	js.Global().Get("document").Call("addEventListener", "gothic:context:"+key.Name, listener)

	return s
}
