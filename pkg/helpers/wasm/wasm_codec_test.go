package helpers

import (
	"strings"
	"testing"
)

func TestResolveType_KnownAlias(t *testing.T) {
	aliases := map[string]string{"MyScore": "int"}
	if got := resolveType("MyScore", aliases); got != "int" {
		t.Errorf("resolveType alias: got %q, want %q", got, "int")
	}
}

func TestResolveType_Unknown(t *testing.T) {
	if got := resolveType("int", map[string]string{}); got != "int" {
		t.Errorf("resolveType passthrough: got %q, want %q", got, "int")
	}
}

func TestKindOf_Primitive(t *testing.T) {
	cases := []string{"int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64",
		"float32", "float64", "bool", "string", "byte", "rune", "time.Time"}
	for _, c := range cases {
		if got := kindOf(c, nil); got != kindPrimitive {
			t.Errorf("kindOf(%q): got %v, want kindPrimitive", c, got)
		}
	}
}

func TestKindOf_Bytes(t *testing.T) {
	if got := kindOf("[]byte", nil); got != kindBytes {
		t.Errorf("kindOf([]byte): got %v, want kindBytes", got)
	}
}

func TestKindOf_Slice(t *testing.T) {
	if got := kindOf("[]string", nil); got != kindSlice {
		t.Errorf("kindOf([]string): got %v, want kindSlice", got)
	}
	if got := kindOf("[][]int", nil); got != kindSlice {
		t.Errorf("kindOf([][]int): got %v, want kindSlice", got)
	}
}

func TestKindOf_Map(t *testing.T) {
	if got := kindOf("map[string]int", nil); got != kindMap {
		t.Errorf("kindOf(map[string]int): got %v, want kindMap", got)
	}
}

func TestKindOf_Pointer(t *testing.T) {
	structs := map[string]bool{"Item": true}
	if got := kindOf("*Item", structs); got != kindPointer {
		t.Errorf("kindOf(*Item): got %v, want kindPointer", got)
	}
	if got := kindOf("*int", nil); got != kindPointer {
		t.Errorf("kindOf(*int): got %v, want kindPointer", got)
	}
}

func TestKindOf_Struct(t *testing.T) {
	structs := map[string]bool{"Item": true}
	if got := kindOf("Item", structs); got != kindStruct {
		t.Errorf("kindOf(Item): got %v, want kindStruct", got)
	}
}

func TestKindOf_Unknown(t *testing.T) {
	if got := kindOf("Unknown", map[string]bool{}); got != kindUnknown {
		t.Errorf("kindOf(Unknown): got %v, want kindUnknown", got)
	}
}

func TestPrimitiveCodec_IntSnapshot(t *testing.T) {
	enc, dec := primitiveCodec("int", "v.X")
	if enc != "e.I64(int64(v.X))" {
		t.Errorf("primitiveCodec(int) enc: got %q", enc)
	}
	if dec != "v.X = int(d.I64())" {
		t.Errorf("primitiveCodec(int) dec: got %q", dec)
	}
}

func TestPrimitiveCodec_StringSnapshot(t *testing.T) {
	enc, dec := primitiveCodec("string", "v.S")
	if enc != "e.String(string(v.S))" {
		t.Errorf("primitiveCodec(string) enc: got %q", enc)
	}
	if dec != "v.S = string(d.String())" {
		t.Errorf("primitiveCodec(string) dec: got %q", dec)
	}
}

func TestCodecLines_IntField(t *testing.T) {
	h := DefaultWasmHelper()
	enc, dec, err := h.codecLines(fieldInfo{Name: "X", Type: "int"}, nil, nil)
	if err != nil {
		t.Fatalf("codecLines int: %v", err)
	}
	if enc != "e.I64(int64(v.X))" {
		t.Errorf("codecLines int enc: got %q", enc)
	}
	if dec != "v.X = int(d.I64())" {
		t.Errorf("codecLines int dec: got %q", dec)
	}
}

func TestCodecLines_BytesField(t *testing.T) {
	h := DefaultWasmHelper()
	enc, dec, err := h.codecLines(fieldInfo{Name: "Data", Type: "[]byte"}, nil, nil)
	if err != nil {
		t.Fatalf("codecLines []byte: %v", err)
	}
	if enc != "e.Bytes(v.Data)" {
		t.Errorf("codecLines []byte enc: got %q", enc)
	}
	if dec != "v.Data = d.Bytes()" {
		t.Errorf("codecLines []byte dec: got %q", dec)
	}
}

func TestCodecLines_StructField(t *testing.T) {
	h := DefaultWasmHelper()
	structs := map[string]bool{"Item": true}
	enc, dec, err := h.codecLines(fieldInfo{Name: "Sub", Type: "Item"}, structs, nil)
	if err != nil {
		t.Fatalf("codecLines struct: %v", err)
	}
	if enc != "_encode_Item(v.Sub, e)" {
		t.Errorf("codecLines struct enc: got %q", enc)
	}
	if dec != "v.Sub = _decode_Item(d)" {
		t.Errorf("codecLines struct dec: got %q", dec)
	}
}

func TestCodecLines_SliceOfString(t *testing.T) {
	h := DefaultWasmHelper()
	enc, _, err := h.codecLines(fieldInfo{Name: "Labels", Type: "[]string"}, nil, nil)
	if err != nil {
		t.Fatalf("codecLines []string: %v", err)
	}
	// Just make sure we got a non-empty slice-style block.
	if !strings.Contains(enc, "e.U32(uint32(len(v.Labels)))") {
		t.Errorf("codecLines []string enc should contain length prefix: %q", enc)
	}
	if !strings.Contains(enc, "e.String(string(_item))") {
		t.Errorf("codecLines []string enc should encode each item as string: %q", enc)
	}
}

func TestCodecLines_AliasToInt(t *testing.T) {
	h := DefaultWasmHelper()
	aliases := map[string]string{"MyScore": "int"}
	enc, dec, err := h.codecLines(fieldInfo{Name: "Score", Type: "MyScore"}, nil, aliases)
	if err != nil {
		t.Fatalf("codecLines alias: %v", err)
	}
	// Enc looks like an int encoder: e.I64(int64(v.Score))
	if enc != "e.I64(int64(v.Score))" {
		t.Errorf("codecLines alias enc: got %q", enc)
	}
	// Dec should cast back to the alias.
	if !strings.Contains(dec, "v.Score = MyScore(") {
		t.Errorf("codecLines alias dec should cast back to MyScore: %q", dec)
	}
}

func TestCaptureLines_IntField(t *testing.T) {
	h := DefaultWasmHelper()
	body, err := h.captureLines(fieldInfo{Name: "X", Type: "int"}, nil, nil)
	if err != nil {
		t.Fatalf("captureLines int: %v", err)
	}
	if !strings.HasPrefix(body, "start := d.Pos") {
		t.Errorf("captureLines int: body must start with `start := d.Pos`, got %q", body)
	}
	if !strings.Contains(body, "d.I64()") {
		t.Errorf("captureLines int: expected d.I64() call, got %q", body)
	}
	if !strings.HasSuffix(body, "return d.Buf[start:d.Pos]") {
		t.Errorf("captureLines int: body must end with zero-copy slice return, got %q", body)
	}
	if strings.Contains(body, "v.X") {
		t.Errorf("captureLines int: body must not reference v.X, got %q", body)
	}
}

func TestCaptureLines_I32Tag(t *testing.T) {
	h := DefaultWasmHelper()
	body, err := h.captureLines(fieldInfo{Name: "Pings", Type: "int", GothicTag: "i32"}, nil, nil)
	if err != nil {
		t.Fatalf("captureLines i32 tag: %v", err)
	}
	if !strings.Contains(body, "d.I32()") {
		t.Errorf("captureLines i32 tag: expected d.I32() call, got %q", body)
	}
	if strings.Contains(body, "v.Pings") {
		t.Errorf("captureLines i32 tag: body must not reference v.Pings, got %q", body)
	}
}

func TestCaptureLines_StringField(t *testing.T) {
	h := DefaultWasmHelper()
	body, err := h.captureLines(fieldInfo{Name: "S", Type: "string"}, nil, nil)
	if err != nil {
		t.Fatalf("captureLines string: %v", err)
	}
	if !strings.Contains(body, "d.String()") {
		t.Errorf("captureLines string: expected d.String() call, got %q", body)
	}
	if strings.Contains(body, "v.S") {
		t.Errorf("captureLines string: body must not reference v.S, got %q", body)
	}
}

func TestCaptureLines_StructField(t *testing.T) {
	h := DefaultWasmHelper()
	structs := map[string]bool{"Item": true}
	body, err := h.captureLines(fieldInfo{Name: "Sub", Type: "Item"}, structs, nil)
	if err != nil {
		t.Fatalf("captureLines struct: %v", err)
	}
	if !strings.Contains(body, "_decode_Item(d)") {
		t.Errorf("captureLines struct: expected _decode_Item(d) call, got %q", body)
	}
	if strings.Contains(body, "v.Sub") {
		t.Errorf("captureLines struct: body must not reference v.Sub, got %q", body)
	}
}

func TestCaptureLines_SliceField(t *testing.T) {
	h := DefaultWasmHelper()
	body, err := h.captureLines(fieldInfo{Name: "Tags", Type: "[]string"}, nil, nil)
	if err != nil {
		t.Fatalf("captureLines []string: %v", err)
	}
	if !strings.Contains(body, "int(d.U32())") {
		t.Errorf("captureLines []string: expected length prefix `int(d.U32())`, got %q", body)
	}
	if !strings.Contains(body, "d.String()") {
		t.Errorf("captureLines []string: expected element decode `d.String()`, got %q", body)
	}
	if strings.Contains(body, "v.Tags") {
		t.Errorf("captureLines []string: body must not reference v.Tags, got %q", body)
	}
}

func TestCaptureLines_BytesField(t *testing.T) {
	h := DefaultWasmHelper()
	body, err := h.captureLines(fieldInfo{Name: "Data", Type: "[]byte"}, nil, nil)
	if err != nil {
		t.Fatalf("captureLines []byte: %v", err)
	}
	if !strings.Contains(body, "d.Bytes()") {
		t.Errorf("captureLines []byte: expected d.Bytes() call, got %q", body)
	}
	if strings.Contains(body, "v.Data") {
		t.Errorf("captureLines []byte: body must not reference v.Data, got %q", body)
	}
}

// TestCaptureLines_AllPrimitives iterates every primitive type supported by
// the codec and asserts each capture body has the canonical wrapper:
// starts with `start := d.Pos` and ends with the append-return that slices
// out the raw bytes consumed by the decoder. This protects the per-field
// capture pipeline from drift across all primitive kinds.
func TestCaptureLines_AllPrimitives(t *testing.T) {
	h := DefaultWasmHelper()
	prims := []string{
		"bool", "string",
		"int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64",
		"float32", "float64",
		"rune", "byte", "time.Time",
	}
	for _, typ := range prims {
		body, err := h.captureLines(fieldInfo{Name: "F", Type: typ}, nil, nil)
		if err != nil {
			t.Fatalf("captureLines(%s): %v", typ, err)
		}
		if !strings.HasPrefix(body, "start := d.Pos") {
			t.Errorf("captureLines(%s): missing `start := d.Pos` prefix, got %q", typ, body)
		}
		if !strings.HasSuffix(body, "return d.Buf[start:d.Pos]") {
			t.Errorf("captureLines(%s): missing zero-copy slice return suffix, got %q", typ, body)
		}
		if strings.Contains(body, "v.F") {
			t.Errorf("captureLines(%s): body must not reference v.F, got %q", typ, body)
		}
	}
}

func TestCaptureLines_PointerToStruct(t *testing.T) {
	h := DefaultWasmHelper()
	structs := map[string]bool{"Item": true}
	body, err := h.captureLines(fieldInfo{Name: "Ptr", Type: "*Item"}, structs, nil)
	if err != nil {
		t.Fatalf("captureLines *Item: %v", err)
	}
	if !strings.HasPrefix(body, "start := d.Pos") {
		t.Errorf("captureLines *Item: missing `start := d.Pos` prefix, got %q", body)
	}
	if !strings.Contains(body, "d.U8()") {
		t.Errorf("captureLines *Item: expected nil-tag check via d.U8(), got %q", body)
	}
	if !strings.HasSuffix(body, "return d.Buf[start:d.Pos]") {
		t.Errorf("captureLines *Item: missing zero-copy slice return suffix, got %q", body)
	}
	if strings.Contains(body, "v.Ptr") {
		t.Errorf("captureLines *Item: body must not reference v.Ptr, got %q", body)
	}
}

func TestCaptureLines_MapStringStruct(t *testing.T) {
	h := DefaultWasmHelper()
	structs := map[string]bool{"Item": true}
	body, err := h.captureLines(fieldInfo{Name: "M", Type: "map[string]Item"}, structs, nil)
	if err != nil {
		t.Fatalf("captureLines map[string]Item: %v", err)
	}
	if !strings.HasPrefix(body, "start := d.Pos") {
		t.Errorf("captureLines map: missing `start := d.Pos` prefix, got %q", body)
	}
	if !strings.Contains(body, "d.U32()") {
		t.Errorf("captureLines map: expected length prefix d.U32(), got %q", body)
	}
	if !strings.Contains(body, "d.String()") {
		t.Errorf("captureLines map: expected key decode d.String(), got %q", body)
	}
	if !strings.Contains(body, "_decode_Item(d)") {
		t.Errorf("captureLines map: expected value decode _decode_Item(d), got %q", body)
	}
	if !strings.HasSuffix(body, "return d.Buf[start:d.Pos]") {
		t.Errorf("captureLines map: missing zero-copy slice return suffix, got %q", body)
	}
	if strings.Contains(body, "v.M") {
		t.Errorf("captureLines map: body must not reference v.M, got %q", body)
	}
}

func TestCodecLines_UnknownTypeReturnsError(t *testing.T) {
	h := DefaultWasmHelper()
	_, _, err := h.codecLines(fieldInfo{Name: "X", Type: "Mystery"}, nil, nil)
	if err == nil {
		t.Errorf("expected error for unknown type, got nil")
	}
}
