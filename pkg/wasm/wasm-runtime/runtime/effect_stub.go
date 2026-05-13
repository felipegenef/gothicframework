//go:build !js || !wasm

package runtime

type Effect struct {
	fn     func()
	active bool
	deps   []dependency
}

func (e *Effect) run()  { e.fn() }
func (e *Effect) Stop() { e.active = false }

func UseEffect(fn func()) *Effect {
	e := &Effect{fn: fn, active: true}
	e.fn()
	return e
}

func UseEffectWithCleanup(fn func() func()) *Effect {
	e := &Effect{active: true}
	cleanup := fn()
	_ = cleanup
	return e
}
