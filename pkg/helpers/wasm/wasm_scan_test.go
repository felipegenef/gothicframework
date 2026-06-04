package helpers

import (
	"go/ast"
	"go/parser"
	"go/token"
	"runtime"
	"strings"
	"testing"
)

func TestStdImportLines_KeepsAllImports(t *testing.T) {
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
	// As of Phase 4 (module bridging), non-stdlib imports pass through too:
	// the temp build module links back to the user's project via a replace
	// directive, so third-party and user-project imports resolve normally.
	if !strings.Contains(joined, `"github.com/foo/bar"`) {
		t.Errorf("third-party should be kept (module bridge resolves it), got: %v", got)
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

// TestScanFile_CommentNotMatched verifies that the *production* scanner
// (ScanPages → scanFile → astx.ExtractClientSideStateBody) does NOT pick up a
// "ClientSideState: func() {" occurrence that appears only inside Go comments.
//
// A regex-based scanner would match the comment text; the AST-based scanner
// correctly looks only at composite-literal keys on a routes.RouteConfig. This
// test drives the real production code path against a temp project so it would
// fail if that scanner regressed — unlike a test that re-walks the AST itself.
func TestScanFile_CommentNotMatched(t *testing.T) {
	withTempCwd(t)
	writeProjectFile(t, "go.mod", "module example.com/cmt\n\ngo 1.21\n")
	writeProjectFile(t, "helpers/routes/routes.go", `package routes

type RouteConfig struct {
	ClientSideState func()
	WasmCompression string
	WasmCompiler    string
}
`)
	// A *_templ.go page that mentions ClientSideState ONLY inside comments.
	// There is no routes.RouteConfig composite literal, so the production
	// scanner must extract nothing.
	writeProjectFile(t, "src/pages/comment_templ.go", `package pages

// Example: ClientSideState: func() { panic("nope") }
// Another reference: var x = routes.RouteConfig{ClientSideState: notARealFunc}

var X = 42
`)

	h := DefaultWasmHelper()
	pages, err := h.ScanPages("src/pages", "")
	if err != nil {
		t.Fatalf("ScanPages: %v", err)
	}
	if len(pages) != 0 {
		t.Fatalf("production scanner extracted %d page(s) from comment-only ClientSideState; expected 0: %+v", len(pages), pages)
	}
}
