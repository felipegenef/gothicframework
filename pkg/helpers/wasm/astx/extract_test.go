package astx

import (
	"go/ast"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// testdataRoot returns the absolute path to this package's testdata directory,
// derived at runtime so the tests are not tied to one developer's machine.
func testdataRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	return filepath.Join(filepath.Dir(thisFile), "testdata")
}

// findClientSideStateBody walks the file's AST and returns the *ast.BlockStmt
// value of any KeyValueExpr whose key is "ClientSideState". Supports inline
// FuncLit and named ident. Used by tests because the testdata uses a local
// PageConfig type (not routes.RouteConfig), so ExtractClientSideStateBody
// would skip it.
func findClientSideStateBody(t *testing.T, entry Entry) *ast.BlockStmt {
	t.Helper()
	var body *ast.BlockStmt
	ast.Inspect(entry.File, func(n ast.Node) bool {
		kv, ok := n.(*ast.KeyValueExpr)
		if !ok {
			return true
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok || key.Name != "ClientSideState" {
			return true
		}
		switch v := kv.Value.(type) {
		case *ast.FuncLit:
			body = v.Body
		case *ast.Ident:
			obj := entry.Pkg.TypesInfo.Uses[v]
			if obj == nil {
				return true
			}
			for _, f := range entry.Pkg.Syntax {
				for _, d := range f.Decls {
					if fd, ok := d.(*ast.FuncDecl); ok && entry.Pkg.TypesInfo.Defs[fd.Name] == obj {
						body = fd.Body
						return false
					}
				}
			}
		}
		return body == nil
	})
	if body == nil {
		t.Fatalf("could not find ClientSideState body in testdata")
	}
	return body
}

func loadTestdata(t *testing.T, sub string) Entry {
	t.Helper()
	dir := filepath.Join(testdataRoot(t), sub)
	l, err := NewLoader(dir)
	if err != nil {
		t.Fatalf("NewLoader(%s): %v", sub, err)
	}
	abs := filepath.Join(dir, "main.go")
	entry, err := l.Get(abs)
	if err != nil {
		t.Fatalf("Get(%s): %v", abs, err)
	}
	return entry
}

func TestExtractUsedHelpers_Inline(t *testing.T) {
	entry := loadTestdata(t, "inline_state")
	body := findClientSideStateBody(t, entry)

	decls, pkgs, err := ExtractUsedHelpers(entry.Pkg, body)
	if err != nil {
		t.Fatalf("ExtractUsedHelpers: %v", err)
	}
	if len(decls) != 0 {
		t.Errorf("expected 0 helpers for inline body, got %d", len(decls))
	}
	if len(pkgs) != 0 {
		t.Errorf("expected 0 external pkg refs, got %d", len(pkgs))
	}
}

func TestExtractUsedHelpers_Named(t *testing.T) {
	entry := loadTestdata(t, "named_state")
	body := findClientSideStateBody(t, entry)

	decls, pkgs, err := ExtractUsedHelpers(entry.Pkg, body)
	if err != nil {
		t.Fatalf("ExtractUsedHelpers: %v", err)
	}
	// The named func body has fmt.Println — fmt should be picked up as an external pkg.
	foundFmt := false
	for _, p := range pkgs {
		if p.Imported().Path() == "fmt" {
			foundFmt = true
		}
	}
	if !foundFmt {
		t.Errorf("expected fmt in pkg refs, got %v", pkgs)
	}
	_ = decls

	imps, err := ExtractUsedImports(entry.Pkg, body)
	if err != nil {
		t.Fatalf("ExtractUsedImports: %v", err)
	}
	foundFmtImp := false
	for _, imp := range imps {
		if strings.Contains(imp.Path.Value, "fmt") {
			foundFmtImp = true
		}
	}
	if !foundFmtImp {
		t.Errorf("expected fmt import spec, got %v", imps)
	}
}

func TestExtractUsedHelpers_TreeShakeDeep(t *testing.T) {
	entry := loadTestdata(t, "tree_shake_deep")
	body := findClientSideStateBody(t, entry)

	decls, _, err := ExtractUsedHelpers(entry.Pkg, body)
	if err != nil {
		t.Fatalf("ExtractUsedHelpers: %v", err)
	}
	// helper1 (body root) → helper2 → helper3 — but body root *is* helper1.
	// So traversal from helper1's body discovers helper2 and helper3.
	if len(decls) != 2 {
		t.Errorf("expected 2 transitive helpers (helper2, helper3), got %d", len(decls))
	}
	names := map[string]bool{}
	for _, d := range decls {
		if fd, ok := d.(*ast.FuncDecl); ok {
			names[fd.Name.Name] = true
		}
	}
	if !names["helper2"] || !names["helper3"] {
		t.Errorf("expected helper2 and helper3, got %v", names)
	}
}

func TestStripPackageAndImports(t *testing.T) {
	cases := []struct {
		name        string
		src         string
		wantContain []string
		wantAbsent  []string
	}{
		{
			name: "plain file",
			src: `package foo

import "fmt"

func Hello() { fmt.Println("hi") }
`,
			wantContain: []string{"func Hello()", "fmt.Println"},
			wantAbsent:  []string{"package foo", "import"},
		},
		{
			name: "parenthesized import block",
			src: `package foo

import (
	"fmt"
	"os"
)

func F() { _ = fmt.Sprint; _ = os.Stdout }
`,
			wantContain: []string{"func F()"},
			wantAbsent:  []string{"package foo", "import", `"os"`},
		},
		{
			name: "single line import",
			src: `package foo

import "x"

func G() {}
`,
			wantContain: []string{"func G()"},
			wantAbsent:  []string{"package foo", "import", `"x"`},
		},
		{
			name: "mixed imports dot and alias",
			src: `package foo

import (
	. "fmt"
	myfmt "fmt"
)

func H() { Println("hi"); _ = myfmt.Sprint }
`,
			wantContain: []string{"func H()", "myfmt.Sprint"},
			wantAbsent:  []string{"package foo", "import (", `. "fmt"`, `myfmt "fmt"`},
		},
		{
			name: "doc comment preserved",
			src: `package foo

// MyType is a documented struct.
type MyType struct {
	A int
}
`,
			wantContain: []string{"MyType is a documented struct", "type MyType struct"},
			wantAbsent:  []string{"package foo"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := StripPackageAndImports(tc.src)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			for _, want := range tc.wantContain {
				if !strings.Contains(out, want) {
					t.Errorf("output missing %q\noutput:\n%s", want, out)
				}
			}
			for _, absent := range tc.wantAbsent {
				if strings.Contains(out, absent) {
					t.Errorf("output unexpectedly contains %q\noutput:\n%s", absent, out)
				}
			}
		})
	}

	t.Run("parse error", func(t *testing.T) {
		_, err := StripPackageAndImports("this is not valid go @@@")
		if err == nil {
			t.Fatalf("expected error for invalid go source, got nil")
		}
	})
}

func TestExtractUsedHelpers_PkgVarRejected(t *testing.T) {
	entry := loadTestdata(t, "pkg_var_error")
	body := findClientSideStateBody(t, entry)

	_, _, err := ExtractUsedHelpers(entry.Pkg, body)
	if err == nil {
		t.Fatalf("expected error for package-level var reference, got nil")
	}
	if !strings.Contains(err.Error(), "package-level var") {
		t.Errorf("expected 'package-level var' error, got: %v", err)
	}
}
