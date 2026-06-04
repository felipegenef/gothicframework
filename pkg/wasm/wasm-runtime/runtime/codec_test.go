package runtime

import (
	"bytes"
	"math"
	"testing"
)

// round-trip helpers
func roundtripU8(t *testing.T, v uint8) {
	t.Helper()
	e := NewEncoder(8)
	e.U8(v)
	d := &Decoder{Buf: e.Buf}
	if got := d.U8(); got != v {
		t.Errorf("U8 round-trip: got %v want %v", got, v)
	}
	if d.Err != nil {
		t.Errorf("unexpected error: %v", d.Err)
	}
}

func TestCodecRoundTrip(t *testing.T) {
	t.Run("U8", func(t *testing.T) {
		for _, v := range []uint8{0, 1, 127, 255} {
			roundtripU8(t, v)
		}
	})

	t.Run("U16", func(t *testing.T) {
		for _, v := range []uint16{0, 1, 256, 0xFFFF} {
			e := NewEncoder(8)
			e.U16(v)
			d := &Decoder{Buf: e.Buf}
			if got := d.U16(); got != v {
				t.Errorf("U16 round-trip: got %v want %v", got, v)
			}
		}
	})

	t.Run("U32", func(t *testing.T) {
		for _, v := range []uint32{0, 1, 0x12345678, math.MaxUint32} {
			e := NewEncoder(8)
			e.U32(v)
			d := &Decoder{Buf: e.Buf}
			if got := d.U32(); got != v {
				t.Errorf("U32 round-trip: got %v want %v", got, v)
			}
		}
	})

	t.Run("U64", func(t *testing.T) {
		for _, v := range []uint64{0, 1, 0x0102030405060708, math.MaxUint64} {
			e := NewEncoder(16)
			e.U64(v)
			d := &Decoder{Buf: e.Buf}
			if got := d.U64(); got != v {
				t.Errorf("U64 round-trip: got %v want %v", got, v)
			}
		}
	})

	t.Run("I32", func(t *testing.T) {
		for _, v := range []int32{-1, 0, 1, math.MinInt32, math.MaxInt32} {
			e := NewEncoder(8)
			e.I32(v)
			d := &Decoder{Buf: e.Buf}
			if got := d.I32(); got != v {
				t.Errorf("I32 round-trip: got %v want %v", got, v)
			}
		}
	})

	t.Run("I64", func(t *testing.T) {
		for _, v := range []int64{-1, 0, 1, math.MinInt64, math.MaxInt64} {
			e := NewEncoder(16)
			e.I64(v)
			d := &Decoder{Buf: e.Buf}
			if got := d.I64(); got != v {
				t.Errorf("I64 round-trip: got %v want %v", got, v)
			}
		}
	})

	t.Run("F32", func(t *testing.T) {
		for _, v := range []float32{0, 1, -1, 3.14, float32(math.Inf(1)), float32(math.Inf(-1)), float32(math.NaN())} {
			e := NewEncoder(8)
			e.F32(v)
			d := &Decoder{Buf: e.Buf}
			got := d.F32()
			if math.IsNaN(float64(v)) {
				if !math.IsNaN(float64(got)) {
					t.Errorf("F32 NaN round-trip: got %v", got)
				}
			} else if got != v {
				t.Errorf("F32 round-trip: got %v want %v", got, v)
			}
		}
	})

	t.Run("F64", func(t *testing.T) {
		for _, v := range []float64{0, 1, -1, 3.14159, math.Inf(1), math.Inf(-1), math.NaN()} {
			e := NewEncoder(16)
			e.F64(v)
			d := &Decoder{Buf: e.Buf}
			got := d.F64()
			if math.IsNaN(v) {
				if !math.IsNaN(got) {
					t.Errorf("F64 NaN round-trip: got %v", got)
				}
			} else if got != v {
				t.Errorf("F64 round-trip: got %v want %v", got, v)
			}
		}
	})

	t.Run("Bool", func(t *testing.T) {
		for _, v := range []bool{true, false} {
			e := NewEncoder(4)
			e.Bool(v)
			d := &Decoder{Buf: e.Buf}
			if got := d.Bool(); got != v {
				t.Errorf("Bool round-trip: got %v want %v", got, v)
			}
		}
	})

	t.Run("Bytes", func(t *testing.T) {
		for _, v := range [][]byte{nil, {}, {1, 2, 3}, make([]byte, 100)} {
			e := NewEncoder(16)
			e.Bytes(v)
			d := &Decoder{Buf: e.Buf}
			got := d.Bytes()
			if len(got) != len(v) {
				t.Errorf("Bytes round-trip len: got %d want %d", len(got), len(v))
			}
		}
	})

	t.Run("String", func(t *testing.T) {
		for _, v := range []string{"", "hello", "unicode: 日本語"} {
			e := NewEncoder(64)
			e.String(v)
			d := &Decoder{Buf: e.Buf}
			if got := d.String(); got != v {
				t.Errorf("String round-trip: got %q want %q", got, v)
			}
		}
	})

	t.Run("U32 little-endian byte order", func(t *testing.T) {
		e := NewEncoder(8)
		e.U32(0x01020304)
		want := []byte{0x04, 0x03, 0x02, 0x01}
		if !bytes.Equal(e.Buf, want) {
			t.Errorf("U32 byte order: got %v want %v", e.Buf, want)
		}
	})
}

func TestDecoderUnderflow(t *testing.T) {
	tests := []struct {
		name string
		fn   func(*Decoder)
	}{
		{"U8", func(d *Decoder) { d.U8() }},
		{"U16", func(d *Decoder) { d.U16() }},
		{"U32", func(d *Decoder) { d.U32() }},
		{"U64", func(d *Decoder) { d.U64() }},
		{"Bool", func(d *Decoder) { d.Bool() }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := &Decoder{Buf: []byte{}} // empty
			tc.fn(d)
			if d.Err == nil {
				t.Errorf("%s: expected underflow error, got nil", tc.name)
			}
		})
	}

	t.Run("Bytes underflow on payload", func(t *testing.T) {
		// encode length=5 but provide only 2 bytes of payload
		e := NewEncoder(8)
		e.U32(5)
		e.Buf = append(e.Buf, 0x01, 0x02) // only 2 bytes instead of 5
		d := &Decoder{Buf: e.Buf}
		if got := d.Bytes(); got != nil {
			t.Errorf("expected nil on underflow, got %v", got)
		}
		if d.Err == nil {
			t.Errorf("expected error, got nil")
		}
	})

	t.Run("String underflow on payload", func(t *testing.T) {
		e := NewEncoder(8)
		e.U32(5)
		e.Buf = append(e.Buf, 'a', 'b') // only 2 bytes
		d := &Decoder{Buf: e.Buf}
		if got := d.String(); got != "" {
			t.Errorf("expected empty on underflow, got %q", got)
		}
		if d.Err == nil {
			t.Errorf("expected error, got nil")
		}
	})

	t.Run("error sticky", func(t *testing.T) {
		d := &Decoder{Buf: []byte{}}
		d.U8() // sets Err
		firstErr := d.Err
		d.U8() // should be no-op
		if d.Err != firstErr {
			t.Errorf("error changed after second call: %v", d.Err)
		}
	})
}

func TestHexEncodeDecode(t *testing.T) {
	tests := []struct {
		src []byte
		hex string
	}{
		{[]byte{}, ""},
		{[]byte{0x00}, "00"},
		{[]byte{0xff}, "ff"},
		{[]byte{0x0f, 0xab}, "0fab"},
		{[]byte{0, 1, 2, 254, 255}, "000102feff"},
	}
	for _, tc := range tests {
		t.Run(tc.hex, func(t *testing.T) {
			if got := HexEncode(tc.src); got != tc.hex {
				t.Errorf("HexEncode: got %q want %q", got, tc.hex)
			}
			if got := HexDecode(tc.hex); string(got) != string(tc.src) {
				t.Errorf("HexDecode: got %v want %v", got, tc.src)
			}
		})
	}

	t.Run("uppercase input", func(t *testing.T) {
		if got := HexDecode("0FAB"); string(got) != string([]byte{0x0f, 0xab}) {
			t.Errorf("HexDecode uppercase: got %v", got)
		}
	})

	t.Run("odd length returns nil", func(t *testing.T) {
		if got := HexDecode("abc"); got != nil {
			t.Errorf("expected nil for odd-length input, got %v", got)
		}
	})
}
