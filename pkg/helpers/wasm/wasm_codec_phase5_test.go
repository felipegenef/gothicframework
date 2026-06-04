package helpers

import (
	"strings"
	"testing"
)

func structNamesSet(names ...string) map[string]bool {
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	return m
}

// TestCodecLines_AllKinds drives codecLines over every supported field kind and
// asserts both encode and decode lines are produced.
func TestCodecLines_AllKinds(t *testing.T) {
	h := DefaultWasmHelper()
	names := structNamesSet("Item")

	cases := []struct {
		name    string
		fi      fieldInfo
		wantEnc string // substring expected in enc
	}{
		{"primitive int", testFieldInfo("N", "int"), "e.I64"},
		{"primitive string", testFieldInfo("S", "string"), "e.String"},
		{"bool", testFieldInfo("B", "bool"), "e.Bool"},
		{"float64", testFieldInfo("F", "float64"), "e.F64"},
		{"time", testFieldInfo("T", "time.Time"), "UnixNano"},
		{"bytes", testFieldInfo("Data", "[]byte"), "e.Bytes"},
		{"slice of primitive", testFieldInfo("Xs", "[]int"), "for"},
		{"slice of struct", testFieldInfo("Items", "[]Item"), "_encode_Item"},
		{"map", testFieldInfo("M", "map[string]int"), "for"},
		{"pointer", testFieldInfo("P", "*Item"), "if"},
		{"struct", testFieldInfo("Sub", "Item"), "_encode_Item"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			enc, dec, err := h.codecLines(c.fi, names, map[string]string{}, nil)
			if err != nil {
				t.Fatalf("codecLines(%s): %v", c.name, err)
			}
			if enc == "" || dec == "" {
				t.Fatalf("codecLines(%s): empty enc/dec", c.name)
			}
			if !strings.Contains(enc, c.wantEnc) {
				t.Errorf("codecLines(%s) enc = %q, want substr %q", c.name, enc, c.wantEnc)
			}
		})
	}
}

func TestCodecLines_GothicTags(t *testing.T) {
	h := DefaultWasmHelper()
	names := structNamesSet()
	mk := func(tag string) fieldInfo {
		fi := testFieldInfo("N", "int")
		fi.GothicTag = tag
		return fi
	}
	for _, tag := range []string{"i32", "i64", "u32", "u64"} {
		enc, dec, err := h.codecLines(mk(tag), names, map[string]string{}, nil)
		if err != nil {
			t.Fatalf("tag %s: %v", tag, err)
		}
		if enc == "" || dec == "" {
			t.Errorf("tag %s: empty enc/dec", tag)
		}
	}
	// skip → empty enc/dec, no error
	enc, dec, err := h.codecLines(mk("skip"), names, map[string]string{}, nil)
	if err != nil || enc != "" || dec != "" {
		t.Errorf("skip tag: got enc=%q dec=%q err=%v", enc, dec, err)
	}
	// unknown tag → error
	if _, _, err := h.codecLines(mk("bogus"), names, map[string]string{}, nil); err == nil {
		t.Error("expected error for unknown gothic tag")
	}
}

func TestCodecLines_Errors(t *testing.T) {
	h := DefaultWasmHelper()
	names := structNamesSet()

	// Missing TypeRef.
	fi := fieldInfo{Name: "X", Type: "int", TypeRef: nil}
	if _, _, err := h.codecLines(fi, names, map[string]string{}, nil); err == nil {
		t.Error("expected error for missing TypeRef")
	}

	// Unsupported named type (not primitive, not a known struct).
	bad := testFieldInfo("X", "SomeUnknownType")
	if _, _, err := h.codecLines(bad, names, map[string]string{}, nil); err == nil {
		t.Error("expected error for unknown named type")
	}
}

func TestCodecLines_TypeAlias(t *testing.T) {
	h := DefaultWasmHelper()
	names := structNamesSet()
	aliases := map[string]string{"MyInt": "int"}
	refAliases := map[string]typeRef{"MyInt": Named{Name: "int"}}
	fi := testFieldInfo("N", "MyInt")
	enc, dec, err := h.codecLines(fi, names, aliases, refAliases)
	if err != nil {
		t.Fatalf("alias codecLines: %v", err)
	}
	if !strings.Contains(enc, "e.I64") {
		t.Errorf("alias enc: got %q", enc)
	}
	// decode should cast back to the alias type.
	if !strings.Contains(dec, "MyInt") {
		t.Errorf("alias dec should cast to MyInt: got %q", dec)
	}
}

// TestCodecLines_SliceOfStruct_GeneratedShape pins the correctness-critical
// structure of the generated encode/decode for []Item: the encode must range
// over the slice value, and the decode must allocate exactly _n elements and
// call the per-element struct decoder.
func TestCodecLines_SliceOfStruct_GeneratedShape(t *testing.T) {
	h := DefaultWasmHelper()
	names := structNamesSet("Item")
	fi := testFieldInfo("Items", "[]Item")
	enc, dec, err := h.codecLines(fi, names, map[string]string{}, nil)
	if err != nil {
		t.Fatalf("codecLines: %v", err)
	}
	// Encode: length prefix + range-over-value loop calling the struct encoder.
	for _, want := range []string{
		"e.U32(uint32(len(v.Items)))",
		"for _, _item := range v.Items",
		"_encode_Item(_item, e)",
	} {
		if !strings.Contains(enc, want) {
			t.Errorf("enc missing %q:\n%s", want, enc)
		}
	}
	// Decode: make a slice of the correct length, then fill via the struct decoder.
	for _, want := range []string{
		"_n := int(d.U32())",
		"v.Items = make([]Item, _n)",
		"v.Items[_i] = _decode_Item(d)",
	} {
		if !strings.Contains(dec, want) {
			t.Errorf("dec missing %q:\n%s", want, dec)
		}
	}
}

// TestCodecLines_Map_GeneratedShape pins that the key codec precedes the value
// codec in both encode and decode for map[string]int, and that the decode
// allocates a map of the correct concrete type.
func TestCodecLines_Map_GeneratedShape(t *testing.T) {
	h := DefaultWasmHelper()
	names := structNamesSet()
	fi := testFieldInfo("M", "map[string]int")
	enc, dec, err := h.codecLines(fi, names, map[string]string{}, nil)
	if err != nil {
		t.Fatalf("codecLines: %v", err)
	}
	// Encode: range over map producing _k (string) then _v (int) in that order.
	if !strings.Contains(enc, "for _k, _v := range v.M") {
		t.Errorf("enc missing map range:\n%s", enc)
	}
	encKey := strings.Index(enc, "e.String(string(_k))")
	encVal := strings.Index(enc, "e.I64(int64(_v))")
	if encKey < 0 || encVal < 0 {
		t.Fatalf("enc missing key/value codecs (key=%d val=%d):\n%s", encKey, encVal, enc)
	}
	if encKey > encVal {
		t.Errorf("enc: key codec must precede value codec:\n%s", enc)
	}
	// Decode: allocate the concrete map type, then decode key before value.
	if !strings.Contains(dec, "v.M = make(map[string]int, _n)") {
		t.Errorf("dec missing typed map allocation:\n%s", dec)
	}
	decKey := strings.Index(dec, "_k = string(d.String())")
	decVal := strings.Index(dec, "_v = int(d.I64())")
	if decKey < 0 || decVal < 0 {
		t.Fatalf("dec missing key/value decoders (key=%d val=%d):\n%s", decKey, decVal, dec)
	}
	if decKey > decVal {
		t.Errorf("dec: key decoder must precede value decoder:\n%s", dec)
	}
}

// TestCodecLines_Pointer_GeneratedShape pins that the nil-pointer branch is
// emitted for *Item (a 0/1 presence byte guarding the dereferenced encode).
func TestCodecLines_Pointer_GeneratedShape(t *testing.T) {
	h := DefaultWasmHelper()
	names := structNamesSet("Item")
	fi := testFieldInfo("P", "*Item")
	enc, dec, err := h.codecLines(fi, names, map[string]string{}, nil)
	if err != nil {
		t.Fatalf("codecLines: %v", err)
	}
	if !strings.Contains(enc, "if v.P == nil") || !strings.Contains(enc, "e.U8(0)") || !strings.Contains(enc, "e.U8(1)") {
		t.Errorf("enc missing nil-pointer presence branch:\n%s", enc)
	}
	if !strings.Contains(enc, "_encode_Item") {
		t.Errorf("enc missing struct encoder for pointee:\n%s", enc)
	}
	// Decode reads the presence byte before constructing the pointer.
	if !strings.Contains(dec, "d.U8()") {
		t.Errorf("dec missing presence-byte read:\n%s", dec)
	}
}

// TestCodecLines_AliasCastPresent pins that an alias primitive (type MyInt int)
// is written through its underlying primitive but cast back to the alias type on
// decode.
func TestCodecLines_AliasCastPresent(t *testing.T) {
	h := DefaultWasmHelper()
	names := structNamesSet()
	aliases := map[string]string{"MyInt": "int"}
	refAliases := map[string]typeRef{"MyInt": Named{Name: "int"}}
	fi := testFieldInfo("N", "MyInt")
	enc, dec, err := h.codecLines(fi, names, aliases, refAliases)
	if err != nil {
		t.Fatalf("codecLines: %v", err)
	}
	if !strings.Contains(enc, "e.I64(int64(v.N))") {
		t.Errorf("enc should write the underlying primitive:\n%s", enc)
	}
	if !strings.Contains(dec, "v.N = MyInt(") || !strings.Contains(dec, "int(d.I64())") {
		t.Errorf("dec should cast back to alias type MyInt:\n%s", dec)
	}
}

func TestCaptureLines(t *testing.T) {
	h := DefaultWasmHelper()
	names := structNamesSet("Item")

	// skip field → captures nothing but advances zero.
	skip := testFieldInfo("S", "int")
	skip.GothicTag = "skip"
	body, err := h.captureLines(skip, names, map[string]string{}, nil)
	if err != nil {
		t.Fatalf("captureLines skip: %v", err)
	}
	if !strings.Contains(body, "d.Buf[start:d.Pos]") {
		t.Errorf("skip capture body unexpected: %q", body)
	}

	// slice field → strips the receiver writes / make allocation.
	sl := testFieldInfo("Items", "[]Item")
	body2, err := h.captureLines(sl, names, map[string]string{}, nil)
	if err != nil {
		t.Fatalf("captureLines slice: %v", err)
	}
	if strings.Contains(body2, "v.Items") {
		t.Errorf("capture body should not reference receiver: %q", body2)
	}

	// error propagation from codecLines (unknown type).
	bad := testFieldInfo("X", "Nope")
	if _, err := h.captureLines(bad, names, map[string]string{}, nil); err == nil {
		t.Error("expected captureLines to propagate codec error")
	}
}

func TestBuildManagerFieldData_MapAndPointer(t *testing.T) {
	h := DefaultWasmHelper()
	s := structInfo{
		Name:    "Page",
		KeyName: "page",
		Fields: []fieldInfo{
			testFieldInfo("M", "map[string]int"),
			testFieldInfo("P", "*Item"),
			testFieldInfo("Data", "[]byte"),
		},
	}
	names := structNamesSet("Page", "Item")
	fields, err := h.buildManagerFieldData(s, names, map[string]string{}, nil)
	if err != nil {
		t.Fatalf("buildManagerFieldData: %v", err)
	}
	if len(fields) != 3 {
		t.Fatalf("expected 3 fields, got %d", len(fields))
	}
	for _, f := range fields {
		if f.CaptureBody == "" {
			t.Errorf("field %s: empty CaptureBody", f.FieldName)
		}
	}
}

func TestBuildManagerFieldData_ErrorPropagates(t *testing.T) {
	h := DefaultWasmHelper()
	s := structInfo{
		Name:    "Page",
		KeyName: "page",
		Fields:  []fieldInfo{testFieldInfo("X", "TotallyUnknown")},
	}
	if _, err := h.buildManagerFieldData(s, structNamesSet("Page"), map[string]string{}, nil); err == nil {
		t.Error("expected error from unknown field type")
	}
}

func TestKindOf(t *testing.T) {
	names := structNamesSet("Item")
	if kindOf(SliceOf{Elem: Named{Name: "byte"}}, names) != kindBytes {
		t.Error("[]byte → kindBytes")
	}
	if kindOf(SliceOf{Elem: Named{Name: "int"}}, names) != kindSlice {
		t.Error("[]int → kindSlice")
	}
	if kindOf(MapOf{Key: Named{Name: "string"}, Val: Named{Name: "int"}}, names) != kindMap {
		t.Error("map → kindMap")
	}
	if kindOf(PointerOf{Elem: Named{Name: "Item"}}, names) != kindPointer {
		t.Error("*Item → kindPointer")
	}
	if kindOf(Named{Name: "int"}, names) != kindPrimitive {
		t.Error("int → kindPrimitive")
	}
	if kindOf(Named{Name: "Item"}, names) != kindStruct {
		t.Error("Item → kindStruct")
	}
	if kindOf(Named{Name: "Whatever"}, names) != kindUnknown {
		t.Error("unknown → kindUnknown")
	}
}

// TestCodecLines_ComplexShapes drives the nested/pointer/map branches that the
// golden fixtures don't all reach.
func TestCodecLines_ComplexShapes(t *testing.T) {
	h := DefaultWasmHelper()
	names := structNamesSet("Item")

	shapes := []string{
		"[][]int",            // nested slice
		"[]*int",             // slice of pointer primitive
		"[]*Item",            // slice of pointer struct
		"map[string]*int",    // map of pointer primitive
		"map[string]*Item",   // map of pointer struct
		"map[string]Item",    // map of struct
		"map[string]map[int]string", // nested map
		"*int",               // pointer primitive
		"*Item",              // pointer struct
	}
	for _, shape := range shapes {
		t.Run(shape, func(t *testing.T) {
			fi := testFieldInfo("F", shape)
			enc, dec, err := h.codecLines(fi, names, map[string]string{}, nil)
			if err != nil {
				t.Fatalf("codecLines(%s): %v", shape, err)
			}
			if enc == "" || dec == "" {
				t.Errorf("codecLines(%s): empty enc/dec", shape)
			}
		})
	}
}

func TestCodecLines_ComplexErrors(t *testing.T) {
	h := DefaultWasmHelper()
	names := structNamesSet() // no known structs

	errShapes := []string{
		"[]Unknown",           // slice element not primitive/struct
		"[]*Unknown",          // slice of pointer to unknown
		"map[Item]int",        // map key not primitive (Item unknown anyway)
		"map[string]Unknown",  // map value not primitive/struct
		"map[string]*Unknown", // map pointer value unknown
		"*Unknown",            // pointer to unknown
		"map[string]map[int]Unknown", // nested map bad value
	}
	for _, shape := range errShapes {
		t.Run(shape, func(t *testing.T) {
			fi := testFieldInfo("F", shape)
			if _, _, err := h.codecLines(fi, names, map[string]string{}, nil); err == nil {
				t.Errorf("codecLines(%s): expected error", shape)
			}
		})
	}
}

// TestCodecLines_AliasCasts exercises the alias-cast branches inside the
// slice/map/pointer codecs (where the resolved primitive type differs from the
// written type, so decode must cast back to the alias).
func TestCodecLines_AliasCasts(t *testing.T) {
	h := DefaultWasmHelper()
	names := structNamesSet()
	aliases := map[string]string{
		"MyInt": "int",
		"MyKey": "string",
	}
	refAliases := map[string]typeRef{
		"MyInt": Named{Name: "int"},
		"MyKey": Named{Name: "string"},
	}

	shapes := []string{
		"[]MyInt",            // slice of alias primitive → cast branch
		"[]*MyInt",           // slice of pointer alias primitive → cast branch
		"map[MyKey]MyInt",    // map with alias key + alias value → both cast branches
		"map[string]*MyInt",  // map pointer alias value → cast branch
		"*MyInt",             // pointer alias primitive → cast branch
		"map[MyKey]map[string]MyInt", // nested map alias key/value
	}
	for _, shape := range shapes {
		t.Run(shape, func(t *testing.T) {
			fi := testFieldInfo("F", shape)
			enc, dec, err := h.codecLines(fi, names, aliases, refAliases)
			if err != nil {
				t.Fatalf("codecLines(%s): %v", shape, err)
			}
			if enc == "" || dec == "" {
				t.Errorf("codecLines(%s): empty enc/dec", shape)
			}
		})
	}
}

func TestBuildCodecData_WithAliasesAndNestedStructs(t *testing.T) {
	h := DefaultWasmHelper()
	structs := []structInfo{
		{
			Name:    "Page",
			KeyName: "page",
			Fields: []fieldInfo{
				testFieldInfo("Items", "[]Item"),
				testFieldInfo("Lookup", "map[string]Item"),
				testFieldInfo("Maybe", "*Item"),
			},
		},
		{
			Name: "Item",
			Fields: []fieldInfo{
				testFieldInfo("V", "int"),
			},
		},
	}
	codecs, err := h.buildCodecData(structs, map[string]string{}, nil)
	if err != nil {
		t.Fatalf("buildCodecData: %v", err)
	}
	if len(codecs) != 2 {
		t.Fatalf("expected 2 codecs, got %d", len(codecs))
	}
}

func TestBuildCodecData_ErrorPropagates(t *testing.T) {
	h := DefaultWasmHelper()
	structs := []structInfo{
		{Name: "Bad", KeyName: "bad", Fields: []fieldInfo{testFieldInfo("X", "Nope")}},
	}
	if _, err := h.buildCodecData(structs, map[string]string{}, nil); err == nil {
		t.Error("expected error from unknown field type")
	}
}

// TestCaptureLines_MapAndNested drives stripReceiverWrites / dropMakeAssignments
// over map and nested shapes (the [_k] receiver-write and make-drop branches).
func TestCaptureLines_MapAndNested(t *testing.T) {
	h := DefaultWasmHelper()
	names := structNamesSet("Item")
	for _, shape := range []string{"map[string]int", "map[string]Item", "[][]int", "map[string]map[int]string"} {
		t.Run(shape, func(t *testing.T) {
			fi := testFieldInfo("F", shape)
			body, err := h.captureLines(fi, names, map[string]string{}, nil)
			if err != nil {
				t.Fatalf("captureLines(%s): %v", shape, err)
			}
			if strings.Contains(body, "v.F = make") {
				t.Errorf("expected make assignment dropped: %q", body)
			}
		})
	}
}

func TestCodecLines_MapOfPointerStruct(t *testing.T) {
	h := DefaultWasmHelper()
	names := structNamesSet("Item")
	fi := testFieldInfo("M", "map[string]*Item")
	enc, dec, err := h.codecLines(fi, names, map[string]string{}, nil)
	if err != nil {
		t.Fatalf("codecLines map[string]*Item: %v", err)
	}
	if !strings.Contains(enc, "_encode_Item") || !strings.Contains(dec, "_decode_Item") {
		t.Errorf("expected struct codec calls; enc=%q dec=%q", enc, dec)
	}
}

func TestCodecLines_NestedMapOfStruct(t *testing.T) {
	h := DefaultWasmHelper()
	names := structNamesSet("Item")
	fi := testFieldInfo("M", "map[string]map[int]Item")
	enc, dec, err := h.codecLines(fi, names, map[string]string{}, nil)
	if err != nil {
		t.Fatalf("codecLines nested map of struct: %v", err)
	}
	if enc == "" || dec == "" {
		t.Error("empty enc/dec for nested map of struct")
	}
}

func TestResolveTypeRef(t *testing.T) {
	refAliases := map[string]typeRef{"MyInt": Named{Name: "int"}}
	got := resolveTypeRef(Named{Name: "MyInt"}, refAliases)
	if n, ok := got.(Named); !ok || n.Name != "int" {
		t.Errorf("resolveTypeRef alias: got %v", got)
	}
	// non-alias passes through
	if got := resolveTypeRef(SliceOf{Elem: Named{Name: "int"}}, refAliases); got.String() != "[]int" {
		t.Errorf("resolveTypeRef passthrough: got %v", got)
	}
}
