//go:build js && wasm

package runtime

var batchDepth int
var pendingEffects []*Effect

func addPending(e *Effect) {
	for _, pe := range pendingEffects {
		if pe == e {
			return
		}
	}
	pendingEffects = append(pendingEffects, e)
}

func Batch(fn func()) {
	batchDepth++
	fn()
	batchDepth--
	if batchDepth != 0 {
		return
	}
	pending := make([]*Effect, len(pendingEffects))
	copy(pending, pendingEffects)
	pendingEffects = pendingEffects[:0]
	for _, e := range pending {
		e.run()
	}
}

func Computed[T any](fn func() T) *Signal[T] {
	s := &Signal[T]{}
	var computeEffect *Effect
	computeEffect = &Effect{active: true}
	computeEffect.fn = func() {
		s.value = fn()
		subs := make([]*Effect, 0, len(s.effects))
		for _, sub := range s.effects {
			if sub != computeEffect {
				subs = append(subs, sub)
			}
		}
		for _, sub := range subs {
			sub.run()
		}
	}
	computeEffect.run()
	return s
}

func Memo[T any](fn func() T) *Signal[T] {
	return Computed(fn)
}
