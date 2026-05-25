//go:build !js || !wasm

package runtime

type Subscription struct {
	fn           func()
	active       bool
	deps         []dependency
	explicitDeps bool
}

func (e *Subscription) run()  {}
func (e *Subscription) Stop() { e.active = false }

func Observe(fn func(), deps ...any) *Subscription {
	if len(deps) == 0 {
		fn()
		return &Subscription{active: false}
	}
	e := &Subscription{fn: fn, active: true, explicitDeps: true}
	fn()
	return e
}

func ObserveWithCleanup(fn func() func(), deps ...any) *Subscription {
	if len(deps) == 0 {
		fn()
		return &Subscription{active: false}
	}
	e := &Subscription{active: true, explicitDeps: true}
	fn()
	return e
}
