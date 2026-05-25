//go:build !js || !wasm

package runtime

var currentSubscription *Subscription

type dependency interface {
	addEffect(e *Subscription)
	removeEffect(e *Subscription)
}

type Observable[T any] struct{ value T }

func CreateObservable[T any](initial T) *Observable[T] { return &Observable[T]{value: initial} }
func (s *Observable[T]) Get() T                        { return s.value }
func (s *Observable[T]) Set(v T)                       { s.value = v }
func (s *Observable[T]) notifyAll()                    {}
func (s *Observable[T]) notifySubscribers()            {}
func (s *Observable[T]) addEffect(_ *Subscription)           {}
func (s *Observable[T]) removeEffect(_ *Subscription)        {}
