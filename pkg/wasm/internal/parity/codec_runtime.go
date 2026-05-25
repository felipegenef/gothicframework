// This file is a byte-identical copy of pkg/wasm/wasm-runtime/runtime/codec.go
// used by codec_parity_test.go to validate that the server-side stub in
// pkg/wasm/stubs.go and the WASM-side runtime stay in sync. Any change
// to runtime/codec.go MUST be mirrored here AND in stubs.go.
package parity

import "math"

// Encoder writes a little-endian binary stream into Buf.
// Preallocate with NewEncoder to avoid repeated append growth.
type Encoder struct{ Buf []byte }

func NewEncoder(cap int) *Encoder { return &Encoder{Buf: make([]byte, 0, cap)} }

func (e *Encoder) U8(v uint8) { e.Buf = append(e.Buf, v) }
func (e *Encoder) U16(v uint16) {
	e.Buf = append(e.Buf, byte(v), byte(v>>8))
}
func (e *Encoder) U32(v uint32) {
	e.Buf = append(e.Buf, byte(v), byte(v>>8), byte(v>>16), byte(v>>24))
}
func (e *Encoder) U64(v uint64) {
	e.Buf = append(e.Buf, byte(v), byte(v>>8), byte(v>>16), byte(v>>24),
		byte(v>>32), byte(v>>40), byte(v>>48), byte(v>>56))
}
func (e *Encoder) I32(v int32)   { e.U32(uint32(v)) }
func (e *Encoder) I64(v int64)   { e.U64(uint64(v)) }
func (e *Encoder) F32(v float32) { e.U32(math.Float32bits(v)) }
func (e *Encoder) F64(v float64) { e.U64(math.Float64bits(v)) }
func (e *Encoder) Bool(v bool) {
	b := byte(0)
	if v {
		b = 1
	}
	e.Buf = append(e.Buf, b)
}
func (e *Encoder) Bytes(v []byte)  { e.U32(uint32(len(v))); e.Buf = append(e.Buf, v...) }
func (e *Encoder) String(v string) { e.U32(uint32(len(v))); e.Buf = append(e.Buf, v...) }

// Decoder reads a little-endian binary stream.
// Err accumulates the first decode error — check it once at the end rather than per call.
type Decoder struct {
	Buf []byte
	Pos int
	Err error
}

type decodeErr string

func (e decodeErr) Error() string { return string(e) }

const errUnderflow decodeErr = "codec: buffer underflow"

func (d *Decoder) need(n int) bool {
	if d.Err != nil {
		return false
	}
	if d.Pos+n > len(d.Buf) {
		d.Err = errUnderflow
		return false
	}
	return true
}

func (d *Decoder) U8() uint8 {
	if !d.need(1) {
		return 0
	}
	v := d.Buf[d.Pos]
	d.Pos++
	return v
}
func (d *Decoder) U16() uint16 {
	if !d.need(2) {
		return 0
	}
	v := uint16(d.Buf[d.Pos]) | uint16(d.Buf[d.Pos+1])<<8
	d.Pos += 2
	return v
}
func (d *Decoder) U32() uint32 {
	if !d.need(4) {
		return 0
	}
	v := uint32(d.Buf[d.Pos]) | uint32(d.Buf[d.Pos+1])<<8 |
		uint32(d.Buf[d.Pos+2])<<16 | uint32(d.Buf[d.Pos+3])<<24
	d.Pos += 4
	return v
}
func (d *Decoder) U64() uint64 {
	if !d.need(8) {
		return 0
	}
	v := uint64(d.Buf[d.Pos]) | uint64(d.Buf[d.Pos+1])<<8 |
		uint64(d.Buf[d.Pos+2])<<16 | uint64(d.Buf[d.Pos+3])<<24 |
		uint64(d.Buf[d.Pos+4])<<32 | uint64(d.Buf[d.Pos+5])<<40 |
		uint64(d.Buf[d.Pos+6])<<48 | uint64(d.Buf[d.Pos+7])<<56
	d.Pos += 8
	return v
}
func (d *Decoder) I32() int32   { return int32(d.U32()) }
func (d *Decoder) I64() int64   { return int64(d.U64()) }
func (d *Decoder) F32() float32 { return math.Float32frombits(d.U32()) }
func (d *Decoder) F64() float64 { return math.Float64frombits(d.U64()) }
func (d *Decoder) Bool() bool   { return d.U8() != 0 }
func (d *Decoder) Bytes() []byte {
	n := d.U32()
	if !d.need(int(n)) {
		return nil
	}
	v := d.Buf[d.Pos : d.Pos+int(n)]
	d.Pos += int(n)
	return v
}
func (d *Decoder) String() string {
	n := d.U32()
	if !d.need(int(n)) {
		return ""
	}
	v := string(d.Buf[d.Pos : d.Pos+int(n)])
	d.Pos += int(n)
	return v
}

// hex helpers — inline to avoid importing encoding/hex.
const hextable = "0123456789abcdef"

func HexEncode(src []byte) string {
	dst := make([]byte, len(src)*2)
	for i, b := range src {
		dst[i*2] = hextable[b>>4]
		dst[i*2+1] = hextable[b&0xf]
	}
	return string(dst)
}

func HexDecode(s string) []byte {
	if len(s)%2 != 0 {
		return nil
	}
	dst := make([]byte, len(s)/2)
	for i := 0; i < len(s); i += 2 {
		dst[i/2] = unhex(s[i])<<4 | unhex(s[i+1])
	}
	return dst
}

func unhex(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	}
	return 0
}
