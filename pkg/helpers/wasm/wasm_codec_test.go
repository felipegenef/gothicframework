package helpers

import (
	"flag"
	"go/parser"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var updateGolden = flag.Bool("update", false, "rewrite codec golden files")

// testFieldInfo parses typeStr as a Go expression and produces a fieldInfo with
// a populated TypeRef. Used by tests to construct fieldInfo values that match
// what parseStructsFromSource would build at runtime.
func testFieldInfo(name, typeStr string) fieldInfo {
	expr, err := parser.ParseExpr(typeStr)
	if err != nil {
		panic("testFieldInfo: " + err.Error())
	}
	tref, _ := typeRefFromExpr(expr)
	return fieldInfo{Name: name, Type: typeStr, TypeRef: tref}
}

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

// testTypeRef parses a Go type expression string into a typeRef. Used by tests
// that want to assert kindOf classification.
func testTypeRef(typeStr string) typeRef {
	expr, err := parser.ParseExpr(typeStr)
	if err != nil {
		panic("testTypeRef: " + err.Error())
	}
	tref, err := typeRefFromExpr(expr)
	if err != nil {
		panic("testTypeRef: " + err.Error())
	}
	return tref
}

func TestKindOf_Primitive(t *testing.T) {
	cases := []string{"int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64",
		"float32", "float64", "bool", "string", "byte", "rune", "time.Time"}
	for _, c := range cases {
		if got := kindOf(testTypeRef(c), nil); got != kindPrimitive {
			t.Errorf("kindOf(%q): got %v, want kindPrimitive", c, got)
		}
	}
}

func TestKindOf_Bytes(t *testing.T) {
	if got := kindOf(testTypeRef("[]byte"), nil); got != kindBytes {
		t.Errorf("kindOf([]byte): got %v, want kindBytes", got)
	}
}

func TestKindOf_Slice(t *testing.T) {
	if got := kindOf(testTypeRef("[]string"), nil); got != kindSlice {
		t.Errorf("kindOf([]string): got %v, want kindSlice", got)
	}
	if got := kindOf(testTypeRef("[][]int"), nil); got != kindSlice {
		t.Errorf("kindOf([][]int): got %v, want kindSlice", got)
	}
}

func TestKindOf_Map(t *testing.T) {
	if got := kindOf(testTypeRef("map[string]int"), nil); got != kindMap {
		t.Errorf("kindOf(map[string]int): got %v, want kindMap", got)
	}
}

func TestKindOf_Pointer(t *testing.T) {
	structs := map[string]bool{"Item": true}
	if got := kindOf(testTypeRef("*Item"), structs); got != kindPointer {
		t.Errorf("kindOf(*Item): got %v, want kindPointer", got)
	}
	if got := kindOf(testTypeRef("*int"), nil); got != kindPointer {
		t.Errorf("kindOf(*int): got %v, want kindPointer", got)
	}
}

func TestKindOf_Struct(t *testing.T) {
	structs := map[string]bool{"Item": true}
	if got := kindOf(testTypeRef("Item"), structs); got != kindStruct {
		t.Errorf("kindOf(Item): got %v, want kindStruct", got)
	}
}

func TestKindOf_Unknown(t *testing.T) {
	if got := kindOf(testTypeRef("Unknown"), map[string]bool{}); got != kindUnknown {
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
	enc, dec, err := h.codecLines(testFieldInfo("X", "int"), nil, nil, nil)
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
	enc, dec, err := h.codecLines(testFieldInfo("Data", "[]byte"), nil, nil, nil)
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
	enc, dec, err := h.codecLines(testFieldInfo("Sub", "Item"), structs, nil, nil)
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
	enc, _, err := h.codecLines(testFieldInfo("Labels", "[]string"), nil, nil, nil)
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
	refAliases := map[string]typeRef{"MyScore": Named{Name: "int"}}
	enc, dec, err := h.codecLines(testFieldInfo("Score", "MyScore"), nil, aliases, refAliases)
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
	body, err := h.captureLines(testFieldInfo("X", "int"), nil, nil, nil)
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
	fi := testFieldInfo("Pings", "int")
	fi.GothicTag = "i32"
	body, err := h.captureLines(fi, nil, nil, nil)
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
	body, err := h.captureLines(testFieldInfo("S", "string"), nil, nil, nil)
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
	body, err := h.captureLines(testFieldInfo("Sub", "Item"), structs, nil, nil)
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
	body, err := h.captureLines(testFieldInfo("Tags", "[]string"), nil, nil, nil)
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
	body, err := h.captureLines(testFieldInfo("Data", "[]byte"), nil, nil, nil)
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
		body, err := h.captureLines(testFieldInfo("F", typ), nil, nil, nil)
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
	body, err := h.captureLines(testFieldInfo("Ptr", "*Item"), structs, nil, nil)
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
	body, err := h.captureLines(testFieldInfo("M", "map[string]Item"), structs, nil, nil)
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

func TestMapCodecLines_NestedMap(t *testing.T) {
	h := DefaultWasmHelper()
	fi := fieldInfo{
		Name:    "Matrix",
		Type:    "map[string]map[string]int",
		TypeRef: MapOf{Key: Named{"string"}, Val: MapOf{Key: Named{"string"}, Val: Named{"int"}}},
	}
	enc, dec, err := h.codecLines(fi, nil, nil, nil)
	if err != nil {
		t.Fatalf("codecLines nested map: %v", err)
	}
	if enc == "" || dec == "" {
		t.Fatalf("nested map produced empty codec lines: enc=%q dec=%q", enc, dec)
	}
	// Outer loop uses _k/_v, inner loop must use _k2/_v2 to avoid shadowing.
	if !strings.Contains(enc, "_k2") || !strings.Contains(enc, "_v2") {
		t.Errorf("nested map enc should use _k2/_v2 for inner loop: %q", enc)
	}
	if !strings.Contains(dec, "_k2") || !strings.Contains(dec, "_v2") {
		t.Errorf("nested map dec should use _k2/_v2 for inner loop: %q", dec)
	}
	if !strings.Contains(dec, "make(map[string]map[string]int") {
		t.Errorf("nested map dec should make outer map: %q", dec)
	}
	if !strings.Contains(dec, "make(map[string]int") {
		t.Errorf("nested map dec should make inner map: %q", dec)
	}
}

func TestCodecLines_UnknownTypeReturnsError(t *testing.T) {
	h := DefaultWasmHelper()
	_, _, err := h.codecLines(testFieldInfo("X", "Mystery"), nil, nil, nil)
	if err == nil {
		t.Errorf("expected error for unknown type, got nil")
	}
}

// TestCodecGolden parses each fixture under testdata/codec_golden/, runs the
// codec pipeline, and compares the rendered codec lines against a golden file.
// Run with `-update` to (re)generate the golden files.
func TestCodecGolden(t *testing.T) {
	h := &WasmHelper{}
	goldenDir := "testdata/codec_golden"
	entries, err := os.ReadDir(goldenDir)
	if err != nil {
		t.Fatalf("reading golden dir: %v", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		t.Run(entry.Name(), func(t *testing.T) {
			inputPath := filepath.Join(goldenDir, entry.Name(), "input.go")
			goldenPath := filepath.Join(goldenDir, entry.Name(), "expected_codec.go.golden")

			data, err := os.ReadFile(inputPath)
			if err != nil {
				t.Fatalf("reading input: %v", err)
			}

			structs, aliases, refAliases := h.parseStructsFromSource(string(data))
			codecData, err := h.buildCodecData(structs, aliases, refAliases)
			if err != nil {
				t.Fatalf("buildCodecData: %v", err)
			}

			var rendered strings.Builder
			for _, sd := range codecData {
				rendered.WriteString("// === " + sd.Name + " ===\n")
				for _, field := range sd.Fields {
					rendered.WriteString("// ENCODE " + field.Name + ":\n")
					rendered.WriteString(field.EncLine + "\n")
					rendered.WriteString("// DECODE " + field.Name + ":\n")
					rendered.WriteString(field.DecLine + "\n")
				}
			}

			got := rendered.String()

			if *updateGolden {
				if err := os.WriteFile(goldenPath, []byte(got), 0644); err != nil {
					t.Fatalf("writing golden: %v", err)
				}
				return
			}

			wantBytes, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("reading golden (run with -update to create): %v", err)
			}

			if string(wantBytes) != got {
				wantLines := strings.Split(string(wantBytes), "\n")
				gotLines := strings.Split(got, "\n")
				for i := 0; i < len(wantLines) || i < len(gotLines); i++ {
					var w, g string
					if i < len(wantLines) {
						w = wantLines[i]
					}
					if i < len(gotLines) {
						g = gotLines[i]
					}
					if w != g {
						t.Errorf("line %d:\n  want: %q\n   got: %q", i+1, w, g)
					}
				}
			}
		})
	}
}

func TestDropMakeAssignments(t *testing.T) {
	tests := []struct {
		name   string
		src    string
		prefix string
	}{
		{name: "drops matching make", src: `v.F = make([]Foo, _n); rest()`, prefix: "v.F"},
		{name: "does not drop non-matching lhs", src: `_ = make([]string, 1); rest()`, prefix: "v.F"},
		{name: "drops only matching field when multiple makes", src: `v.F = make([]Foo, _n); v.G = make([]Bar, _n); rest()`, prefix: "v.F"},
		{name: "adversarial: make arg contains closing paren in string", src: `v.F = make([]T, len("has)here")); rest()`, prefix: "v.F"},
		{name: "no make calls returns src unchanged", src: `v.F = 42; other()`, prefix: "v.F"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dropMakeAssignments(tt.src, tt.prefix)
			switch tt.name {
			case "drops matching make":
				if strings.Contains(got, "make(") {
					t.Errorf("make() should have been dropped, got: %q", got)
				}
				if !strings.Contains(got, "rest()") {
					t.Errorf("rest() should be preserved, got: %q", got)
				}
			case "does not drop non-matching lhs":
				if !strings.Contains(got, "make(") {
					t.Errorf("non-matching make should be preserved, got: %q", got)
				}
			case "drops only matching field when multiple makes":
				if strings.Contains(got, "v.F ") || strings.Contains(got, "v.F=") {
					t.Errorf("v.F make should be dropped, got: %q", got)
				}
				if !strings.Contains(got, "v.G") {
					t.Errorf("v.G make should be kept, got: %q", got)
				}
			case "adversarial: make arg contains closing paren in string":
				if strings.Contains(got, "v.F") {
					t.Errorf("v.F make should be dropped even with paren in string, got: %q", got)
				}
				if !strings.Contains(got, "rest()") {
					t.Errorf("rest() should be preserved, got: %q", got)
				}
			case "no make calls returns src unchanged":
				if !strings.Contains(got, "v.F = 42") {
					t.Errorf("non-make assignment should be preserved, got: %q", got)
				}
			}
		})
	}
}
