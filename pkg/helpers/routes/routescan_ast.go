package helpers

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
)

// astScanResult holds the AST-extracted facts about a single route source file.
// It replaces the five regex fields previously stored on FileBasedRouteHelper.
type astScanResult struct {
	PackageName        string // package declaration name (e.g. "pages")
	RouteConfigName    string // name of the `var X = routes.RouteConfig[T]{...}` declaration
	ApiRouteConfigName string // name of the `var X = routes.ApiRouteConfig{...}` declaration
	RouteFuncName      string // name of the first exported func returning templ.Component
	ApiFuncName        string // name of the first top-level func with no return values
}
//
// Why parser.ParseFile (not the astx.Loader from wasm/astx)?
//
// The route scanner runs in user-project context, file by file, including in
// hot-reload where source may be mid-edit. The astx.Loader requires a fully
// type-checkable `go/packages` load of the whole module ("./..."), which is
// expensive (~hundreds of ms even on small projects) and fails noisily when
// any file in the module has a transient parse error. Route discovery only
// needs syntactic facts — package name, var declaration names by spelling
// (`routes.RouteConfig[...]`), and func signatures — so a single
// parser.ParseFile call is the right granularity. If go/packages-grade
// type-info becomes necessary later (e.g. detecting aliased imports of the
// routes package), this can be upgraded to use astx.Loader without changing
// the call sites in fileBasedRouting.go.

// astScanFile parses filePath and extracts the route-related identifiers that
// the regex scanner used to discover. Returns an error if the file cannot be
// read or parsed; callers should treat errors as "skip this file" rather than
// fatal, matching the regex behavior (which would silently return zero
// matches on malformed input).
func astScanFile(filePath string) (astScanResult, error) {
	var res astScanResult

	src, err := os.ReadFile(filePath)
	if err != nil {
		return res, fmt.Errorf("astScanFile: read %s: %w", filePath, err)
	}

	fset := token.NewFileSet()
	// SkipObjectResolution keeps parsing fast; we do not need *ast.Object links.
	f, err := parser.ParseFile(fset, filePath, src, parser.SkipObjectResolution)
	if err != nil {
		return res, fmt.Errorf("astScanFile: parse %s: %w", filePath, err)
	}

	res.PackageName = f.Name.Name

	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			if d.Tok != token.VAR {
				continue
			}
			for _, spec := range d.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok || len(vs.Names) == 0 || len(vs.Values) == 0 {
					continue
				}
				// Look at the RHS composite literal type:
				//   var X = routes.RouteConfig[T]{...}
				//   var X = routes.ApiRouteConfig{...}
				cl, ok := vs.Values[0].(*ast.CompositeLit)
				if !ok {
					continue
				}
				name := vs.Names[0].Name
				switch typeName := selectorTypeName(cl.Type); typeName {
				case "routes.RouteConfig":
					if res.RouteConfigName == "" {
						res.RouteConfigName = name
					}
				case "routes.ApiRouteConfig":
					if res.ApiRouteConfigName == "" {
						res.ApiRouteConfigName = name
					}
				}
			}

		case *ast.FuncDecl:
			// Skip methods — the regex required `^func\s+(\w+)\s*\(`, which
			// does not match `func (r *Recv) Name(...)`.
			if d.Recv != nil {
				continue
			}
			if d.Type == nil {
				continue
			}
			results := d.Type.Results
			// First top-level func with no return value → API handler candidate.
			if results == nil || len(results.List) == 0 {
				if res.ApiFuncName == "" {
					res.ApiFuncName = d.Name.Name
				}
				continue
			}
			// First top-level func returning templ.Component → route func.
			if res.RouteFuncName == "" && returnsTemplComponent(results) {
				res.RouteFuncName = d.Name.Name
			}
		}
	}

	return res, nil
}

// selectorTypeName extracts the dotted name of a composite-literal type, e.g.
// `routes.RouteConfig[T]` → "routes.RouteConfig", `routes.ApiRouteConfig` →
// "routes.ApiRouteConfig". Returns "" if the type isn't a qualified selector.
func selectorTypeName(expr ast.Expr) string {
	// Unwrap `routes.RouteConfig[T]` → `routes.RouteConfig`.
	if idx, ok := expr.(*ast.IndexExpr); ok {
		expr = idx.X
	}
	// Generics with multiple type args: `routes.RouteConfig[A, B]`.
	if idx, ok := expr.(*ast.IndexListExpr); ok {
		expr = idx.X
	}
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	xIdent, ok := sel.X.(*ast.Ident)
	if !ok {
		return ""
	}
	return xIdent.Name + "." + sel.Sel.Name
}

// returnsTemplComponent reports whether a function's result list is exactly
// one result of type `templ.Component`.
func returnsTemplComponent(results *ast.FieldList) bool {
	if results == nil || len(results.List) != 1 {
		return false
	}
	field := results.List[0]
	if len(field.Names) > 1 {
		return false
	}
	sel, ok := field.Type.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	xIdent, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	return xIdent.Name == "templ" && sel.Sel.Name == "Component"
}
