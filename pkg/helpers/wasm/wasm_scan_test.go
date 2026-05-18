package helpers

import (
	"runtime"
	"strings"
	"testing"
)

func TestExtractFuncBody_Basic(t *testing.T) {
	h := DefaultWasmHelper()
	src := "func foo() {\n\tprintln(\"hi\")\n}"
	idx := strings.Index(src, "{")
	got := h.extractFuncBody(src, idx)
	want := "println(\"hi\")"
	if got != want {
		t.Errorf("extractFuncBody: got %q, want %q", got, want)
	}
}

func TestExtractFuncBody_NestedBraces(t *testing.T) {
	h := DefaultWasmHelper()
	src := "func foo() {\n\tif x { y() }\n\tfor i := range list {\n\t\tprint(i)\n\t}\n}"
	idx := strings.Index(src, "{")
	got := h.extractFuncBody(src, idx)
	if !strings.Contains(got, "if x { y() }") {
		t.Errorf("nested braces not preserved: %q", got)
	}
	if !strings.Contains(got, "for i := range list") {
		t.Errorf("for body not preserved: %q", got)
	}
}

func TestExtractFuncBody_StringWithBrace(t *testing.T) {
	h := DefaultWasmHelper()
	src := "func foo() {\n\ts := \"}\"\n\treturn s\n}"
	idx := strings.Index(src, "{")
	got := h.extractFuncBody(src, idx)
	if !strings.Contains(got, "\"}\"") {
		t.Errorf("brace inside string was counted: %q", got)
	}
}

func TestFilterStdImports_KeepsUsed(t *testing.T) {
	h := DefaultWasmHelper()
	content := `package x

import (
	"fmt"
	"strings"
)
`
	body := `fmt.Println("hi")`
	got := h.filterStdImports(content, body)
	joined := strings.Join(got, "\n")
	if !strings.Contains(joined, `"fmt"`) {
		t.Errorf("expected fmt import to be kept, got: %v", got)
	}
	if strings.Contains(joined, `"strings"`) {
		t.Errorf("expected strings import to be dropped (unused), got: %v", got)
	}
}

func TestFilterStdImports_DropsThirdParty(t *testing.T) {
	h := DefaultWasmHelper()
	content := `package x

import (
	"github.com/foo/bar"
	"fmt"
)
`
	body := `bar.X(); fmt.Println("hi")`
	got := h.filterStdImports(content, body)
	joined := strings.Join(got, "\n")
	if strings.Contains(joined, "github.com/foo/bar") {
		t.Errorf("third-party import should be dropped, got: %v", got)
	}
	if !strings.Contains(joined, `"fmt"`) {
		t.Errorf("std fmt import should be kept, got: %v", got)
	}
}

func TestNormalizeWasmHttpPath_StripsTemplSuffix(t *testing.T) {
	h := DefaultWasmHelper()
	got := h.normalizeWasmHttpPath("src/pages/counter_templ.go")
	if got != "/counter" {
		t.Errorf("normalizeWasmHttpPath: got %q, want %q", got, "/counter")
	}
}

func TestNormalizeWasmHttpPath_IndexBecomesRoot(t *testing.T) {
	h := DefaultWasmHelper()
	got := h.normalizeWasmHttpPath("src/pages/index_templ.go")
	if got != "/" {
		t.Errorf("normalizeWasmHttpPath: got %q, want %q", got, "/")
	}
}

func TestNormalizeWasmHttpPath_ParamVar(t *testing.T) {
	h := DefaultWasmHelper()
	got := h.normalizeWasmHttpPath("src/pages/blog/var_slug_templ.go")
	if got != "/blog/{slug}" {
		t.Errorf("normalizeWasmHttpPath: got %q, want %q", got, "/blog/{slug}")
	}
}

func TestNormalizeWasmHttpPath_WindowsBackslashes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows-specific path handling")
	}
	// Even on non-windows, the function should be idempotent on forward slashes.
	h := DefaultWasmHelper()
	got := h.normalizeWasmHttpPath("src/pages/foo/bar_templ.go")
	if got != "/foo/bar" {
		t.Errorf("normalizeWasmHttpPath: got %q, want %q", got, "/foo/bar")
	}
}
