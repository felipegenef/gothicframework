//go:build !js || !wasm

package runtime

var currentEffect *Effect

type dependency interface{ removeEffect(e *Effect) }

type Signal[T any] struct{ value T }

func UseState[T any](initial T) *Signal[T]    { return &Signal[T]{value: initial} }
func (s *Signal[T]) Get() T                   { return s.value }
func (s *Signal[T]) Set(v T)                  { s.value = v }
func (s *Signal[T]) notifyAll()               {}
func (s *Signal[T]) notifySubscribers()       {}
func (s *Signal[T]) removeEffect(_ *Effect)   {}
