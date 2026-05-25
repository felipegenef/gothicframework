//go:build js && wasm

package runtime

import "syscall/js"

type Subscription struct {
	fn           func()
	cleanup      func()
	deps         []dependency
	active       bool
	explicitDeps bool
}

func (e *Subscription) run() {
	if !e.active {
		return
	}

	if !e.explicitDeps {
		// Auto-tracking mode: unsubscribe from all current deps, then re-track
		// during fn() via the currentSubscription global.
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
		prev := currentSubscription
		currentSubscription = e
		e.fn()
		currentSubscription = prev
	} else {
		e.fn()
	}
}

func (e *Subscription) Stop() {
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

// Observe runs fn immediately and re-runs it whenever a listed dep changes.
// Pass no deps to run fn exactly once with no reactive subscription.
// It is the Gothic equivalent of React's useEffect hook.
//
// Example — update the DOM whenever count changes:
//
//	count := CreateObservable(0)
//
//	Observe(func() {
//	    document.GetElementById("counter").SetInnerHTML(fmt.Sprintf("%d", count.Get()))
//	}, count)
//
//	// Later, updating count re-runs the callback automatically:
//	count.Set(count.Get() + 1)
//
// Pass multiple deps to react to any of them:
//
//	Observe(func() { ... }, a, b, c)
//
// Pass no deps to run once on mount with no subscription:
//
//	Observe(func() { fmt.Println("mounted") })
func Observe(fn func(), deps ...any) *Subscription {
	if len(deps) == 0 {
		fn()
		return &Subscription{active: false}
	}
	e := &Subscription{fn: fn, active: true, explicitDeps: true}
	for _, dep := range deps {
		d, ok := dep.(dependency)
		if !ok {
			devWarn("Observe: a dep is not an *Observable and will be ignored — only values returned by CreateObservable may be passed as dependencies")
			continue
		}
		d.addEffect(e)
		e.deps = append(e.deps, d)
	}
	fn()
	return e
}

// ObserveWithCleanup is like Observe but fn may return a cleanup function
// that runs before each re-execution and when Stop() is called.
//
// Example — add and remove an event listener reactively:
//
//	ObserveWithCleanup(func() func() {
//	    el := document.GetElementById("btn")
//	    handler := el.AddEventListener("click", func() { ... })
//	    return func() { el.RemoveEventListener("click", handler) }
//	}, someObservable)
func ObserveWithCleanup(fn func() func(), deps ...any) *Subscription {
	if len(deps) == 0 {
		cleanup := fn()
		_ = cleanup
		return &Subscription{active: false}
	}
	e := &Subscription{active: true, explicitDeps: true}
	e.fn = func() {
		e.cleanup = fn()
	}
	for _, dep := range deps {
		d, ok := dep.(dependency)
		if !ok {
			devWarn("ObserveWithCleanup: a dep is not an *Observable and will be ignored — only values returned by CreateObservable may be passed as dependencies")
			continue
		}
		d.addEffect(e)
		e.deps = append(e.deps, d)
	}
	e.fn()
	return e
}
