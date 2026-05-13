//go:build js && wasm

package runtime

import "syscall/js"

type Effect struct {
	fn           func()
	cleanup      func()
	deps         []dependency
	active       bool
	explicitDeps bool
}

func (e *Effect) run() {
	if !e.active {
		return
	}

	if !e.explicitDeps {
		// Auto-tracking mode: unsubscribe from all current deps, then re-track
		// during fn() via the currentEffect global.
		for _, d := range e.deps {
			d.removeEffect(e)
		}
		e.deps = e.deps[:0]
	}

	if e.cleanup != nil {
		e.cleanup()
		e.cleanup = nil
	}

	if !e.explicitDeps {
		prev := currentEffect
		currentEffect = e
		e.fn()
		currentEffect = prev
	} else {
		e.fn()
	}
}

func (e *Effect) Stop() {
	e.active = false
	for _, d := range e.deps {
		d.removeEffect(e)
	}
	e.deps = nil
}

func devWarn(msg string) {
	if js.Global().Get("__gothic_dev").Truthy() {
		js.Global().Get("console").Call("warn", "[gothic] "+msg)
	}
}

// UseEffect runs fn immediately and re-runs it whenever a listed dep changes.
// Pass no deps to run fn exactly once with no reactive subscription.
func UseEffect(fn func(), deps ...any) *Effect {
	if len(deps) == 0 {
		fn()
		return &Effect{active: false}
	}
	e := &Effect{fn: fn, active: true, explicitDeps: true}
	for _, dep := range deps {
		d, ok := dep.(dependency)
		if !ok {
			devWarn("UseEffect: a dep is not a *Signal and will be ignored — only values returned by UseState may be passed as dependencies")
			continue
		}
		d.addEffect(e)
		e.deps = append(e.deps, d)
	}
	fn()
	return e
}

// UseEffectWithCleanup is like UseEffect but fn may return a cleanup function
// that runs before each re-execution and when Stop() is called.
func UseEffectWithCleanup(fn func() func(), deps ...any) *Effect {
	if len(deps) == 0 {
		cleanup := fn()
		_ = cleanup
		return &Effect{active: false}
	}
	e := &Effect{active: true, explicitDeps: true}
	e.fn = func() {
		e.cleanup = fn()
	}
	for _, dep := range deps {
		d, ok := dep.(dependency)
		if !ok {
			devWarn("UseEffectWithCleanup: a dep is not a *Signal and will be ignored — only values returned by UseState may be passed as dependencies")
			continue
		}
		d.addEffect(e)
		e.deps = append(e.deps, d)
	}
	e.fn()
	return e
}
