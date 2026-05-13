//go:build js && wasm

package runtime

var currentEffect *Effect

type dependency interface {
	removeEffect(e *Effect)
}

type Signal[T any] struct {
	value   T
	effects []*Effect
}

func UseState[T any](initial T) *Signal[T] {
	return &Signal[T]{value: initial}
}

func (s *Signal[T]) Get() T {
	if currentEffect != nil {
		for _, e := range s.effects {
			if e == currentEffect {
				return s.value
			}
		}
		s.effects = append(s.effects, currentEffect)
		currentEffect.deps = append(currentEffect.deps, s)
	}
	return s.value
}

func (s *Signal[T]) Set(v T) {
	s.value = v
	s.notifyAll()
}

func (s *Signal[T]) notifyAll() {
	if batchDepth > 0 {
		for _, e := range s.effects {
			addPending(e)
		}
		return
	}
	effects := make([]*Effect, len(s.effects))
	copy(effects, s.effects)
	for _, e := range effects {
		e.run()
	}
}

func (s *Signal[T]) notifySubscribers() {
	effects := make([]*Effect, len(s.effects))
	copy(effects, s.effects)
	for _, e := range effects {
		e.run()
	}
}

func (s *Signal[T]) removeEffect(e *Effect) {
	for i, eff := range s.effects {
		if eff == e {
			last := len(s.effects) - 1
			s.effects[i] = s.effects[last]
			s.effects = s.effects[:last]
			return
		}
	}
}
