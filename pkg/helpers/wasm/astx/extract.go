package astx

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/packages"
)

// StripPackageAndImports parses src as a Go file and returns the formatted
// source with the package clause and all import declarations removed. Doc
// comments on remaining declarations are preserved.
func StripPackageAndImports(src string) (string, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", src, parser.ParseComments)
	if err != nil {
		return "", &PosError{Pos: fset.Position(token.Pos(1)), Msg: fmt.Sprintf("parse error: %v", err)}
	}
	var parts []string
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if ok && gd.Tok == token.IMPORT {
			continue
		}
		var buf bytes.Buffer
		if err := format.Node(&buf, fset, decl); err != nil {
			return "", &PosError{Pos: fset.Position(decl.Pos()), Msg: fmt.Sprintf("format error: %v", err)}
		}
		parts = append(parts, buf.String())
	}
	return strings.Join(parts, "\n\n"), nil
}

// PageConfigResult holds extracted data from a routes.RouteConfig composite literal.
type PageConfigResult struct {
	Body        *ast.BlockStmt
	Compression string
	Compiler    string
}

// ExtractClientSideStateBody scans entry.File for a composite literal whose
// type (per TypesInfo) is routes.RouteConfig and which contains a
// ClientSideState key. It supports either an inline FuncLit value or a named
// function ident (resolved through TypesInfo.Uses → its FuncDecl).
//
// It also extracts the WasmCompression field value (as a plain identifier
// name like "BROTLI" or "GZIP", or a string literal if written as such) when
// present.
//
// Returns (result, true, nil) when found, (zero, false, nil) when no such
// literal exists, and a *PosError when the literal is malformed.
func ExtractClientSideStateBody(entry Entry) (PageConfigResult, bool, error) {
	pkg := entry.Pkg
	if pkg == nil || pkg.TypesInfo == nil || entry.File == nil {
		return PageConfigResult{}, false, nil
	}

	var (
		result   PageConfigResult
		found    bool
		posErr   *PosError
		stopWalk bool
	)

	fset := pkg.Fset

	ast.Inspect(entry.File, func(n ast.Node) bool {
		if stopWalk {
			return false
		}
		cl, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		tv, hasType := pkg.TypesInfo.Types[cl]
		if !hasType || tv.Type == nil {
			return true
		}
		typeStr := tv.Type.String()
		if !strings.Contains(typeStr, "helpers/routes.RouteConfig") {
			return true
		}

		var bodyBlock *ast.BlockStmt
		var compression string
		var compiler string
		var haveCSS bool

		for _, elt := range cl.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			keyIdent, ok := kv.Key.(*ast.Ident)
			if !ok {
				continue
			}
			switch keyIdent.Name {
			case "ClientSideState":
				haveCSS = true
				switch v := kv.Value.(type) {
				case *ast.FuncLit:
					bodyBlock = v.Body
				case *ast.Ident:
					obj := pkg.TypesInfo.Uses[v]
					if obj == nil {
						posErr = &PosError{Pos: fset.Position(v.Pos()), Msg: fmt.Sprintf("ClientSideState references unresolved identifier %q", v.Name)}
						stopWalk = true
						return false
					}
					fnObj, ok := obj.(*types.Func)
					if !ok {
						posErr = &PosError{Pos: fset.Position(v.Pos()), Msg: fmt.Sprintf("ClientSideState references non-function %q", v.Name)}
						stopWalk = true
						return false
					}
					decl := findFuncDeclForObj(pkg, fnObj)
					if decl == nil || decl.Body == nil {
						posErr = &PosError{Pos: fset.Position(v.Pos()), Msg: fmt.Sprintf("ClientSideState func %q: declaration body not found", v.Name)}
						stopWalk = true
						return false
					}
					bodyBlock = decl.Body
				default:
					posErr = &PosError{Pos: fset.Position(kv.Value.Pos()), Msg: "ClientSideState must be a func literal or a named function identifier"}
					stopWalk = true
					return false
				}
			case "WasmCompression":
				switch v := kv.Value.(type) {
				case *ast.BasicLit:
					compression = strings.Trim(v.Value, "\"`")
				case *ast.Ident:
					compression = v.Name
				case *ast.SelectorExpr:
					compression = v.Sel.Name
				}
			case "WasmCompiler":
				switch v := kv.Value.(type) {
				case *ast.BasicLit:
					compiler = strings.Trim(v.Value, "\"`")
				case *ast.Ident:
					compiler = v.Name
				case *ast.SelectorExpr:
					compiler = v.Sel.Name
				}
			}
		}

		if haveCSS && bodyBlock != nil {
			result = PageConfigResult{Body: bodyBlock, Compression: compression, Compiler: compiler}
			found = true
			stopWalk = true
			return false
		}
		return true
	})

	if posErr != nil {
		return PageConfigResult{}, false, posErr
	}
	return result, found, nil
}

// findFuncDeclForObj searches pkg.Syntax for the *ast.FuncDecl matching obj.
func findFuncDeclForObj(pkg *packages.Package, obj *types.Func) *ast.FuncDecl {
	for _, f := range pkg.Syntax {
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok {
				continue
			}
			if pkg.TypesInfo.Defs[fd.Name] == obj {
				return fd
			}
		}
	}
	return nil
}

// findDeclForObj searches pkg.Syntax for the ast.Decl that defines obj.
func findDeclForObj(pkg *packages.Package, obj types.Object) ast.Decl {
	for _, f := range pkg.Syntax {
		for _, d := range f.Decls {
			switch decl := d.(type) {
			case *ast.FuncDecl:
				if pkg.TypesInfo.Defs[decl.Name] == obj {
					return decl
				}
			case *ast.GenDecl:
				for _, spec := range decl.Specs {
					switch s := spec.(type) {
					case *ast.ValueSpec:
						for _, name := range s.Names {
							if pkg.TypesInfo.Defs[name] == obj {
								return decl
							}
						}
					case *ast.TypeSpec:
						if pkg.TypesInfo.Defs[s.Name] == obj {
							return decl
						}
					}
				}
			}
		}
	}
	return nil
}

// ExtractUsedImports returns the import specs in pkg that are referenced from
// the AST subtree rooted at root. Imports are deduplicated by path; aliases
// from the original spec are preserved.
func ExtractUsedImports(pkg *packages.Package, root ast.Node) ([]*ast.ImportSpec, error) {
	if pkg == nil || pkg.TypesInfo == nil {
		return nil, nil
	}
	usedPaths := make(map[string]bool)

	ast.Inspect(root, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if !ok {
			return true
		}
		obj := pkg.TypesInfo.Uses[id]
		if obj == nil {
			return true
		}
		if pn, ok := obj.(*types.PkgName); ok {
			usedPaths[pn.Imported().Path()] = true
		}
		return true
	})

	if len(usedPaths) == 0 {
		return nil, nil
	}

	seen := make(map[string]bool)
	var out []*ast.ImportSpec
	for _, f := range pkg.Syntax {
		for _, imp := range f.Imports {
			path := strings.Trim(imp.Path.Value, "\"")
			if !usedPaths[path] || seen[path] {
				continue
			}
			seen[path] = true
			out = append(out, imp)
		}
	}
	return out, nil
}

// ExtractUsedHelpers performs a BFS over identifier references starting at
// root. Same-package function/const/type declarations referenced
// (transitively) are returned in discovery order. External package names are
// collected separately and deduplicated by imported *types.Package.
//
// Package-level vars are rejected with a positioned error to keep WASM bodies
// pure (no hidden mutable shared state pulled into the generated main).
func ExtractUsedHelpers(pkg *packages.Package, root ast.Node) ([]ast.Decl, []*types.PkgName, error) {
	if pkg == nil || pkg.TypesInfo == nil {
		return nil, nil, nil
	}

	queue := []ast.Node{root}
	seenObj := map[types.Object]bool{}
	seenPkg := map[*types.Package]bool{}

	var decls []ast.Decl
	var pkgNames []*types.PkgName
	var posErr *PosError

	for len(queue) > 0 && posErr == nil {
		node := queue[0]
		queue = queue[1:]

		ast.Inspect(node, func(x ast.Node) bool {
			if posErr != nil {
				return false
			}
			id, ok := x.(*ast.Ident)
			if !ok {
				return true
			}
			obj := pkg.TypesInfo.Uses[id]
			if obj == nil || seenObj[obj] {
				return true
			}
			seenObj[obj] = true

			switch o := obj.(type) {
			case *types.PkgName:
				if imported := o.Imported(); imported != nil && !seenPkg[imported] {
					seenPkg[imported] = true
					pkgNames = append(pkgNames, o)
				}
			case *types.Func, *types.Const, *types.TypeName:
				if obj.Pkg() != pkg.Types {
					return true
				}
				decl := findDeclForObj(pkg, obj)
				if decl == nil {
					return true
				}
				if err := assertPure(decl); err != nil {
					posErr = &PosError{Pos: pkg.Fset.Position(decl.Pos()), Msg: err.Error()}
					return false
				}
				decls = append(decls, decl)
				queue = append(queue, decl)
				_ = o
			case *types.Var:
				if o.Pkg() == pkg.Types && o.Parent() == pkg.Types.Scope() {
					posErr = &PosError{
						Pos: pkg.Fset.Position(id.Pos()),
						Msg: fmt.Sprintf("ClientSideState references package-level var %q — only func, const, and type declarations can be tree-shaken into WASM; move %q inside the ClientSideState body", id.Name, id.Name),
					}
					return false
				}
			}
			return true
		})
	}

	if posErr != nil {
		return nil, nil, posErr
	}
	return decls, pkgNames, nil
}

// assertPure rejects declarations that shouldn't be inlined into a WASM body
// (currently: init functions).
func assertPure(decl ast.Decl) error {
	if fd, ok := decl.(*ast.FuncDecl); ok {
		if fd.Name != nil && fd.Name.Name == "init" {
			return fmt.Errorf("init() functions cannot be inlined into ClientSideState")
		}
	}
	return nil
}
