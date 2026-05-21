package helpers

import (
	"strings"
	"testing"
)

func TestRewriteAutoKeys_Primitive(t *testing.T) {
	got, err := astRewriteAutoKeys(`AutoKey[int]("x")`)
	if err != nil {
		t.Fatalf("astRewriteAutoKeys: err=%v, expected nil", err)
	}
	want := `BinaryKey[int]("x", _encode_int, _decode_int)`
	if got != want {
		t.Errorf("astRewriteAutoKeys primitive:\n got: %q\nwant: %q", got, want)
	}
}

func TestRewriteAutoKeys_Slice(t *testing.T) {
	got, err := astRewriteAutoKeys(`AutoKey[[]string]("x")`)
	if err != nil {
		t.Fatalf("astRewriteAutoKeys: err=%v, expected nil", err)
	}
	want := `BinaryKey[[]string]("x", _encode_slicestring, _decode_slicestring)`
	if got != want {
		t.Errorf("astRewriteAutoKeys slice:\n got: %q\nwant: %q", got, want)
	}
}

func TestRewriteAutoKeys_Map(t *testing.T) {
	got, err := astRewriteAutoKeys(`AutoKey[map[string]int]("x")`)
	if err != nil {
		t.Fatalf("astRewriteAutoKeys: err=%v, expected nil", err)
	}
	// Map types get bracket-stripped to form a valid Go ident; the regex
	// path could not match this at all, so the AST path is what unlocks it.
	want := `BinaryKey[map[string]int]("x", _encode_mapstringint, _decode_mapstringint)`
	if got != want {
		t.Errorf("astRewriteAutoKeys map:\n got: %q\nwant: %q", got, want)
	}
}

func TestRewriteAutoKeys_MultiLine(t *testing.T) {
	// The regex path requires the call to be on one line; the AST path
	// handles arbitrary whitespace including newlines.
	src := "var k = AutoKey[int](\n\t\"some-name\",\n)"
	got, err := astRewriteAutoKeys(src)
	if err != nil {
		t.Fatalf("astRewriteAutoKeys multi-line: err=%v, expected nil", err)
	}
	if !strings.Contains(got, `BinaryKey[int]("some-name", _encode_int, _decode_int)`) {
		t.Errorf("astRewriteAutoKeys multi-line did not rewrite:\n got: %q", got)
	}
}

func TestRewriteAutoKeys_Unparseable(t *testing.T) {
	// Garbage that is neither a top-level decl nor a valid statement.
	src := `}{[(@!! AutoKey[int]("x")`
	got, err := astRewriteAutoKeys(src)
	if err == nil {
		t.Fatalf("astRewriteAutoKeys unparseable: err=nil, expected non-nil (got %q)", got)
	}
	if got != "" {
		t.Errorf("astRewriteAutoKeys unparseable: got=%q, expected empty", got)
	}
	// Error must carry positional info in file:line:col form (we expect at
	// minimum a ":<line>:" substring).
	msg := err.Error()
	if !strings.Contains(msg, ":1:") && !strings.Contains(msg, ":2:") {
		t.Errorf("astRewriteAutoKeys unparseable: error %q lacks line:col position info", msg)
	}
}

func TestRewriteAutoKeys_NoChangeWhenAbsent(t *testing.T) {
	src := "var x = 1\n"
	got, err := astRewriteAutoKeys(src)
	if err != nil {
		t.Fatalf("astRewriteAutoKeys absent: err=%v", err)
	}
	if got != src {
		t.Errorf("astRewriteAutoKeys absent: got=%q, want=%q", got, src)
	}
}

func TestRewriteContextCalls_UseContextWithKey(t *testing.T) {
	h := &WasmHelper{}
	structs := []structInfo{{Name: "Page", KeyName: "page"}}
	got, err := h.rewriteContextCalls("UseContext(PageKey, Page{Pings: 1})", structs)
	if err != nil {
		t.Fatalf("rewriteContextCalls: err=%v", err)
	}
	want := "PageContext(Page{Pings: 1})"
	if got != want {
		t.Errorf("rewriteContextCalls UseContext+Key:\n got: %q\nwant: %q", got, want)
	}
}

func TestRewriteContextCalls_UseContextWithNameIdent(t *testing.T) {
	h := &WasmHelper{}
	structs := []structInfo{{Name: "Page", KeyName: "page"}}
	got, err := h.rewriteContextCalls("UseContext(Page, Page{})", structs)
	if err != nil {
		t.Fatalf("rewriteContextCalls: err=%v", err)
	}
	want := "PageContext(Page{})"
	if got != want {
		t.Errorf("rewriteContextCalls UseContext+Name ident:\n got: %q\nwant: %q", got, want)
	}
}

func TestRewriteContextCalls_UseName(t *testing.T) {
	h := &WasmHelper{}
	structs := []structInfo{{Name: "Page", KeyName: "page"}}
	got, err := h.rewriteContextCalls("UsePage(Page{})", structs)
	if err != nil {
		t.Fatalf("rewriteContextCalls: err=%v", err)
	}
	want := "PageContext(Page{})"
	if got != want {
		t.Errorf("rewriteContextCalls UseName:\n got: %q\nwant: %q", got, want)
	}
}

func TestRewriteContextCalls_UseNameContext(t *testing.T) {
	h := &WasmHelper{}
	structs := []structInfo{{Name: "Page", KeyName: "page"}}
	got, err := h.rewriteContextCalls("UsePageContext(Page{})", structs)
	if err != nil {
		t.Fatalf("rewriteContextCalls: err=%v", err)
	}
	want := "PageContext(Page{})"
	if got != want {
		t.Errorf("rewriteContextCalls UseNameContext:\n got: %q\nwant: %q", got, want)
	}
}

func TestRewriteContextCalls_NoMatchUnknownStruct(t *testing.T) {
	h := &WasmHelper{}
	structs := []structInfo{{Name: "Page", KeyName: "page"}}
	// "Other" is not in structs, so the call must be left alone.
	src := "UseContext(Other, Other{})"
	got, err := h.rewriteContextCalls(src, structs)
	if err != nil {
		t.Fatalf("rewriteContextCalls: err=%v", err)
	}
	if got != src {
		t.Errorf("rewriteContextCalls should ignore unknown struct names:\n got: %q\nwant: %q", got, src)
	}
}

func TestRewriteContextCalls_MultipleCalls(t *testing.T) {
	h := &WasmHelper{}
	structs := []structInfo{
		{Name: "Page", KeyName: "page"},
		{Name: "User", KeyName: "user"},
	}
	src := "UseContext(PageKey, Page{}); UseUser(User{})"
	got, err := h.rewriteContextCalls(src, structs)
	if err != nil {
		t.Fatalf("rewriteContextCalls: err=%v", err)
	}
	want := "PageContext(Page{}); UserContext(User{})"
	if got != want {
		t.Errorf("rewriteContextCalls multi:\n got: %q\nwant: %q", got, want)
	}
}

func TestRewriteContextCalls_UnparseableReturnsError(t *testing.T) {
	h := &WasmHelper{}
	unparseableSrc := `}}}this is not valid Go{{{{`
	structs := []structInfo{{Name: "Page", KeyName: "page"}}
	_, err := h.rewriteContextCalls(unparseableSrc, structs)
	if err == nil {
		t.Fatal("expected error for unparseable source, got nil")
	}
}
