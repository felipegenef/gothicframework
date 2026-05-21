package helpers

import (
	"strings"
	"testing"
)

func pageStructsFixture() (structInfo, []structInfo) {
	pings := testFieldInfo("Pings", "int")
	pings.GothicTag = "i32"
	page := structInfo{
		Name:    "Page",
		KeyName: "page",
		Fields: []fieldInfo{
			pings,
			testFieldInfo("Label", "string"),
			testFieldInfo("Theme", "string"),
			testFieldInfo("Tests", "[]Test"),
		},
	}
	test := structInfo{
		Name: "Test",
		Fields: []fieldInfo{
			testFieldInfo("Value", "int"),
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
	fields, err := h.buildManagerFieldData(page, structNames, map[string]string{}, nil)
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
	codecs, err := h.buildPerFieldCodecs(page, structNames, map[string]string{}, nil)
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
			testFieldInfo("V", "int"),
		},
	}
	parent := structInfo{
		Name:    "AllKinds",
		KeyName: "allkinds",
		Fields: []fieldInfo{
			testFieldInfo("Prim", "int"),
			testFieldInfo("Sub", "Item"),
			testFieldInfo("Tags", "[]string"),
			testFieldInfo("M", "map[string]Item"),
			testFieldInfo("Ptr", "*Item"),
			testFieldInfo("Data", "[]byte"),
		},
	}
	structNames := map[string]bool{"Item": true, "AllKinds": true}
	codecs, err := h.buildPerFieldCodecs(parent, structNames, map[string]string{}, nil)
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

func TestParseFieldTag(t *testing.T) {
	h := &WasmHelper{}
	tests := []struct {
		name       string
		input      string
		wantGothic string
		wantName   string
		wantCompr  WasmCompression
	}{
		{
			name:       "basic gothic tag",
			input:      "`gothic:\"compress\"`",
			wantGothic: "compress",
			wantName:   "",
			wantCompr:  WasmCompressionGzip,
		},
		{
			name:       "empty tag string",
			input:      "",
			wantGothic: "",
			wantName:   "",
			wantCompr:  WasmCompressionGzip,
		},
		{
			name:       "multi-tag brotli",
			input:      "`gothic:\"compress\" name:\"Foo\" compression:\"brotli\"`",
			wantGothic: "compress",
			wantName:   "Foo",
			wantCompr:  WasmCompressionBrotli,
		},
		{
			name:       "brotli uppercase",
			input:      "`gothic:\"compress\" compression:\"BROTLI\"`",
			wantGothic: "compress",
			wantName:   "",
			wantCompr:  WasmCompressionBrotli,
		},
		{
			name:       "single tag skip",
			input:      "`gothic:\"skip\"`",
			wantGothic: "skip",
			wantName:   "",
			wantCompr:  WasmCompressionGzip,
		},
		{
			name:       "unrelated key",
			input:      "`json:\"bar\"`",
			wantGothic: "",
			wantName:   "",
			wantCompr:  WasmCompressionGzip,
		},
		{
			name:       "quoted char in value",
			input:      "`gothic:\"a\\\"b\"`",
			wantGothic: `a"b`,
			wantName:   "",
			wantCompr:  WasmCompressionGzip,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotGothic, gotName, gotCompr := h.parseFieldTag(tt.input)
			if gotGothic != tt.wantGothic {
				t.Errorf("gothic: got %q, want %q", gotGothic, tt.wantGothic)
			}
			if gotName != tt.wantName {
				t.Errorf("name: got %q, want %q", gotName, tt.wantName)
			}
			if gotCompr != tt.wantCompr {
				t.Errorf("compression: got %v, want %v", gotCompr, tt.wantCompr)
			}
		})
	}
}
