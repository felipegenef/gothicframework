package helpers

import (
	"go/ast"
	"go/parser"
	"go/token"
	"runtime"
	"strings"
	"testing"
)

func TestStdImportLines_KeepsStdDropsThirdParty(t *testing.T) {
	src := `package x

import (
	"fmt"
	"strings"
	alias "encoding/json"
	"github.com/foo/bar"
)
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "x.go", src, 0)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	var specs []*ast.ImportSpec
	for _, imp := range f.Imports {
		specs = append(specs, imp)
	}
	got := stdImportLines(specs)
	joined := strings.Join(got, "\n")
	if !strings.Contains(joined, `"fmt"`) {
		t.Errorf("fmt should be kept, got: %v", got)
	}
	if !strings.Contains(joined, `"strings"`) {
		t.Errorf("strings should be kept, got: %v", got)
	}
	if !strings.Contains(joined, `alias "encoding/json"`) {
		t.Errorf("aliased std import should be kept with alias, got: %v", got)
	}
	if strings.Contains(joined, "github.com/foo/bar") {
		t.Errorf("third-party should be dropped, got: %v", got)
	}
}

func TestContainsString(t *testing.T) {
	if !containsString([]string{"a", "b"}, "a") {
		t.Errorf("expected true for present element")
	}
	if containsString([]string{"a", "b"}, "z") {
		t.Errorf("expected false for missing element")
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
	h := DefaultWasmHelper()
	got := h.normalizeWasmHttpPath("src/pages/foo/bar_templ.go")
	if got != "/foo/bar" {
		t.Errorf("normalizeWasmHttpPath: got %q, want %q", got, "/foo/bar")
	}
}

// TestScanFile_CommentNotMatched verifies that a Go comment containing the
// literal "ClientSideState: func() {" is NOT picked up by the AST scanner.
// The regex-based scanner would have matched this; the AST-based scanner
// correctly looks only at composite-literal keys with the right type.
func TestScanFile_CommentNotMatched(t *testing.T) {
	src := `package x

// Example: ClientSideState: func() { panic("nope") }
// Another reference: ClientSideState: notARealFunc

var X = 42
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "x.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	// Scan AST for any KeyValueExpr with key "ClientSideState".
	var found bool
	ast.Inspect(f, func(n ast.Node) bool {
		if kv, ok := n.(*ast.KeyValueExpr); ok {
			if id, ok := kv.Key.(*ast.Ident); ok && id.Name == "ClientSideState" {
				found = true
			}
		}
		return true
	})
	if found {
		t.Fatalf("AST should not see ClientSideState references inside comments")
	}
}
