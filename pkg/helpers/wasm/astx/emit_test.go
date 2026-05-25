package astx

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

func TestFormatNode_FuncDecl(t *testing.T) {
	src := `package p

func Hello() string {
	return "hi"
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "x.go", src, 0)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	var fn *ast.FuncDecl
	for _, d := range file.Decls {
		if f, ok := d.(*ast.FuncDecl); ok {
			fn = f
			break
		}
	}
	if fn == nil {
		t.Fatalf("no func decl found")
	}
	out, err := FormatNode(fn, fset)
	if err != nil {
		t.Fatalf("FormatNode: %v", err)
	}
	if out == "" {
		t.Fatalf("empty output")
	}
	if !strings.Contains(out, "Hello") {
		t.Fatalf("expected output to contain func name 'Hello', got: %q", out)
	}
}

func TestFormatNode_BlockStmt(t *testing.T) {
	src := `package p

func F() {
	a := 1
	_ = a
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "x.go", src, 0)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	fn := file.Decls[0].(*ast.FuncDecl)
	out, err := FormatNode(fn.Body, fset)
	if err != nil {
		t.Fatalf("FormatNode: %v", err)
	}
	if strings.HasPrefix(out, "{") {
		t.Fatalf("expected leading '{' stripped, got: %q", out)
	}
	if strings.HasSuffix(out, "}") {
		t.Fatalf("expected trailing '}' stripped, got: %q", out)
	}
	if !strings.Contains(out, "a := 1") {
		t.Fatalf("expected body content, got: %q", out)
	}
}
