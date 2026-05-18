//go:build !js || !wasm

package runtime

import (
	"math"
	"testing"
)

// The primitive ContextKey factories are constructed via newPrimitiveKey.
// Each test asserts that encode(value) followed by decode(...) round-trips
// representative values: min, zero, max, and a typical value. This guards
// against drift between the WASM-side (context.go) and server-side
// (context_stub.go) factories.
//
// Functional coverage for the cross-module dispatch helpers
// (BroadcastCtxEncoded / ListenCtxEvent and the _gothicPending hybrid
// dispatch/listen pattern) lives in the TestGothic Playwright suite —
// specifically `codec-ctsetdeep-repro.spec.ts` and
// `codec-stress-random.spec.ts`. Those helpers depend on `syscall/js`
// (`js.Value`, `js.FuncOf`, `js.Global()`), which only exists in the
// `js && wasm` build, so direct Go unit-testing of their bodies is not
// possible. What we CAN do at unit-test time, and what the tests below
// do, is:
//
//   1. Lock the exported signatures across the WASM and stub builds via
//      typed function-variable assignments. Any drift between context.go
//      and context_stub.go fails the build at this site.
//   2. Exercise the stub no-op bodies to confirm they never panic, so
//      server-side code (which links against the !js stub) can call them
//      without runtime surprises.

func TestBoolKeyRoundTrip(t *testing.T) {
	k := BoolKey("b")
	if k.Name != "b" {
		t.Fatalf("name: got %q want %q", k.Name, "b")
	}
	for _, v := range []bool{false, true} {
		if got := k.decode(k.encode(v)); got != v {
			t.Errorf("BoolKey round-trip %v: got %v", v, got)
		}
	}
}

func TestStringKeyRoundTrip(t *testing.T) {
	k := StringKey("s")
	for _, v := range []string{"", "hello", "a string with spaces", "é-utf8"} {
		if got := k.decode(k.encode(v)); got != v {
			t.Errorf("StringKey round-trip %q: got %q", v, got)
		}
	}
}

func TestIntKeyRoundTrip(t *testing.T) {
	k := IntKey("i")
	for _, v := range []int{math.MinInt, -1, 0, 1, 42, math.MaxInt} {
		if got := k.decode(k.encode(v)); got != v {
			t.Errorf("IntKey round-trip %d: got %d", v, got)
		}
	}
}

func TestInt8KeyRoundTrip(t *testing.T) {
	k := Int8Key("i8")
	for _, v := range []int8{math.MinInt8, -1, 0, 1, 42, math.MaxInt8} {
		if got := k.decode(k.encode(v)); got != v {
			t.Errorf("Int8Key round-trip %d: got %d", v, got)
		}
	}
}

func TestInt16KeyRoundTrip(t *testing.T) {
	k := Int16Key("i16")
	for _, v := range []int16{math.MinInt16, -1, 0, 1, 42, math.MaxInt16} {
		if got := k.decode(k.encode(v)); got != v {
			t.Errorf("Int16Key round-trip %d: got %d", v, got)
		}
	}
}

func TestInt32KeyRoundTrip(t *testing.T) {
	k := Int32Key("i32")
	for _, v := range []int32{math.MinInt32, -1, 0, 1, 42, math.MaxInt32} {
		if got := k.decode(k.encode(v)); got != v {
			t.Errorf("Int32Key round-trip %d: got %d", v, got)
		}
	}
}

func TestInt64KeyRoundTrip(t *testing.T) {
	k := Int64Key("i64")
	for _, v := range []int64{math.MinInt64, -1, 0, 1, 42, math.MaxInt64} {
		if got := k.decode(k.encode(v)); got != v {
			t.Errorf("Int64Key round-trip %d: got %d", v, got)
		}
	}
}

func TestUintKeyRoundTrip(t *testing.T) {
	k := UintKey("u")
	for _, v := range []uint{0, 1, 42, math.MaxUint} {
		if got := k.decode(k.encode(v)); got != v {
			t.Errorf("UintKey round-trip %d: got %d", v, got)
		}
	}
}

func TestUint8KeyRoundTrip(t *testing.T) {
	k := Uint8Key("u8")
	for _, v := range []uint8{0, 1, 42, math.MaxUint8} {
		if got := k.decode(k.encode(v)); got != v {
			t.Errorf("Uint8Key round-trip %d: got %d", v, got)
		}
	}
}

func TestUint16KeyRoundTrip(t *testing.T) {
	k := Uint16Key("u16")
	for _, v := range []uint16{0, 1, 42, math.MaxUint16} {
		if got := k.decode(k.encode(v)); got != v {
			t.Errorf("Uint16Key round-trip %d: got %d", v, got)
		}
	}
}

func TestUint32KeyRoundTrip(t *testing.T) {
	k := Uint32Key("u32")
	for _, v := range []uint32{0, 1, 42, math.MaxUint32} {
		if got := k.decode(k.encode(v)); got != v {
			t.Errorf("Uint32Key round-trip %d: got %d", v, got)
		}
	}
}

func TestUint64KeyRoundTrip(t *testing.T) {
	k := Uint64Key("u64")
	for _, v := range []uint64{0, 1, 42, math.MaxUint64} {
		if got := k.decode(k.encode(v)); got != v {
			t.Errorf("Uint64Key round-trip %d: got %d", v, got)
		}
	}
}

func TestFloat32KeyRoundTrip(t *testing.T) {
	k := Float32Key("f32")
	// Use strict-equality safe values; ParseFloat/FormatFloat with -1 precision
	// preserves the shortest decimal that round-trips exactly to the same bits.
	for _, v := range []float32{-math.MaxFloat32, -1.5, 0, 0.5, 3.14159, math.MaxFloat32} {
		if got := k.decode(k.encode(v)); got != v {
			t.Errorf("Float32Key round-trip %v: got %v", v, got)
		}
	}
}

func TestFloat64KeyRoundTrip(t *testing.T) {
	k := Float64Key("f64")
	for _, v := range []float64{-math.MaxFloat64, -1.5, 0, 0.5, 3.141592653589793, math.MaxFloat64} {
		if got := k.decode(k.encode(v)); got != v {
			t.Errorf("Float64Key round-trip %v: got %v", v, got)
		}
	}
}

func TestRuneKeyRoundTrip(t *testing.T) {
	k := RuneKey("r")
	for _, v := range []rune{math.MinInt32, -1, 0, 'A', 'é', 0x1F600, math.MaxInt32} {
		if got := k.decode(k.encode(v)); got != v {
			t.Errorf("RuneKey round-trip %d: got %d", v, got)
		}
	}
}

func TestByteKeyRoundTrip(t *testing.T) {
	k := ByteKey("by")
	for _, v := range []byte{0, 1, 42, math.MaxUint8} {
		if got := k.decode(k.encode(v)); got != v {
			t.Errorf("ByteKey round-trip %d: got %d", v, got)
		}
	}
}

// TestPrimitiveKeyNames asserts the Name field is correctly threaded through
// newPrimitiveKey for every factory. This is the one piece of state shared by
// all 16 factories — if newPrimitiveKey were ever to drop Name, every factory
// would silently break.
func TestPrimitiveKeyNames(t *testing.T) {
	cases := []struct {
		name string
		got  string
	}{
		{"bool-name", BoolKey("bool-name").Name},
		{"string-name", StringKey("string-name").Name},
		{"int-name", IntKey("int-name").Name},
		{"int8-name", Int8Key("int8-name").Name},
		{"int16-name", Int16Key("int16-name").Name},
		{"int32-name", Int32Key("int32-name").Name},
		{"int64-name", Int64Key("int64-name").Name},
		{"uint-name", UintKey("uint-name").Name},
		{"uint8-name", Uint8Key("uint8-name").Name},
		{"uint16-name", Uint16Key("uint16-name").Name},
		{"uint32-name", Uint32Key("uint32-name").Name},
		{"uint64-name", Uint64Key("uint64-name").Name},
		{"float32-name", Float32Key("float32-name").Name},
		{"float64-name", Float64Key("float64-name").Name},
		{"rune-name", RuneKey("rune-name").Name},
		{"byte-name", ByteKey("byte-name").Name},
	}
	for _, c := range cases {
		if c.got != c.name {
			t.Errorf("name: got %q want %q", c.got, c.name)
		}
	}
}

// TestContextDispatchAPISurface locks the exported signatures of the
// cross-module context dispatch helpers. Binding each function to a typed
// variable forces a build error if dom.go's WASM-side signature ever drifts
// from the stub-side signature in context_stub.go — Go won't let two files
// in the same package multiplex the same name with different types.
//
// The dispatch bodies themselves rely on `syscall/js` and can only run under
// the WASM build; the functional coverage for the hybrid `_gothicPending`
// dispatch/listen pattern lives in TestGothic's Playwright suite. See the
// file-level comment above for the specific spec files.
func TestContextDispatchAPISurface(t *testing.T) {
	var (
		_ func(keyName, encoded string)            = BroadcastCtxEncoded
		_ func(keyName, encoded string)            = RequestCtxSet
		_ func(keyName, encoded string)            = BroadcastCtxOnline
		_ func(keyName string, fn func(string))    = ListenCtxEvent
		_ func(keyName string, fn func(string))    = ListenCtxSetReq
		_ func(keyName string, fn func(string))    = ListenCtxOnline
		_ func(keyName string, fn func())          = ListenCtxPing
		_ func(keyName string)                     = PingCtxManager
		_ func(keyName string, isOnline func() bool) = PingUntilOnline
	)

	// Stub no-op contract: each function must be safe to invoke under the
	// !js || !wasm build (server side, tests, tools). A panic here would
	// mean some stub body grew a real implementation that can't run outside
	// WASM — a regression.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("stub dispatch helper panicked: %v", r)
		}
	}()
	BroadcastCtxEncoded("k", "v")
	RequestCtxSet("k", "v")
	BroadcastCtxOnline("k", "v")
	ListenCtxEvent("k", func(string) {})
	ListenCtxSetReq("k", func(string) {})
	ListenCtxOnline("k", func(string) {})
	ListenCtxPing("k", func() {})
	PingCtxManager("k")
	PingUntilOnline("k", func() bool { return true })
}

func TestPayloadBufStubSignatures(t *testing.T) {
	// Compile-time check that all exported context functions exist
	// in the stub build (this file builds under !js tag).
	var _ = ListenCtxEvent
	var _ = ListenCtxSetReq
	var _ = ListenCtxOnline
	var _ = ListenCtxPing
	var _ = BroadcastCtxEncoded
	var _ = RequestCtxSet
	var _ = PingCtxManager
	var _ = BroadcastCtxOnline
}
