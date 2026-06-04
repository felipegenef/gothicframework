//go:build !js || !wasm

package runtime

import "testing"

// --- decodeErr ---

func TestDecodeErrMessage(t *testing.T) {
	var e decodeErr = "test error"
	if got := e.Error(); got != "test error" {
		t.Errorf("decodeErr.Error() = %q, want %q", got, "test error")
	}
	if got := errUnderflow.Error(); got != "codec: buffer underflow" {
		t.Errorf("errUnderflow.Error() = %q", got)
	}
}

// --- Observable ---

func TestObservable(t *testing.T) {
	o := CreateObservable(42)
	if got := o.Get(); got != 42 {
		t.Errorf("Get() = %v, want 42", got)
	}
	o.Set(99)
	if got := o.Get(); got != 99 {
		t.Errorf("Get() after Set() = %v, want 99", got)
	}
	// no-op methods must not panic
	o.notifyAll()
	o.notifySubscribers()
	o.addEffect(nil)
	o.removeEffect(nil)
}

func TestObservableString(t *testing.T) {
	o := CreateObservable("hello")
	if got := o.Get(); got != "hello" {
		t.Errorf("Get() = %q, want %q", got, "hello")
	}
	o.Set("world")
	if got := o.Get(); got != "world" {
		t.Errorf("Get() after Set() = %q, want %q", got, "world")
	}
}

// --- Subscription / Observer ---

func TestSubscriptionStop(t *testing.T) {
	called := 0
	sub := Observe(func() { called++ })
	// With no deps: fn runs once, returns inactive subscription.
	if called != 1 {
		t.Errorf("Observe(fn) without deps: fn called %d times, want 1", called)
	}
	if sub.active {
		t.Errorf("subscription without deps should be inactive")
	}
	sub.Stop() // must not panic
}

func TestObserveWithDeps(t *testing.T) {
	called := 0
	o := CreateObservable(0)
	sub := Observe(func() { called++ }, o)
	if called != 1 {
		t.Errorf("Observe with dep: fn called %d times, want 1", called)
	}
	if !sub.active {
		t.Errorf("subscription with dep should be active")
	}
	sub.Stop()
	if sub.active {
		t.Errorf("after Stop(), active should be false")
	}
	sub.run() // no-op, must not panic
}

func TestObserveWithCleanup_NoDeps(t *testing.T) {
	called := 0
	sub := ObserveWithCleanup(func() func() {
		called++
		return func() {}
	})
	if called != 1 {
		t.Errorf("ObserveWithCleanup without deps: fn called %d times, want 1", called)
	}
	if sub.active {
		t.Errorf("subscription without deps should be inactive")
	}
}

func TestObserveWithCleanup_WithDeps(t *testing.T) {
	called := 0
	o := CreateObservable(0)
	sub := ObserveWithCleanup(func() func() {
		called++
		return func() {}
	}, o)
	if called != 1 {
		t.Errorf("ObserveWithCleanup with dep: fn called %d times, want 1", called)
	}
	if !sub.active {
		t.Errorf("subscription with dep should be active")
	}
}

// --- Scheduler ---

func TestBatchNoop(t *testing.T) {
	// must not panic
	BeginBatch()
	EndBatch()
	addPendingSubscription(nil)
}

// --- Events stubs ---

func TestEventStubs(t *testing.T) {
	// All are no-ops; just ensure they compile and don't panic.
	CreateWasmFunc("test", func() {})
	CreateWasmStringFunc("test", func(s string) {})
	CreateWasmBoolFunc("test", func(b bool) {})
	_ = CreateWasmFuncWithReturn("test", func(this JSValue, args []JSValue) any { return nil })
}

// --- DOM stubs not covered by dom_stub_test.go ---

func TestDomStubsRemaining(t *testing.T) {
	// GetFileBytes returns nil
	if got := GetFileBytes("x"); got != nil {
		t.Errorf("GetFileBytes: got %v, want nil", got)
	}
	// Fetch/FetchBytes return zero values
	s, err := Fetch("http://example.com")
	if s != "" || err != nil {
		t.Errorf("Fetch stub: got (%q, %v)", s, err)
	}
	b, err := FetchBytes("http://example.com")
	if b != nil || err != nil {
		t.Errorf("FetchBytes stub: got (%v, %v)", b, err)
	}
	// JSValue zero-value contract.
	jv := JS()
	if !jv.IsNull() {
		t.Error("zero JSValue.IsNull() should be true")
	}
	if !jv.IsUndefined() {
		t.Error("zero JSValue.IsUndefined() should be true")
	}
	if jv.Truthy() {
		t.Error("zero JSValue.Truthy() should be false")
	}
	if jv.Int() != 0 {
		t.Error("zero JSValue.Int() should be 0")
	}
	if jv.Float() != 0 {
		t.Error("zero JSValue.Float() should be 0")
	}
	if jv.String() != "" {
		t.Error("zero JSValue.String() should be empty")
	}
	if jv.Length() != 0 {
		t.Error("zero JSValue.Length() should be 0")
	}
	if jv.Bool() {
		t.Error("zero JSValue.Bool() should be false")
	}
}
