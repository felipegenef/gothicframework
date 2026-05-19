package helpers

import (
	"strings"
	"testing"
)

func pageStructsFixture() (structInfo, []structInfo) {
	page := structInfo{
		Name:    "Page",
		KeyName: "page",
		Fields: []fieldInfo{
			{Name: "Pings", Type: "int", GothicTag: "i32"},
			{Name: "Label", Type: "string"},
			{Name: "Theme", Type: "string"},
			{Name: "Tests", Type: "[]Test"},
		},
	}
	test := structInfo{
		Name: "Test",
		Fields: []fieldInfo{
			{Name: "Value", Type: "int"},
		},
	}
	return page, []structInfo{page, test}
}

func TestBuildManagerFieldData_OrderAndShape(t *testing.T) {
	h := DefaultWasmHelper()
	page, all := pageStructsFixture()
	structNames := map[string]bool{}
	for _, s := range all {
		structNames[s.Name] = true
	}
	fields, err := h.buildManagerFieldData(page, structNames, map[string]string{})
	if err != nil {
		t.Fatalf("buildManagerFieldData: %v", err)
	}
	if len(fields) != len(page.Fields) {
		t.Fatalf("len: got %d, want %d", len(fields), len(page.Fields))
	}
	if fields[0].FieldName != "Pings" {
		t.Errorf("fields[0].FieldName: got %q, want %q", fields[0].FieldName, "Pings")
	}
	if fields[1].FieldName != "Label" {
		t.Errorf("fields[1].FieldName: got %q, want %q", fields[1].FieldName, "Label")
	}
	if !strings.Contains(fields[0].EncodeLines, "I32") {
		t.Errorf("fields[0].EncodeLines: want to contain %q, got %q", "I32", fields[0].EncodeLines)
	}
}

func TestBuildPerFieldCodecs_PopulatesFieldType(t *testing.T) {
	h := DefaultWasmHelper()
	page, all := pageStructsFixture()
	structNames := map[string]bool{}
	for _, s := range all {
		structNames[s.Name] = true
	}
	codecs, err := h.buildPerFieldCodecs(page, structNames, map[string]string{})
	if err != nil {
		t.Fatalf("buildPerFieldCodecs: %v", err)
	}
	if len(codecs) == 0 {
		t.Fatal("expected at least one codec")
	}
	if codecs[0].FieldType != "int" {
		t.Errorf("codecs[0].FieldType: got %q, want %q", codecs[0].FieldType, "int")
	}
}

// TestBuildPerFieldCodecs_AllKindsCompile constructs a fixture struct that
// touches every supported kind (primitive, struct, slice, map, pointer,
// []byte) and asserts buildPerFieldCodecs emits one PerFieldCodec per field
// with both EncLines and DecLines populated. Guards the per-field codec
// generator from regressing on any one kind.
func TestBuildPerFieldCodecs_AllKindsCompile(t *testing.T) {
	h := DefaultWasmHelper()
	item := structInfo{
		Name: "Item",
		Fields: []fieldInfo{
			{Name: "V", Type: "int"},
		},
	}
	parent := structInfo{
		Name:    "AllKinds",
		KeyName: "allkinds",
		Fields: []fieldInfo{
			{Name: "Prim", Type: "int"},
			{Name: "Sub", Type: "Item"},
			{Name: "Tags", Type: "[]string"},
			{Name: "M", Type: "map[string]Item"},
			{Name: "Ptr", Type: "*Item"},
			{Name: "Data", Type: "[]byte"},
		},
	}
	structNames := map[string]bool{"Item": true, "AllKinds": true}
	codecs, err := h.buildPerFieldCodecs(parent, structNames, map[string]string{})
	if err != nil {
		t.Fatalf("buildPerFieldCodecs: %v", err)
	}
	if len(codecs) != len(parent.Fields) {
		t.Fatalf("len(codecs): got %d, want %d", len(codecs), len(parent.Fields))
	}
	for i, c := range codecs {
		if c.EncLines == "" {
			t.Errorf("codecs[%d] (%s): EncLines empty", i, parent.Fields[i].Name)
		}
		if c.DecLines == "" {
			t.Errorf("codecs[%d] (%s): DecLines empty", i, parent.Fields[i].Name)
		}
	}
	_ = item
}

func TestParseGothicTag_Basic(t *testing.T) {
	h := DefaultWasmHelper()
	if got := h.parseGothicTag(`gothic:"i32"`); got != "i32" {
		t.Errorf("parseGothicTag: got %q, want %q", got, "i32")
	}
	if got := h.parseGothicTag(`json:"x" gothic:"skip"`); got != "skip" {
		t.Errorf("parseGothicTag mixed: got %q, want %q", got, "skip")
	}
	if got := h.parseGothicTag(`json:"x"`); got != "" {
		t.Errorf("parseGothicTag missing: got %q, want empty", got)
	}
}

func TestParseGothicTag_Empty(t *testing.T) {
	h := DefaultWasmHelper()
	if got := h.parseGothicTag(""); got != "" {
		t.Errorf("parseGothicTag empty input: got %q, want empty", got)
	}
}

func TestParseNameTag_Basic(t *testing.T) {
	h := DefaultWasmHelper()
	if got := h.parseNameTag(`name:"page"`); got != "page" {
		t.Errorf("parseNameTag: got %q, want %q", got, "page")
	}
	if got := h.parseNameTag(`json:"x" name:"my-key"`); got != "my-key" {
		t.Errorf("parseNameTag mixed: got %q, want %q", got, "my-key")
	}
	if got := h.parseNameTag(`other:"x"`); got != "" {
		t.Errorf("parseNameTag missing: got %q, want empty", got)
	}
}

func TestParseCompressionTag_DefaultsToGzip(t *testing.T) {
	h := DefaultWasmHelper()
	if got := h.parseCompressionTag(""); got != WasmCompressionGzip {
		t.Errorf("parseCompressionTag empty: got %v, want gzip", got)
	}
	if got := h.parseCompressionTag(`json:"x"`); got != WasmCompressionGzip {
		t.Errorf("parseCompressionTag missing: got %v, want gzip", got)
	}
}

func TestParseCompressionTag_Brotli(t *testing.T) {
	h := DefaultWasmHelper()
	if got := h.parseCompressionTag(`compression:"brotli"`); got != WasmCompressionBrotli {
		t.Errorf("parseCompressionTag brotli: got %v, want brotli", got)
	}
	// Case-insensitive.
	if got := h.parseCompressionTag(`compression:"BROTLI"`); got != WasmCompressionBrotli {
		t.Errorf("parseCompressionTag BROTLI upper: got %v, want brotli", got)
	}
}
