package helpers

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/scanner"
	"go/token"
	"sort"
	"strconv"
	"strings"
)

// Source rewriting: transforms the user's source code before it is embedded
// into the generated WASM main.go. Two rewrites today:
//
//  1. rewriteAutoKeys — `AutoKey[T]("name")` → `BinaryKey[T]("name", _encode_T, _decode_T)`.
//  2. rewriteTopicCalls — `UseTopic(MyKey, MyCtx{...})` → `MyTopic(MyCtx{...})`.
//
// The implementation is AST-based (handles arbitrary whitespace, multi-line
// calls, nested generic type arguments like `map[string]int`). Parse failures
// surface as hard errors with file:line:col position info so callers can
// abort the build.

// rewriteAutoKeys rewrites every `AutoKey[T]("name")` call inside src to the
// equivalent `BinaryKey[T]("name", _encode_T, _decode_T)` call. The input is
// a slice of top-level declarations stripped of the `package` line and the
// `import` block (see collectTopicSnippets). Returns an error with
// positional info on AST parse failure — callers must abort.
func (h *WasmHelper) rewriteAutoKeys(src string) (string, error) {
	return astRewriteAutoKeys(src)
}

// astRewriteAutoKeys rewrites every AutoKey[T]("name") call inside src using
// the Go AST. Returns (rewritten source, nil) on success; ("", error) when
// the source cannot be parsed. The error carries file:line:col position info.
//
// In production the source is a fragment of top-level declarations (no
// package clause, no imports). For unit-test convenience we also accept bare
// expressions/statements by re-wrapping inside a function body if the
// top-level parse fails. Position information from the AST is used to
// perform surgical text replacements, which preserves the surrounding
// formatting (comments, whitespace).
func astRewriteAutoKeys(src string) (string, error) {
	const topWrap = "package _x\n"
	fset := token.NewFileSet()
	wrapper := topWrap
	file, err := parser.ParseFile(fset, "", wrapper+src, parser.ParseComments)
	if err != nil {
		// Retry as a function body — handles bare expressions/statements.
		fset2 := token.NewFileSet()
		wrapper2 := "package _x\nfunc _f() {\n"
		file2, err2 := parser.ParseFile(fset2, "", wrapper2+src+"\n}\n", parser.ParseComments)
		if err2 != nil {
			return "", firstPositionedError(err, len(wrapper), err2, len(wrapper2))
		}
		fset = fset2
		wrapper = wrapper2
		file = file2
	}

	type edit struct {
		start, end int
		repl       string
	}
	var edits []edit

	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		// Detect AutoKey[T] or AutoKey[T, U] — IndexExpr (single type arg) or
		// IndexListExpr (multiple type args; AutoKey only has one but we
		// handle both for forward-compat).
		var typeExpr ast.Expr
		switch fn := call.Fun.(type) {
		case *ast.IndexExpr:
			id, ok := fn.X.(*ast.Ident)
			if !ok || id.Name != "AutoKey" {
				return true
			}
			typeExpr = fn.Index
		case *ast.IndexListExpr:
			id, ok := fn.X.(*ast.Ident)
			if !ok || id.Name != "AutoKey" || len(fn.Indices) == 0 {
				return true
			}
			typeExpr = fn.Indices[0]
		default:
			return true
		}

		if len(call.Args) != 1 {
			return true
		}
		nameLit, ok := call.Args[0].(*ast.BasicLit)
		if !ok || nameLit.Kind != token.STRING {
			return true
		}

		// Render the type expression and the string literal back to text.
		var typBuf bytes.Buffer
		if err := format.Node(&typBuf, fset, typeExpr); err != nil {
			return true
		}
		typ := typBuf.String()
		nameStr, err := strconv.Unquote(nameLit.Value)
		if err != nil {
			return true
		}

		encFn, decFn := autoKeyHelperNames(typ)
		replacement := fmt.Sprintf(`BinaryKey[%s]("%s", %s, %s)`, typ, nameStr, encFn, decFn)

		// Compute byte offsets within the wrapped source. Subtract len(wrapper)
		// (the prefix we prepended for parsing) to convert back to offsets in
		// the original src.
		start := fset.Position(call.Pos()).Offset - len(wrapper)
		end := fset.Position(call.End()).Offset - len(wrapper)
		if start < 0 || end < start || end > len(src) {
			return true
		}
		edits = append(edits, edit{start: start, end: end, repl: replacement})
		return true
	})

	if len(edits) == 0 {
		return src, nil
	}

	// Apply edits in reverse offset order so earlier edits' offsets stay valid.
	sort.Slice(edits, func(i, j int) bool { return edits[i].start > edits[j].start })
	out := src
	for _, e := range edits {
		out = out[:e.start] + e.repl + out[e.end:]
	}
	return out, nil
}

// firstPositionedError extracts the first parser/scanner error from one of two
// candidate parse failures (the top-level retry and the func-body retry) and
// returns a positioned error string. The wrapperLen is subtracted from line
// offsets so positions refer to the caller's original src when possible.
func firstPositionedError(err1 error, _ int, err2 error, _ int) error {
	pick := err1
	if pick == nil {
		pick = err2
	}
	if pick == nil {
		return fmt.Errorf("ast parse failed")
	}
	if list, ok := pick.(scanner.ErrorList); ok && len(list) > 0 {
		e := list[0]
		return fmt.Errorf("%s: %s", e.Pos.String(), e.Msg)
	}
	return pick
}

// autoKeyHelperNames returns the encode/decode helper names that match the
// codec generator's output. The codec generator emits `_encode_<Name>` and
// `_encode_slice<Name>` for struct types and slices of struct types; for
// primitive types it emits `_encode_<primitive>` and `_encode_slice<primitive>`.
// For any other type (maps, pointers, nested types), bracket and space
// characters are stripped so the resulting identifier is a valid Go ident
// (e.g. `map[string]int` → `_encode_mapstringint`).
func autoKeyHelperNames(typ string) (string, string) {
	if strings.HasPrefix(typ, "[]") {
		elem := normalizeTypeIdent(typ[2:])
		return "_encode_slice" + elem, "_decode_slice" + elem
	}
	n := normalizeTypeIdent(typ)
	return "_encode_" + n, "_decode_" + n
}

// normalizeTypeIdent strips characters that are illegal inside a Go identifier
// from a type string. Used to derive _encode/_decode helper names for compound
// types like `map[string]int` → `mapstringint`.
func normalizeTypeIdent(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '[', ']', ' ', '\t', '\n', '*':
			// skip
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// rewriteTopicCalls rewrites the four legacy "use the topic X" spellings
// inside src to the canonical `<Name>Topic(...)` constructor call. The
// input is the body of a `ClientSideState` function (no package, no func
// header). We wrap it as `package _x; func _f() { ... }` for parsing.
//
// The four spellings handled per struct s (with KeyName != ""):
//
//	UseTopic(<Name>Key, <Name>{...})  → <Name>Topic(<Name>{...})
//	UseTopic(<Name>{...})             → <Name>Topic(<Name>{...})
//	Use<Name>(<Name>{...})            → <Name>Topic(<Name>{...})
//	Use<Name>Topic(<Name>{...})       → <Name>Topic(<Name>{...})
//
// On AST-parse failure we return a positioned error so the caller can abort.
func (h *WasmHelper) rewriteTopicCalls(src string, structs []structInfo) (string, error) {
	return astRewriteTopicCalls(src, structs)
}

// astRewriteTopicCalls rewrites the four legacy "use topic" spellings into
// the canonical `<Name>Topic(...)` call using the Go AST. Returns the
// rewritten source plus nil on success; ("", error) when parsing fails.
func astRewriteTopicCalls(src string, structs []structInfo) (string, error) {
	// Build a lookup of valid struct names → ctor name. Only structs with a
	// non-empty KeyName participate.
	ctorFor := make(map[string]string, len(structs))
	for _, s := range structs {
		if s.KeyName == "" {
			continue
		}
		ctorFor[s.Name] = s.Name + "Topic"
	}
	if len(ctorFor) == 0 {
		return src, nil
	}

	const prefix = "package _x\nfunc _f() {\n"
	const suffix = "\n}\n"
	wrapped := prefix + src + suffix
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", wrapped, parser.ParseComments)
	if err != nil {
		if list, ok := err.(scanner.ErrorList); ok && len(list) > 0 {
			e := list[0]
			return "", fmt.Errorf("%s: %s", e.Pos.String(), e.Msg)
		}
		return "", err
	}

	type edit struct {
		start, end int
		repl       string
	}
	var edits []edit

	// firstArgTypeName returns the type-name of the first call argument when
	// the argument is a struct literal (composite literal) whose type is a
	// plain identifier. For other shapes (e.g. variables, selectors) it
	// returns "".
	firstArgTypeName := func(args []ast.Expr) string {
		if len(args) == 0 {
			return ""
		}
		cl, ok := args[0].(*ast.CompositeLit)
		if !ok {
			return ""
		}
		switch t := cl.Type.(type) {
		case *ast.Ident:
			return t.Name
		}
		return ""
	}

	// Render a slice of args back to text using go/format on each, joined by
	// ", ". We use the original source via positions when possible to
	// preserve formatting exactly.
	renderArgs := func(args []ast.Expr) string {
		if len(args) == 0 {
			return ""
		}
		parts := make([]string, 0, len(args))
		for _, a := range args {
			start := fset.Position(a.Pos()).Offset
			end := fset.Position(a.End()).Offset
			if start >= 0 && end >= start && end <= len(wrapped) {
				parts = append(parts, wrapped[start:end])
				continue
			}
			var buf bytes.Buffer
			if err := format.Node(&buf, fset, a); err == nil {
				parts = append(parts, buf.String())
			}
		}
		return strings.Join(parts, ", ")
	}

	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		fnIdent, ok := call.Fun.(*ast.Ident)
		if !ok {
			return true
		}
		fnName := fnIdent.Name

		var ctor string
		var newArgs []ast.Expr

		switch {
		case fnName == "UseTopic":
			// Three flavors:
			//  UseTopic(NameKey, NameValue) — first arg is the Key ident
			//  UseTopic(Name,    NameValue) — first arg is the struct name ident
			//  UseTopic(NameValue)          — single struct-literal arg
			if len(call.Args) >= 2 {
				if id, ok := call.Args[0].(*ast.Ident); ok {
					candidate := id.Name
					if strings.HasSuffix(candidate, "Key") {
						candidate = strings.TrimSuffix(candidate, "Key")
					}
					if c, ok := ctorFor[candidate]; ok {
						ctor = c
						newArgs = call.Args[1:]
						break
					}
				}
			}
			if name := firstArgTypeName(call.Args); name != "" {
				if c, ok := ctorFor[name]; ok {
					ctor = c
					newArgs = call.Args
				}
			}
		case strings.HasPrefix(fnName, "Use"):
			rest := strings.TrimPrefix(fnName, "Use")
			// UseNameTopic(NameValue)
			if strings.HasSuffix(rest, "Topic") {
				name := strings.TrimSuffix(rest, "Topic")
				if c, ok := ctorFor[name]; ok {
					ctor = c
					newArgs = call.Args
				}
			}
			// UseName(NameValue)
			if ctor == "" {
				if c, ok := ctorFor[rest]; ok {
					ctor = c
					newArgs = call.Args
				}
			}
		}

		if ctor == "" {
			return true
		}

		start := fset.Position(call.Pos()).Offset
		end := fset.Position(call.End()).Offset
		if start < len(prefix) || end > len(wrapped)-len(suffix) {
			return true
		}
		// Convert wrapped offsets to src offsets.
		srcStart := start - len(prefix)
		srcEnd := end - len(prefix)

		replacement := ctor + "(" + renderArgs(newArgs) + ")"
		edits = append(edits, edit{start: srcStart, end: srcEnd, repl: replacement})
		return true
	})

	if len(edits) == 0 {
		return src, nil
	}
	sort.Slice(edits, func(i, j int) bool { return edits[i].start > edits[j].start })
	out := src
	for _, e := range edits {
		out = out[:e.start] + e.repl + out[e.end:]
	}
	return out, nil
}
