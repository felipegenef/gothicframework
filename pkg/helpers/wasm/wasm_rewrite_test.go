package helpers

import (
	"strings"
	"testing"
)

func TestRewriteAutoKeys_Primitive(t *testing.T) {
	got, ok := astRewriteAutoKeys(`AutoKey[int]("x")`)
	if !ok {
		t.Fatalf("astRewriteAutoKeys: ok=false, expected true")
	}
	want := `BinaryKey[int]("x", _encode_int, _decode_int)`
	if got != want {
		t.Errorf("astRewriteAutoKeys primitive:\n got: %q\nwant: %q", got, want)
	}
}

func TestRewriteAutoKeys_Slice(t *testing.T) {
	got, ok := astRewriteAutoKeys(`AutoKey[[]string]("x")`)
	if !ok {
		t.Fatalf("astRewriteAutoKeys: ok=false, expected true")
	}
	want := `BinaryKey[[]string]("x", _encode_slicestring, _decode_slicestring)`
	if got != want {
		t.Errorf("astRewriteAutoKeys slice:\n got: %q\nwant: %q", got, want)
	}
}

func TestRewriteAutoKeys_Map(t *testing.T) {
	got, ok := astRewriteAutoKeys(`AutoKey[map[string]int]("x")`)
	if !ok {
		t.Fatalf("astRewriteAutoKeys: ok=false, expected true")
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
	got, ok := astRewriteAutoKeys(src)
	if !ok {
		t.Fatalf("astRewriteAutoKeys multi-line: ok=false, expected true")
	}
	if !strings.Contains(got, `BinaryKey[int]("some-name", _encode_int, _decode_int)`) {
		t.Errorf("astRewriteAutoKeys multi-line did not rewrite:\n got: %q", got)
	}
}

func TestRewriteAutoKeys_Unparseable(t *testing.T) {
	// Garbage that is neither a top-level decl nor a valid statement.
	src := `}{[(@!! AutoKey[int]("x")`
	got, ok := astRewriteAutoKeys(src)
	if ok {
		t.Errorf("astRewriteAutoKeys unparseable: ok=true, expected false (got %q)", got)
	}
	if got != "" {
		t.Errorf("astRewriteAutoKeys unparseable: got=%q, expected empty", got)
	}
}

func TestRewriteAutoKeys_NoChangeWhenAbsent(t *testing.T) {
	src := "var x = 1\n"
	got, ok := astRewriteAutoKeys(src)
	if !ok {
		t.Fatalf("astRewriteAutoKeys absent: ok=false")
	}
	if got != src {
		t.Errorf("astRewriteAutoKeys absent: got=%q, want=%q", got, src)
	}
}

func TestRewriteContextCalls_UseContextWithKey(t *testing.T) {
	h := &WasmHelper{}
	structs := []structInfo{{Name: "Page", KeyName: "page"}}
	got := h.rewriteContextCalls("UseContext(PageKey, Page{Pings: 1})", structs)
	want := "PageContext(Page{Pings: 1})"
	if got != want {
		t.Errorf("rewriteContextCalls UseContext+Key:\n got: %q\nwant: %q", got, want)
	}
}

func TestRewriteContextCalls_UseContextWithNameIdent(t *testing.T) {
	h := &WasmHelper{}
	structs := []structInfo{{Name: "Page", KeyName: "page"}}
	got := h.rewriteContextCalls("UseContext(Page, Page{})", structs)
	want := "PageContext(Page{})"
	if got != want {
		t.Errorf("rewriteContextCalls UseContext+Name ident:\n got: %q\nwant: %q", got, want)
	}
}

func TestRewriteContextCalls_UseName(t *testing.T) {
	h := &WasmHelper{}
	structs := []structInfo{{Name: "Page", KeyName: "page"}}
	got := h.rewriteContextCalls("UsePage(Page{})", structs)
	want := "PageContext(Page{})"
	if got != want {
		t.Errorf("rewriteContextCalls UseName:\n got: %q\nwant: %q", got, want)
	}
}

func TestRewriteContextCalls_UseNameContext(t *testing.T) {
	h := &WasmHelper{}
	structs := []structInfo{{Name: "Page", KeyName: "page"}}
	got := h.rewriteContextCalls("UsePageContext(Page{})", structs)
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
	got := h.rewriteContextCalls(src, structs)
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
	got := h.rewriteContextCalls(src, structs)
	want := "PageContext(Page{}); UserContext(User{})"
	if got != want {
		t.Errorf("rewriteContextCalls multi:\n got: %q\nwant: %q", got, want)
	}
}

func TestRewriteContextCalls_FallbackOnUnparseable(t *testing.T) {
	// When AST parsing fails, the fallback regex/string-replace path runs.
	h := &WasmHelper{}
	structs := []structInfo{{Name: "Page", KeyName: "page"}}
	// Make the body unparseable on its own but still containing a recognizable
	// substring that the legacy fallback can replace.
	src := "}}}UsePage(Page{})"
	got := h.rewriteContextCalls(src, structs)
	// Both AST (best-effort) and legacy ReplaceAll should yield this. Legacy
	// is what we rely on here since the AST parse would fail.
	if !strings.Contains(got, "PageContext(Page{}") {
		t.Errorf("rewriteContextCalls fallback did not rewrite: %q", got)
	}
}
