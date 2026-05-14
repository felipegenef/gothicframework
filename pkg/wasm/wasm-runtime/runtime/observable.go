//go:build js && wasm

package runtime

var currentSubscription *Subscription

type dependency interface {
	addEffect(e *Subscription)
	removeEffect(e *Subscription)
}

// Observable is a reactive value container. Reading it inside an Observe callback
// automatically subscribes that callback to future updates; calling Set notifies
// all subscribers synchronously.
//
// Create one with CreateObservable:
//
//	count := CreateObservable(0)
//	name  := CreateObservable("Alice")
//
// Similar to useState in React, but the value is held in the Observable itself
// rather than being destructured into a [value, setter] pair.
type Observable[T any] struct {
	value   T
	effects []*Subscription
}

// CreateObservable creates a new Observable with the given initial value.
// It is the Gothic equivalent of React's useState hook.
//
// Example:
//
//	count := CreateObservable(0)
//	label := CreateObservable("hello")
//
//	// Read the current value:
//	fmt.Println(count.Get())
//
//	// Update the value (triggers all Observe callbacks that depend on count):
//	count.Set(count.Get() + 1)
func CreateObservable[T any](initial T) *Observable[T] {
	return &Observable[T]{value: initial}
}

func (s *Observable[T]) Get() T {
	if currentSubscription != nil {
		for _, e := range s.effects {
			if e == currentSubscription {
				return s.value
			}
		}
		s.effects = append(s.effects, currentSubscription)
		currentSubscription.deps = append(currentSubscription.deps, s)
	}
	return s.value
}

func (s *Observable[T]) Set(v T) {
	s.value = v
	s.notifyAll()
}

func (s *Observable[T]) notifyAll() {
	if batchDepth > 0 {
		for _, e := range s.effects {
			addPendingSubscription(e)
		}
		return
	}
	effects := make([]*Subscription, len(s.effects))
	copy(effects, s.effects)
	for _, e := range effects {
		e.run()
	}
}

func (s *Observable[T]) notifySubscribers() {
	effects := make([]*Subscription, len(s.effects))
	copy(effects, s.effects)
	for _, e := range effects {
		e.run()
	}
}

func (s *Observable[T]) addEffect(e *Subscription) {
	for _, existing := range s.effects {
		if existing == e {
			return
		}
	}
	s.effects = append(s.effects, e)
}

func (s *Observable[T]) removeEffect(e *Subscription) {
	for i, eff := range s.effects {
		if eff == e {
			last := len(s.effects) - 1
			s.effects[i] = s.effects[last]
			s.effects = s.effects[:last]
			return
		}
	}
}
