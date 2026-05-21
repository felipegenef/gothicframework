package helpers

import (
	"fmt"
	"go/ast"
)

// typeRef captures the structural shape of a Go type for codec generation.
// It is built once during parseStructsFromSource and consumed by the codec pipeline.
type typeRef interface {
	String() string
	typeRef()
}

type Named struct{ Name string }
type SliceOf struct{ Elem typeRef }
type MapOf struct{ Key, Val typeRef }
type PointerOf struct{ Elem typeRef }

func (Named) typeRef()     {}
func (SliceOf) typeRef()   {}
func (MapOf) typeRef()     {}
func (PointerOf) typeRef() {}

func (n Named) String() string     { return n.Name }
func (s SliceOf) String() string   { return "[]" + s.Elem.String() }
func (m MapOf) String() string     { return "map[" + m.Key.String() + "]" + m.Val.String() }
func (p PointerOf) String() string { return "*" + p.Elem.String() }

// typeRefFromExpr walks ast.Expr and returns the corresponding typeRef.
// Returns nil and error for unsupported shapes (channel, interface, func, anonymous struct).
func typeRefFromExpr(expr ast.Expr) (typeRef, error) {
	switch e := expr.(type) {
	case *ast.Ident:
		return Named{Name: e.Name}, nil
	case *ast.SelectorExpr:
		x, ok := e.X.(*ast.Ident)
		if !ok {
			return nil, fmt.Errorf("unsupported selector type: %T", e.X)
		}
		return Named{Name: x.Name + "." + e.Sel.Name}, nil
	case *ast.ArrayType:
		if e.Len != nil {
			// Fixed-size array — treat as slice of element (matches astTypeString behavior)
			return typeRefFromExpr(e.Elt)
		}
		elem, err := typeRefFromExpr(e.Elt)
		if err != nil {
			return nil, err
		}
		return SliceOf{Elem: elem}, nil
	case *ast.StarExpr:
		elem, err := typeRefFromExpr(e.X)
		if err != nil {
			return nil, err
		}
		return PointerOf{Elem: elem}, nil
	case *ast.MapType:
		k, err := typeRefFromExpr(e.Key)
		if err != nil {
			return nil, err
		}
		v, err := typeRefFromExpr(e.Value)
		if err != nil {
			return nil, err
		}
		return MapOf{Key: k, Val: v}, nil
	default:
		return nil, fmt.Errorf("unsupported type expression: %T", expr)
	}
}
