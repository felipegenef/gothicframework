//go:build js && wasm

package runtime

type Effect struct {
	fn      func()
	cleanup func()
	deps    []dependency
	active  bool
}

func (e *Effect) run() {
	if !e.active {
		return
	}
	for _, d := range e.deps {
		d.removeEffect(e)
	}
	e.deps = e.deps[:0]

	if e.cleanup != nil {
		e.cleanup()
		e.cleanup = nil
	}

	prev := currentEffect
	currentEffect = e
	e.fn()
	currentEffect = prev
}

func (e *Effect) Stop() {
	e.active = false
	for _, d := range e.deps {
		d.removeEffect(e)
	}
	e.deps = nil
}

func UseEffect(fn func()) *Effect {
	e := &Effect{fn: fn, active: true}
	e.run()
	return e
}

func UseEffectWithCleanup(fn func() func()) *Effect {
	e := &Effect{active: true}
	e.fn = func() {
		cleanup := fn()
		e.cleanup = cleanup
	}
	e.run()
	return e
}
