package wasm_test

import (
	"bytes"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	wasm "github.com/felipegenef/gothicframework/pkg/wasm"
	"github.com/felipegenef/gothicframework/pkg/wasm/internal/parity"
)

// payload describes one logical "field" written to an encoder, paired with the
// assertions that verify a decoder read the same value back.
//
// kind names mirror the Encoder/Decoder method names exactly so the wire format
// being exercised is unambiguous when a test fails.
type payload struct {
	name string
	kind string
	// values per kind (only the field matching kind is used)
	u8     uint8
	u16    uint16
	u32    uint32
	u64    uint64
	i32    int32
	i64    int64
	f32    float32
	f64    float64
	b      bool
	bytes  []byte
	str    string
}

// parityPayloads returns the canonical set of values used by every parity test.
// Coverage: 0, max, min, negative for signed; empty + long for variable-length.
func parityPayloads() []payload {
	longBytes := make([]byte, 4096)
	for i := range longBytes {
		longBytes[i] = byte(i & 0xff)
	}
	longStr := strings.Repeat("gothic-wasm-parity-", 200) // ~3.8 KB

	return []payload{
		// U8
		{name: "u8_zero", kind: "U8", u8: 0},
		{name: "u8_max", kind: "U8", u8: math.MaxUint8},
		{name: "u8_mid", kind: "U8", u8: 0x7f},

		// U16
		{name: "u16_zero", kind: "U16", u16: 0},
		{name: "u16_max", kind: "U16", u16: math.MaxUint16},
		{name: "u16_mid", kind: "U16", u16: 0x1234},

		// U32
		{name: "u32_zero", kind: "U32", u32: 0},
		{name: "u32_max", kind: "U32", u32: math.MaxUint32},
		{name: "u32_mid", kind: "U32", u32: 0xDEADBEEF},

		// U64
		{name: "u64_zero", kind: "U64", u64: 0},
		{name: "u64_max", kind: "U64", u64: math.MaxUint64},
		{name: "u64_mid", kind: "U64", u64: 0xCAFEBABEDEADBEEF},

		// I32
		{name: "i32_zero", kind: "I32", i32: 0},
		{name: "i32_max", kind: "I32", i32: math.MaxInt32},
		{name: "i32_min", kind: "I32", i32: math.MinInt32},
		{name: "i32_neg", kind: "I32", i32: -42},

		// I64
		{name: "i64_zero", kind: "I64", i64: 0},
		{name: "i64_max", kind: "I64", i64: math.MaxInt64},
		{name: "i64_min", kind: "I64", i64: math.MinInt64},
		{name: "i64_neg", kind: "I64", i64: -123456789012},

		// F32
		{name: "f32_zero", kind: "F32", f32: 0},
		{name: "f32_neg", kind: "F32", f32: -1.5},
		{name: "f32_pi", kind: "F32", f32: 3.14159},
		{name: "f32_smallest_nonzero", kind: "F32", f32: math.SmallestNonzeroFloat32},
		{name: "f32_max", kind: "F32", f32: math.MaxFloat32},

		// F64
		{name: "f64_zero", kind: "F64", f64: 0},
		{name: "f64_neg", kind: "F64", f64: -2.718281828},
		{name: "f64_pi", kind: "F64", f64: math.Pi},
		{name: "f64_smallest_nonzero", kind: "F64", f64: math.SmallestNonzeroFloat64},
		{name: "f64_max", kind: "F64", f64: math.MaxFloat64},

		// Bool
		{name: "bool_false", kind: "Bool", b: false},
		{name: "bool_true", kind: "Bool", b: true},

		// Bytes
		{name: "bytes_nil", kind: "Bytes", bytes: nil},
		{name: "bytes_empty", kind: "Bytes", bytes: []byte{}},
		{name: "bytes_one", kind: "Bytes", bytes: []byte{0xAB}},
		{name: "bytes_short", kind: "Bytes", bytes: []byte{0x00, 0x01, 0x02, 0xFF}},
		{name: "bytes_long", kind: "Bytes", bytes: longBytes},

		// String
		{name: "string_empty", kind: "String", str: ""},
		{name: "string_ascii", kind: "String", str: "hello, gothic"},
		{name: "string_unicode", kind: "String", str: "héllo🦊wörld"},
		{name: "string_long", kind: "String", str: longStr},
	}
}

// encodeStub writes the payload list using the server-side stub encoder.
func encodeStub(t *testing.T, items []payload) []byte {
	t.Helper()
	e := wasm.NewEncoder(64)
	for _, p := range items {
		writeWith(stubWriter{e}, p)
	}
	return e.Buf
}

// encodeParity writes the payload list using the build-tag-free runtime copy.
func encodeParity(t *testing.T, items []payload) []byte {
	t.Helper()
	e := parity.NewEncoder(64)
	for _, p := range items {
		writeWith(parityWriter{e}, p)
	}
	return e.Buf
}

// encoderIface abstracts both Encoder types behind a common interface so the
// test exercises the same byte sequence through both implementations.
type encoderIface interface {
	U8(uint8)
	U16(uint16)
	U32(uint32)
	U64(uint64)
	I32(int32)
	I64(int64)
	F32(float32)
	F64(float64)
	Bool(bool)
	Bytes([]byte)
	String(string)
}

type stubWriter struct{ e *wasm.Encoder }

func (w stubWriter) U8(v uint8)      { w.e.U8(v) }
func (w stubWriter) U16(v uint16)    { w.e.U16(v) }
func (w stubWriter) U32(v uint32)    { w.e.U32(v) }
func (w stubWriter) U64(v uint64)    { w.e.U64(v) }
func (w stubWriter) I32(v int32)     { w.e.I32(v) }
func (w stubWriter) I64(v int64)     { w.e.I64(v) }
func (w stubWriter) F32(v float32)   { w.e.F32(v) }
func (w stubWriter) F64(v float64)   { w.e.F64(v) }
func (w stubWriter) Bool(v bool)     { w.e.Bool(v) }
func (w stubWriter) Bytes(v []byte)  { w.e.Bytes(v) }
func (w stubWriter) String(v string) { w.e.String(v) }

type parityWriter struct{ e *parity.Encoder }

func (w parityWriter) U8(v uint8)      { w.e.U8(v) }
func (w parityWriter) U16(v uint16)    { w.e.U16(v) }
func (w parityWriter) U32(v uint32)    { w.e.U32(v) }
func (w parityWriter) U64(v uint64)    { w.e.U64(v) }
func (w parityWriter) I32(v int32)     { w.e.I32(v) }
func (w parityWriter) I64(v int64)     { w.e.I64(v) }
func (w parityWriter) F32(v float32)   { w.e.F32(v) }
func (w parityWriter) F64(v float64)   { w.e.F64(v) }
func (w parityWriter) Bool(v bool)     { w.e.Bool(v) }
func (w parityWriter) Bytes(v []byte)  { w.e.Bytes(v) }
func (w parityWriter) String(v string) { w.e.String(v) }

func writeWith(e encoderIface, p payload) {
	switch p.kind {
	case "U8":
		e.U8(p.u8)
	case "U16":
		e.U16(p.u16)
	case "U32":
		e.U32(p.u32)
	case "U64":
		e.U64(p.u64)
	case "I32":
		e.I32(p.i32)
	case "I64":
		e.I64(p.i64)
	case "F32":
		e.F32(p.f32)
	case "F64":
		e.F64(p.f64)
	case "Bool":
		e.Bool(p.b)
	case "Bytes":
		e.Bytes(p.bytes)
	case "String":
		e.String(p.str)
	}
}

// decoderIface abstracts both Decoder types behind a common interface so the
// test reads back the same byte stream through both implementations.
type decoderIface interface {
	U8() uint8
	U16() uint16
	U32() uint32
	U64() uint64
	I32() int32
	I64() int64
	F32() float32
	F64() float64
	Bool() bool
	Bytes() []byte
	String() string
}

type stubReader struct{ d *wasm.Decoder }

func (r stubReader) U8() uint8      { return r.d.U8() }
func (r stubReader) U16() uint16    { return r.d.U16() }
func (r stubReader) U32() uint32    { return r.d.U32() }
func (r stubReader) U64() uint64    { return r.d.U64() }
func (r stubReader) I32() int32     { return r.d.I32() }
func (r stubReader) I64() int64     { return r.d.I64() }
func (r stubReader) F32() float32   { return r.d.F32() }
func (r stubReader) F64() float64   { return r.d.F64() }
func (r stubReader) Bool() bool     { return r.d.Bool() }
func (r stubReader) Bytes() []byte  { return r.d.Bytes() }
func (r stubReader) String() string { return r.d.String() }

type parityReader struct{ d *parity.Decoder }

func (r parityReader) U8() uint8      { return r.d.U8() }
func (r parityReader) U16() uint16    { return r.d.U16() }
func (r parityReader) U32() uint32    { return r.d.U32() }
func (r parityReader) U64() uint64    { return r.d.U64() }
func (r parityReader) I32() int32     { return r.d.I32() }
func (r parityReader) I64() int64     { return r.d.I64() }
func (r parityReader) F32() float32   { return r.d.F32() }
func (r parityReader) F64() float64   { return r.d.F64() }
func (r parityReader) Bool() bool     { return r.d.Bool() }
func (r parityReader) Bytes() []byte  { return r.d.Bytes() }
func (r parityReader) String() string { return r.d.String() }

// readAll runs every payload through the decoder and returns the read values.
// The shape mirrors `payload` so we can compare element-by-element with the
// original input.
func readAll(d decoderIface, items []payload) []payload {
	out := make([]payload, len(items))
	for i, p := range items {
		out[i] = payload{name: p.name, kind: p.kind}
		switch p.kind {
		case "U8":
			out[i].u8 = d.U8()
		case "U16":
			out[i].u16 = d.U16()
		case "U32":
			out[i].u32 = d.U32()
		case "U64":
			out[i].u64 = d.U64()
		case "I32":
			out[i].i32 = d.I32()
		case "I64":
			out[i].i64 = d.I64()
		case "F32":
			out[i].f32 = d.F32()
		case "F64":
			out[i].f64 = d.F64()
		case "Bool":
			out[i].b = d.Bool()
		case "Bytes":
			out[i].bytes = append([]byte(nil), d.Bytes()...)
		case "String":
			out[i].str = d.String()
		}
	}
	return out
}

func assertPayloadEqual(t *testing.T, want, got payload) {
	t.Helper()
	if want.kind != got.kind || want.name != got.name {
		t.Fatalf("payload metadata mismatch: want %+v got %+v", want, got)
	}
	switch want.kind {
	case "U8":
		if want.u8 != got.u8 {
			t.Errorf("%s: U8 want %d got %d", want.name, want.u8, got.u8)
		}
	case "U16":
		if want.u16 != got.u16 {
			t.Errorf("%s: U16 want %d got %d", want.name, want.u16, got.u16)
		}
	case "U32":
		if want.u32 != got.u32 {
			t.Errorf("%s: U32 want %d got %d", want.name, want.u32, got.u32)
		}
	case "U64":
		if want.u64 != got.u64 {
			t.Errorf("%s: U64 want %d got %d", want.name, want.u64, got.u64)
		}
	case "I32":
		if want.i32 != got.i32 {
			t.Errorf("%s: I32 want %d got %d", want.name, want.i32, got.i32)
		}
	case "I64":
		if want.i64 != got.i64 {
			t.Errorf("%s: I64 want %d got %d", want.name, want.i64, got.i64)
		}
	case "F32":
		// Use bit equality so NaN payloads (none here, but defensively) and
		// signed zero are distinguished from value equality.
		if math.Float32bits(want.f32) != math.Float32bits(got.f32) {
			t.Errorf("%s: F32 want %v got %v", want.name, want.f32, got.f32)
		}
	case "F64":
		if math.Float64bits(want.f64) != math.Float64bits(got.f64) {
			t.Errorf("%s: F64 want %v got %v", want.name, want.f64, got.f64)
		}
	case "Bool":
		if want.b != got.b {
			t.Errorf("%s: Bool want %v got %v", want.name, want.b, got.b)
		}
	case "Bytes":
		// nil and empty are wire-format-equivalent (both encode length 0 and
		// no payload). After a decode we always get back a slice — possibly
		// length 0 — so compare as length-0 sequences.
		if !bytes.Equal(want.bytes, got.bytes) {
			t.Errorf("%s: Bytes want %v got %v", want.name, want.bytes, got.bytes)
		}
	case "String":
		if want.str != got.str {
			t.Errorf("%s: String want %q got %q", want.name, want.str, got.str)
		}
	}
}

// TestEncoderParity confirms that the server-side stub and the runtime copy
// produce byte-identical encodings for every wire-format primitive.
func TestEncoderParity(t *testing.T) {
	items := parityPayloads()

	stubBytes := encodeStub(t, items)
	parityBytes := encodeParity(t, items)

	if !bytes.Equal(stubBytes, parityBytes) {
		// On mismatch, find the first divergence and show a small window.
		min := len(stubBytes)
		if len(parityBytes) < min {
			min = len(parityBytes)
		}
		div := -1
		for i := 0; i < min; i++ {
			if stubBytes[i] != parityBytes[i] {
				div = i
				break
			}
		}
		t.Fatalf("encoded byte streams diverged (stub=%d bytes parity=%d bytes, first diff at %d)",
			len(stubBytes), len(parityBytes), div)
	}
}

// TestDecoderParity reads the same encoded stream with both decoders and
// confirms the recovered values match the originals.
func TestDecoderParity(t *testing.T) {
	items := parityPayloads()
	buf := encodeStub(t, items)

	stubOut := readAll(stubReader{d: &wasm.Decoder{Buf: buf}}, items)
	parityOut := readAll(parityReader{d: &parity.Decoder{Buf: buf}}, items)

	for i, p := range items {
		assertPayloadEqual(t, p, stubOut[i])
		assertPayloadEqual(t, p, parityOut[i])
	}
}

// TestRoundTripParity proves the wire format is symmetric across the boundary:
// encode-stub/decode-parity AND encode-parity/decode-stub both recover the
// originals.
func TestRoundTripParity(t *testing.T) {
	items := parityPayloads()

	t.Run("stub_encode_parity_decode", func(t *testing.T) {
		buf := encodeStub(t, items)
		out := readAll(parityReader{d: &parity.Decoder{Buf: buf}}, items)
		for i, p := range items {
			assertPayloadEqual(t, p, out[i])
		}
	})

	t.Run("parity_encode_stub_decode", func(t *testing.T) {
		buf := encodeParity(t, items)
		out := readAll(stubReader{d: &wasm.Decoder{Buf: buf}}, items)
		for i, p := range items {
			assertPayloadEqual(t, p, out[i])
		}
	})
}

// TestFilesInSyncWarning is a tripwire that fires when codec.go and the
// parity copy drift significantly in size. It is intentionally fuzzy
// (10% tolerance) because formatting differences are acceptable; only a
// material change to one without the other should trip this.
func TestFilesInSyncWarning(t *testing.T) {
	// Locate both files relative to this test file's directory so the test
	// works regardless of `go test` cwd.
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not locate test file path")
	}
	wasmDir := filepath.Dir(thisFile)

	runtimePath := filepath.Join(wasmDir, "wasm-runtime", "runtime", "codec.go")
	parityPath := filepath.Join(wasmDir, "internal", "parity", "codec_runtime.go")

	runtimeSrc, err := os.ReadFile(runtimePath)
	if err != nil {
		t.Fatalf("read runtime codec.go: %v", err)
	}
	paritySrc, err := os.ReadFile(parityPath)
	if err != nil {
		t.Fatalf("read parity codec_runtime.go: %v", err)
	}

	runtimeBody := stripHeader(string(runtimeSrc))
	parityBody := stripHeader(string(paritySrc))

	runtimeLen := len(runtimeBody)
	parityLen := len(parityBody)

	if runtimeLen == 0 {
		t.Fatal("runtime codec.go body is empty after header strip")
	}

	diff := runtimeLen - parityLen
	if diff < 0 {
		diff = -diff
	}
	// 10% tolerance — enough to absorb gofmt churn but small enough to catch
	// any substantive divergence (a single new method is ~80–200 bytes).
	tolerance := runtimeLen / 10
	if diff > tolerance {
		t.Fatalf(
			"codec parity drift: runtime body is %d bytes, parity body is %d bytes (delta %d > %d)\n"+
				"Reminder: pkg/wasm/wasm-runtime/runtime/codec.go and pkg/wasm/internal/parity/codec_runtime.go\n"+
				"must be updated together. Mirror any change in BOTH files AND pkg/wasm/stubs.go.",
			runtimeLen, parityLen, diff, tolerance,
		)
	}
}

// stripHeader removes leading comment lines, a //go:build constraint line if
// present, and the `package …` line. It does not attempt to normalize formatting
// — the goal is to make the byte-length comparison robust to differences in
// package name / build tag, not to make the two files textually identical.
func stripHeader(src string) string {
	lines := strings.Split(src, "\n")
	i := 0
	for i < len(lines) {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			i++
			continue
		}
		if strings.HasPrefix(trimmed, "//") {
			i++
			continue
		}
		if strings.HasPrefix(trimmed, "package ") {
			i++
			continue
		}
		break
	}
	return strings.Join(lines[i:], "\n")
}

func TestEncoderDecoderRoundTrip_5MB(t *testing.T) {
	payload := make([]byte, 5*1024*1024)
	for i := range payload {
		payload[i] = byte(i)
	}
	e := wasm.NewEncoder(64)
	e.Bytes(payload)
	d := &wasm.Decoder{Buf: e.Buf}
	got := d.Bytes()
	if !bytes.Equal(got, payload) {
		t.Fatalf("round-trip mismatch: got %d bytes, want %d", len(got), len(payload))
	}
}
