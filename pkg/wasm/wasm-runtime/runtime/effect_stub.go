//go:build !js || !wasm

package runtime

type Effect struct {
	fn           func()
	active       bool
	deps         []dependency
	explicitDeps bool
}

func (e *Effect) run()  {}
func (e *Effect) Stop() { e.active = false }

func UseEffect(fn func(), deps ...any) *Effect {
	if len(deps) == 0 {
		fn()
		return &Effect{active: false}
	}
	e := &Effect{fn: fn, active: true, explicitDeps: true}
	fn()
	return e
}

func UseEffectWithCleanup(fn func() func(), deps ...any) *Effect {
	if len(deps) == 0 {
		fn()
		return &Effect{active: false}
	}
	e := &Effect{active: true, explicitDeps: true}
	fn()
	return e
}
