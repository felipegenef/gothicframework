package helpers

import (
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"strings"
)

// Codec line generation. Each function returns the encode/decode lines that
// will be embedded in the generated topic_gen.go / wasm_page_main.go for
// a given struct field. The wire format mirrors pkg/wasm/stubs.go and
// pkg/wasm/wasm-runtime/runtime/codec.go.

// typeKind classifies a (resolved) Go type for codec dispatch.
type typeKind int

const (
	kindPrimitive typeKind = iota
	kindBytes              // []byte — single Bytes() call, not a slice loop
	kindSlice
	kindMap
	kindPointer
	kindStruct
	kindUnknown
)

// resolveType returns the underlying type for typ if it appears in aliases,
// else typ unchanged. The caller is responsible for tracking whether the
// returned type differs from the input.
func resolveType(typ string, aliases map[string]string) string {
	if underlying, ok := aliases[typ]; ok {
		return underlying
	}
	return typ
}

// kindOf classifies (already alias-resolved) typeRef for codec dispatch.
// It returns kindUnknown when the type cannot be encoded with the current
// codec; callers should produce a useful error in that case.
func kindOf(ref typeRef, structs map[string]bool) typeKind {
	switch t := ref.(type) {
	case SliceOf:
		if named, ok := t.Elem.(Named); ok && named.Name == "byte" {
			return kindBytes
		}
		return kindSlice
	case MapOf:
		return kindMap
	case PointerOf:
		return kindPointer
	case Named:
		if pe, _ := primitiveCodec(t.Name, "_"); pe != "" {
			return kindPrimitive
		}
		if structs[t.Name] {
			return kindStruct
		}
		return kindUnknown
	}
	return kindUnknown
}

// resolveTypeRef returns the underlying typeRef for ref if it is a Named
// alias appearing in refAliases, else ref unchanged.
func resolveTypeRef(ref typeRef, refAliases map[string]typeRef) typeRef {
	if named, ok := ref.(Named); ok {
		if resolved, found := refAliases[named.Name]; found {
			return resolved
		}
	}
	return ref
}

// primitiveCodec returns the encode/decode expressions for a single primitive value
// referenced by variable name varExpr (e.g. "v.Field" or "_item").
// Returns ("", "") if the type is not a known primitive.
func primitiveCodec(typ, varExpr string) (enc, dec string) {
	switch typ {
	case "bool":
		return fmt.Sprintf("e.Bool(bool(%s))", varExpr), fmt.Sprintf("%s = bool(d.Bool())", varExpr)
	case "string":
		return fmt.Sprintf("e.String(string(%s))", varExpr), fmt.Sprintf("%s = string(d.String())", varExpr)
	case "int":
		return fmt.Sprintf("e.I64(int64(%s))", varExpr), fmt.Sprintf("%s = int(d.I64())", varExpr)
	case "int8":
		return fmt.Sprintf("e.I32(int32(%s))", varExpr), fmt.Sprintf("%s = int8(d.I32())", varExpr)
	case "int16":
		return fmt.Sprintf("e.I32(int32(%s))", varExpr), fmt.Sprintf("%s = int16(d.I32())", varExpr)
	case "int32", "rune":
		return fmt.Sprintf("e.I32(int32(%s))", varExpr), fmt.Sprintf("%s = int32(d.I32())", varExpr)
	case "int64":
		return fmt.Sprintf("e.I64(int64(%s))", varExpr), fmt.Sprintf("%s = int64(d.I64())", varExpr)
	case "uint8", "byte":
		return fmt.Sprintf("e.U8(uint8(%s))", varExpr), fmt.Sprintf("%s = uint8(d.U8())", varExpr)
	case "uint16":
		return fmt.Sprintf("e.U16(uint16(%s))", varExpr), fmt.Sprintf("%s = uint16(d.U16())", varExpr)
	case "uint32":
		return fmt.Sprintf("e.U32(uint32(%s))", varExpr), fmt.Sprintf("%s = uint32(d.U32())", varExpr)
	case "uint":
		return fmt.Sprintf("e.U64(uint64(%s))", varExpr), fmt.Sprintf("%s = uint(d.U64())", varExpr)
	case "uint64":
		return fmt.Sprintf("e.U64(uint64(%s))", varExpr), fmt.Sprintf("%s = uint64(d.U64())", varExpr)
	case "float32":
		return fmt.Sprintf("e.F32(float32(%s))", varExpr), fmt.Sprintf("%s = float32(d.F32())", varExpr)
	case "float64":
		return fmt.Sprintf("e.F64(float64(%s))", varExpr), fmt.Sprintf("%s = float64(d.F64())", varExpr)
	case "time.Time":
		return fmt.Sprintf("e.I64(%s.UnixNano())", varExpr),
			fmt.Sprintf("%s = time.Unix(0, d.I64())", varExpr)
	}
	return "", ""
}

func (h *WasmHelper) codecLines(fi fieldInfo, structNames map[string]bool, aliases map[string]string, refAliases map[string]typeRef) (enc, dec string, err error) {
	n := fi.Name
	typ := fi.Type
	tag := fi.GothicTag

	if tag == "skip" {
		return "", "", nil
	}

	// Explicit gothic tag overrides — only valid on int/uint fields.
	if tag != "" {
		switch tag {
		case "i32":
			return fmt.Sprintf("e.I32(int32(v.%s))", n), fmt.Sprintf("v.%s = int(d.I32())", n), nil
		case "i64":
			return fmt.Sprintf("e.I64(int64(v.%s))", n), fmt.Sprintf("v.%s = int(d.I64())", n), nil
		case "u32":
			return fmt.Sprintf("e.U32(uint32(v.%s))", n), fmt.Sprintf("v.%s = uint(d.U32())", n), nil
		case "u64":
			return fmt.Sprintf("e.U64(uint64(v.%s))", n), fmt.Sprintf("v.%s = uint(d.U64())", n), nil
		default:
			return "", "", fmt.Errorf("unknown gothic tag %q (valid: skip, i32, i64, u32, u64)", tag)
		}
	}

	ref := fi.TypeRef
	if ref == nil {
		return "", "", fmt.Errorf("field %s: missing TypeRef (parser fell through on type %q)", fi.Name, fi.Type)
	}
	resolvedRef := resolveTypeRef(ref, refAliases)

	switch kindOf(resolvedRef, structNames) {
	case kindBytes:
		return fmt.Sprintf("e.Bytes(v.%s)", n), fmt.Sprintf("v.%s = d.Bytes()", n), nil

	case kindPrimitive:
		resolvedName := resolvedRef.(Named).Name
		pe, pd := primitiveCodec(resolvedName, "v."+n)
		if resolvedName != typ {
			// Type alias — fix decode to cast back to the alias type.
			pd = fmt.Sprintf("{ _v := %s; v.%s = %s(_v) }", strings.Replace(pd, "v."+n+" = ", "", 1), n, typ)
		}
		return pe, pd, nil

	case kindSlice:
		return h.sliceCodecLines(n, resolvedRef.(SliceOf).Elem, structNames, aliases, refAliases)

	case kindMap:
		return h.mapCodecLines(n, resolvedRef.(MapOf), structNames, aliases, refAliases)

	case kindPointer:
		return h.pointerCodecLines(n, resolvedRef.(PointerOf).Elem, structNames, aliases, refAliases)

	case kindStruct:
		// Known struct defined in src/context/ — prefer original name when present,
		// fall back to the resolved name (handles `type MyItem Item`).
		structTyp := typ
		resolvedName := resolvedRef.(Named).Name
		if !structNames[typ] && structNames[resolvedName] {
			structTyp = resolvedName
		}
		return fmt.Sprintf("_encode_%s(v.%s, e)", structTyp, n),
			fmt.Sprintf("v.%s = _decode_%s(d)", n, structTyp), nil
	}

	return "", "", fmt.Errorf(
		"unsupported type %q — supported: primitives, []T, map[K]V, *T, time.Time, and structs defined in src/topics/\n"+
			"  Tip: add `gothic:\"skip\"` to exclude this field from the topic wire format",
		typ,
	)
}

// captureLines emits the body of a _capture<FieldName>(d *Decoder) []byte helper.
// The body advances the decoder past this field's bytes (without writing to any
// receiver struct) and returns a copy of the consumed byte range. This is used by
// the WASM32 heap-pressure fix: instead of decoding into a Go struct, we keep the
// raw wire bytes and re-decode lazily on demand.
//
// Implementation strategy: call codecLines to get the decode line(s), then
// mechanically strip writes to the (non-existent) `v.<FieldName>` receiver so
// that only side effects on `d` remain.
func (h *WasmHelper) captureLines(fi fieldInfo, structNames map[string]bool, aliases map[string]string, refAliases map[string]typeRef) (captureBody string, err error) {
	_, dec, err := h.codecLines(fi, structNames, aliases, refAliases)
	if err != nil {
		return "", err
	}
	if dec == "" {
		// gothic:"skip" — nothing to advance past.
		return "start := d.Pos\nreturn d.Buf[start:d.Pos]", nil
	}

	stripped := stripReceiverWrites(dec, fi.Name)
	// Return a zero-copy subslice of d.Buf — the caller is responsible for
	// keeping d.Buf alive for the lifetime of the returned slice. This avoids
	// per-click allocations proportional to the size of large fields (e.g.
	// big []Item or []byte image blobs) on TinyGo, which otherwise overwhelm
	// the GC and cause `unreachable` traps from heap exhaustion.
	return fmt.Sprintf("start := d.Pos\n%s\nreturn d.Buf[start:d.Pos]", stripped), nil
}

// stripReceiverWrites rewrites a generated decode snippet so it no longer
// references `v.<fieldName>` (the receiver struct). The transformations:
//
//  1. `v.<F> = make(T, _n);` → ``  (drop the allocation entirely; we only advance d)
//  2. `for _i := range v.<F>` → `for _i := 0; _i < _n; _i++`
//  3. `v.<F>[_i] = <expr>` → `_ = <expr>`
//  4. `v.<F>[_k] = <expr>` → `_ = <expr>`
//  5. `v.<F> = <expr>` → `_ = <expr>`  (catch-all, runs last)
func stripReceiverWrites(dec, fieldName string) string {
	prefix := "v." + fieldName
	out := dec

	// 1. Drop slice/map allocation: `v.F = make(...);` — we don't need it.
	out = dropMakeAssignments(out, prefix)

	// 2. Range-over-slice: `for _i := range v.F` → `for _i := 0; _i < _n; _i++`
	out = strings.ReplaceAll(out, "for _i := range "+prefix, "for _i := 0; _i < _n; _i++")

	// 3/4. Indexed writes: `v.F[_i] = ` and `v.F[_k] = ` → `_ = `
	out = strings.ReplaceAll(out, prefix+"[_i] = ", "_ = ")
	// For maps, the decoded key `_k` is no longer read after we drop the
	// receiver write — add an explicit discard so TinyGo doesn't error on
	// "declared and not used". We replace the map write with `_ = _k; _ = `
	// so the existing `_v` discard still consumes the value expression.
	out = strings.ReplaceAll(out, prefix+"[_k] = ", "_ = _k; _ = ")

	// 5. Catch-all top-level assignment: `v.F = ` → `_ = `
	out = strings.ReplaceAll(out, prefix+" = ", "_ = ")

	return out
}

// dropMakeAssignments removes `<prefix> = make(...)` statements from src by
// parsing src as a function body and walking its AST. This correctly handles
// adversarial cases like `make(...)` arguments containing strings with `)`
// inside them, which a naive paren-counter would mis-parse.
//
// prefix is of the form "v.FieldName"; only AssignStmts whose LHS is exactly
// that selector and whose RHS is a `make(...)` call are dropped.
func dropMakeAssignments(src, prefix string) string {
	fset := token.NewFileSet()
	wrapped := "package _x\nfunc _f() {\n" + src + "\n}\n"
	f, err := parser.ParseFile(fset, "", wrapped, 0)
	if err != nil {
		// src wasn't parseable as a sequence of statements — leave it alone.
		return src
	}

	var body *ast.BlockStmt
	for _, decl := range f.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Name.Name != "_f" {
			continue
		}
		body = fd.Body
	}
	if body == nil {
		return src
	}

	dotIdx := strings.Index(prefix, ".")
	if dotIdx < 0 {
		return src
	}
	receiverName := prefix[:dotIdx]
	fieldName := prefix[dotIdx+1:]

	var kept []ast.Stmt
	for _, stmt := range body.List {
		assign, ok := stmt.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) == 0 || len(assign.Rhs) == 0 {
			kept = append(kept, stmt)
			continue
		}
		sel, ok := assign.Lhs[0].(*ast.SelectorExpr)
		if !ok {
			kept = append(kept, stmt)
			continue
		}
		ident, ok := sel.X.(*ast.Ident)
		if !ok || ident.Name != receiverName || sel.Sel.Name != fieldName {
			kept = append(kept, stmt)
			continue
		}
		call, ok := assign.Rhs[0].(*ast.CallExpr)
		if !ok {
			kept = append(kept, stmt)
			continue
		}
		fun, ok := call.Fun.(*ast.Ident)
		if !ok || fun.Name != "make" {
			kept = append(kept, stmt)
			continue
		}
		// Matching `v.<fieldName> = make(...)` — drop.
	}

	if len(kept) == 0 {
		return ""
	}

	var buf strings.Builder
	for i, stmt := range kept {
		var sb strings.Builder
		if err := format.Node(&sb, fset, stmt); err != nil {
			return src
		}
		if i > 0 {
			buf.WriteString("; ")
		}
		buf.WriteString(sb.String())
	}
	return buf.String()
}

func (h *WasmHelper) sliceCodecLines(fieldName string, elemRef typeRef, structNames map[string]bool, aliases map[string]string, refAliases map[string]typeRef) (enc, dec string, err error) {
	elem := elemRef.String()

	// [][]T — nested slice
	if inner, ok := elemRef.(SliceOf); ok {
		innerStr := inner.Elem.String()
		innerEnc, innerDec, err := h.sliceCodecLines("_inner", inner.Elem, structNames, aliases, refAliases)
		if err != nil {
			return "", "", fmt.Errorf("[]%s: %w", elem, err)
		}
		// innerEnc/Dec reference "_inner" — wrap in a helper closure inline
		enc = fmt.Sprintf(
			"{ e.U32(uint32(len(v.%s))); for _, _row := range v.%s { var _inner []%s; _ = _inner; %s } }",
			fieldName, fieldName, innerStr, strings.ReplaceAll(innerEnc, "v._inner", "_row"))
		dec = fmt.Sprintf(
			"{ _n := int(d.U32()); v.%s = make([][]%s, _n); for _i := range v.%s { var _inner []%s; %s; v.%s[_i] = _inner } }",
			fieldName, innerStr, fieldName, innerStr,
			strings.ReplaceAll(innerDec, "v._inner", "_inner"),
			fieldName)
		return enc, dec, nil
	}

	// []KnownStruct
	if named, ok := elemRef.(Named); ok && structNames[named.Name] {
		enc = fmt.Sprintf(
			"{ e.U32(uint32(len(v.%s))); for _, _item := range v.%s { _encode_%s(_item, e) } }",
			fieldName, fieldName, named.Name)
		dec = fmt.Sprintf(
			"{ _n := int(d.U32()); v.%s = make([]%s, _n); for _i := range v.%s { v.%s[_i] = _decode_%s(d) } }",
			fieldName, named.Name, fieldName, fieldName, named.Name)
		return enc, dec, nil
	}

	// []primitive (including type aliases over primitives)
	resolvedElemRef := resolveTypeRef(elemRef, refAliases)
	if resolvedNamed, ok := resolvedElemRef.(Named); ok {
		if pe, pd := primitiveCodec(resolvedNamed.Name, "_item"); pe != "" {
			if resolvedNamed.Name != elem {
				// Decode: primitiveCodec returns the underlying type; cast back to the alias.
				rhs := strings.TrimPrefix(pd, "_item = ")
				pd = fmt.Sprintf("_item = %s(%s)", elem, rhs)
			}
			enc = fmt.Sprintf(
				"{ e.U32(uint32(len(v.%s))); for _, _item := range v.%s { %s } }",
				fieldName, fieldName, pe)
			dec = fmt.Sprintf(
				"{ _n := int(d.U32()); v.%s = make([]%s, _n); for _i := range v.%s { var _item %s; %s; v.%s[_i] = _item } }",
				fieldName, elem, fieldName, elem, pd, fieldName)
			return enc, dec, nil
		}
	}

	// []*T — slice of pointers
	if ptr, ok := elemRef.(PointerOf); ok {
		base := ptr.Elem.String()
		resolvedBaseRef := resolveTypeRef(ptr.Elem, refAliases)
		if resolvedBaseNamed, ok := resolvedBaseRef.(Named); ok {
			if pe, pd := primitiveCodec(resolvedBaseNamed.Name, "_pv"); pe != "" {
				if resolvedBaseNamed.Name != base {
					rhs := strings.TrimPrefix(pd, "_pv = ")
					pd = fmt.Sprintf("_pv = %s(%s)", base, rhs)
				}
				itemEnc := fmt.Sprintf("if _item == nil { e.U8(0) } else { e.U8(1); _pv := *_item; %s }", pe)
				itemDec := fmt.Sprintf("if d.U8() != 0 { var _pv %s; %s; v.%s[_i] = &_pv }", base, pd, fieldName)
				enc = fmt.Sprintf(
					"{ e.U32(uint32(len(v.%s))); for _, _item := range v.%s { %s } }",
					fieldName, fieldName, itemEnc)
				dec = fmt.Sprintf(
					"{ _n := int(d.U32()); v.%s = make([]%s, _n); for _i := range v.%s { %s } }",
					fieldName, elem, fieldName, itemDec)
				return enc, dec, nil
			}
		}

		structT := base
		if baseNamed, ok := ptr.Elem.(Named); ok {
			if !structNames[baseNamed.Name] {
				if resolvedNamed, ok := resolvedBaseRef.(Named); ok && structNames[resolvedNamed.Name] {
					structT = resolvedNamed.Name
				}
			}
		}
		if structNames[structT] {
			itemEnc := fmt.Sprintf("if _item == nil { e.U8(0) } else { e.U8(1); _encode_%s(*_item, e) }", structT)
			itemDec := fmt.Sprintf("if d.U8() != 0 { _sv := _decode_%s(d); v.%s[_i] = &_sv }", structT, fieldName)
			enc = fmt.Sprintf(
				"{ e.U32(uint32(len(v.%s))); for _, _item := range v.%s { %s } }",
				fieldName, fieldName, itemEnc)
			dec = fmt.Sprintf(
				"{ _n := int(d.U32()); v.%s = make([]%s, _n); for _i := range v.%s { %s } }",
				fieldName, elem, fieldName, itemDec)
			return enc, dec, nil
		}

		return "", "", fmt.Errorf("slice element pointer type %q base type %q is not a supported primitive or known struct", elem, base)
	}

	return "", "", fmt.Errorf("slice element type %q is not supported", elem)
}

func (h *WasmHelper) mapCodecLines(fieldName string, m MapOf, structNames map[string]bool, aliases map[string]string, refAliases map[string]typeRef) (enc, dec string, err error) {
	keyTyp := m.Key.String()
	valTyp := m.Val.String()
	typ := m.String()

	resolvedKeyRef := resolveTypeRef(m.Key, refAliases)
	resolvedKeyTyp := resolvedKeyRef.String()
	resolvedValRef := resolveTypeRef(m.Val, refAliases)
	resolvedValTyp := resolvedValRef.String()

	keyEnc, keyDec := primitiveCodec(resolvedKeyTyp, "_k")
	if keyEnc == "" {
		return "", "", fmt.Errorf("map key type %q is not a supported primitive", keyTyp)
	}
	if resolvedKeyTyp != keyTyp {
		rhs := strings.TrimPrefix(keyDec, "_k = ")
		keyDec = fmt.Sprintf("_k = %s(%s)", keyTyp, rhs)
	}

	var valEnc, valDec string
	if ve, vd := primitiveCodec(resolvedValTyp, "_v"); ve != "" {
		valEnc, valDec = ve, vd
		if resolvedValTyp != valTyp {
			rhs := strings.TrimPrefix(valDec, "_v = ")
			valDec = fmt.Sprintf("_v = %s(%s)", valTyp, rhs)
		}
	} else if ptr, ok := m.Val.(PointerOf); ok {
		// map[K]*V — pointer value: nil tag byte + encoded value.
		baseVal := ptr.Elem.String()
		resolvedBaseRef := resolveTypeRef(ptr.Elem, refAliases)
		if resolvedBaseNamed, ok := resolvedBaseRef.(Named); ok {
			if pe, pd := primitiveCodec(resolvedBaseNamed.Name, "_pv"); pe != "" {
				if resolvedBaseNamed.Name != baseVal {
					rhs := strings.TrimPrefix(pd, "_pv = ")
					pd = fmt.Sprintf("_pv = %s(%s)", baseVal, rhs)
				}
				valEnc = fmt.Sprintf("{ if _v == nil { e.U8(0) } else { e.U8(1); _pv := *_v; %s } }", pe)
				valDec = fmt.Sprintf("if d.U8() != 0 { var _pv %s; %s; _v = &_pv }", baseVal, pd)
				goto emit
			}
		}
		// struct pointer
		structT := baseVal
		if baseNamed, ok := ptr.Elem.(Named); ok {
			if !structNames[baseNamed.Name] {
				if resolvedNamed, ok := resolvedBaseRef.(Named); ok && structNames[resolvedNamed.Name] {
					structT = resolvedNamed.Name
				}
			}
		}
		if !structNames[structT] {
			return "", "", fmt.Errorf("map value pointer type %q base type is not a supported primitive or known struct", "*"+baseVal)
		}
		valEnc = fmt.Sprintf("{ if _v == nil { e.U8(0) } else { e.U8(1); _encode_%s(*_v, e) } }", structT)
		valDec = fmt.Sprintf("if d.U8() != 0 { _sv := _decode_%s(d); _v = &_sv }", structT)
	} else if named, ok := m.Val.(Named); ok && structNames[named.Name] {
		valEnc = fmt.Sprintf("_encode_%s(_v, e)", named.Name)
		valDec = fmt.Sprintf("_v = _decode_%s(d)", named.Name)
	} else if inner, ok := m.Val.(MapOf); ok {
		// map[K]map[K2]V — nested map. Emit a length-prefixed inner loop using
		// _k2/_v2/_n2 so we don't shadow the outer _k/_v/_n bindings.
		innerKeyTyp := inner.Key.String()
		innerValTyp := inner.Val.String()
		innerTyp := inner.String()

		resolvedInnerKeyRef := resolveTypeRef(inner.Key, refAliases)
		resolvedInnerKeyTyp := resolvedInnerKeyRef.String()
		resolvedInnerValRef := resolveTypeRef(inner.Val, refAliases)
		resolvedInnerValTyp := resolvedInnerValRef.String()

		innerKeyEnc, innerKeyDec := primitiveCodec(resolvedInnerKeyTyp, "_k2")
		if innerKeyEnc == "" {
			return "", "", fmt.Errorf("nested map key type %q is not a supported primitive", innerKeyTyp)
		}
		if resolvedInnerKeyTyp != innerKeyTyp {
			rhs := strings.TrimPrefix(innerKeyDec, "_k2 = ")
			innerKeyDec = fmt.Sprintf("_k2 = %s(%s)", innerKeyTyp, rhs)
		}

		var innerValEnc, innerValDec string
		if ve, vd := primitiveCodec(resolvedInnerValTyp, "_v2"); ve != "" {
			innerValEnc, innerValDec = ve, vd
			if resolvedInnerValTyp != innerValTyp {
				rhs := strings.TrimPrefix(innerValDec, "_v2 = ")
				innerValDec = fmt.Sprintf("_v2 = %s(%s)", innerValTyp, rhs)
			}
		} else if innerNamed, ok := inner.Val.(Named); ok && structNames[innerNamed.Name] {
			innerValEnc = fmt.Sprintf("_encode_%s(_v2, e)", innerNamed.Name)
			innerValDec = fmt.Sprintf("_v2 = _decode_%s(d)", innerNamed.Name)
		} else {
			return "", "", fmt.Errorf("nested map value type %q is not a supported primitive or known struct", innerValTyp)
		}

		valEnc = fmt.Sprintf(
			"{ e.U32(uint32(len(_v))); for _k2, _v2 := range _v { %s; %s } }",
			innerKeyEnc, innerValEnc)
		valDec = fmt.Sprintf(
			"{ _n2 := int(d.U32()); _v = make(%s, _n2); for _j := 0; _j < _n2; _j++ { var _k2 %s; var _v2 %s; %s; %s; _v[_k2] = _v2 } }",
			innerTyp, innerKeyTyp, innerValTyp, innerKeyDec, innerValDec)
	} else {
		return "", "", fmt.Errorf("map value type %q is not a supported primitive or known struct", valTyp)
	}

emit:
	enc = fmt.Sprintf(
		"{ e.U32(uint32(len(v.%s))); for _k, _v := range v.%s { %s; %s } }",
		fieldName, fieldName, keyEnc, valEnc)
	dec = fmt.Sprintf(
		"{ _n := int(d.U32()); v.%s = make(%s, _n); for _i := 0; _i < _n; _i++ { var _k %s; var _v %s; %s; %s; v.%s[_k] = _v } }",
		fieldName, typ, keyTyp, valTyp, keyDec, valDec, fieldName)
	return enc, dec, nil
}

func (h *WasmHelper) pointerCodecLines(fieldName string, baseRef typeRef, structNames map[string]bool, aliases map[string]string, refAliases map[string]typeRef) (enc, dec string, err error) {
	var valEnc, valDec string

	baseTyp := baseRef.String()
	resolvedBaseRef := resolveTypeRef(baseRef, refAliases)
	resolvedBase := resolvedBaseRef.String()

	if resolvedNamed, ok := resolvedBaseRef.(Named); ok {
		if pe, pd := primitiveCodec(resolvedNamed.Name, "_pv"); pe != "" {
			valEnc = pe
			valDec = pd
			if resolvedBase != baseTyp {
				// Decode: cast back from the underlying type to the alias.
				rhs := strings.TrimPrefix(valDec, "_pv = ")
				valDec = fmt.Sprintf("_pv = %s(%s)", baseTyp, rhs)
			}
			enc = fmt.Sprintf(
				"{ if v.%s == nil { e.U8(0) } else { e.U8(1); _pv := *v.%s; %s } }",
				fieldName, fieldName, valEnc)
			dec = fmt.Sprintf(
				"{ if d.U8() != 0 { var _pv %s; %s; v.%s = &_pv } }",
				baseTyp, valDec, fieldName)
			return enc, dec, nil
		}
	}

	if baseNamed, ok := baseRef.(Named); ok && structNames[baseNamed.Name] {
		valEnc = fmt.Sprintf("_encode_%s(*v.%s, e)", baseNamed.Name, fieldName)
		valDec = fmt.Sprintf("{ _sv := _decode_%s(d); v.%s = &_sv }", baseNamed.Name, fieldName)
		enc = fmt.Sprintf("{ if v.%s == nil { e.U8(0) } else { e.U8(1); %s } }", fieldName, valEnc)
		dec = fmt.Sprintf("{ if d.U8() != 0 { %s } }", valDec)
		return enc, dec, nil
	}

	return "", "", fmt.Errorf("pointer element type %q is not a supported primitive or known struct", baseTyp)
}

func (h *WasmHelper) buildCodecData(structs []structInfo, aliases map[string]string, refAliases map[string]typeRef) ([]StructCodecData, error) {
	names := make(map[string]bool, len(structs))
	for _, s := range structs {
		names[s.Name] = true
	}
	result := make([]StructCodecData, 0, len(structs))
	for _, s := range structs {
		sd := StructCodecData{Name: s.Name}
		for _, f := range s.Fields {
			enc, dec, err := h.codecLines(f, names, aliases, refAliases)
			if err != nil {
				return nil, fmt.Errorf("struct %s field %s: %w", s.Name, f.Name, err)
			}
			sd.Fields = append(sd.Fields, FieldCodec{Name: f.Name, EncLine: enc, DecLine: dec})
		}
		result = append(result, sd)
	}
	return result, nil
}

func (h *WasmHelper) buildKeyVarData(structs []structInfo) []KeyVarData {
	var result []KeyVarData
	for _, s := range structs {
		if s.KeyName == "" {
			continue
		}
		result = append(result, KeyVarData{StructName: s.Name, KeyName: s.KeyName})
	}
	return result
}

func (h *WasmHelper) buildTopicTypeData(structs []structInfo) []TopicTypeData {
	var result []TopicTypeData
	for _, s := range structs {
		if s.KeyName == "" {
			continue
		}
		td := TopicTypeData{TypeName: h.topicTypeName(s.Name)}
		for _, f := range s.Fields {
			td.Fields = append(td.Fields, TopicFieldData{Name: f.Name, Type: f.Type})
		}
		result = append(result, td)
	}
	return result
}

// buildManagerFieldData produces one ManagerFieldData per field of the named
// context struct, in declaration order.
func (h *WasmHelper) buildManagerFieldData(s structInfo, structNames map[string]bool, aliases map[string]string, refAliases map[string]typeRef) ([]ManagerFieldData, error) {
	out := make([]ManagerFieldData, 0, len(s.Fields))
	for _, f := range s.Fields {
		enc, dec, err := h.codecLines(f, structNames, aliases, refAliases)
		if err != nil {
			return nil, fmt.Errorf("manager field %s: %w", f.Name, err)
		}
		capture, err := h.captureLines(f, structNames, aliases, refAliases)
		if err != nil {
			return nil, fmt.Errorf("capture %s: %w", f.Name, err)
		}
		out = append(out, ManagerFieldData{
			FieldName:   f.Name,
			EncodeLines: enc,
			DecodeLines: dec,
			CaptureBody: capture,
		})
	}
	return out, nil
}

// buildPerFieldCodecs produces one PerFieldCodec per field of the named context
// struct, in declaration order. Used by the consumer (page) template.
func (h *WasmHelper) buildPerFieldCodecs(s structInfo, structNames map[string]bool, aliases map[string]string, refAliases map[string]typeRef) ([]PerFieldCodec, error) {
	out := make([]PerFieldCodec, 0, len(s.Fields))
	for _, f := range s.Fields {
		enc, dec, err := h.codecLines(f, structNames, aliases, refAliases)
		if err != nil {
			return nil, fmt.Errorf("per-field codec %s: %w", f.Name, err)
		}
		out = append(out, PerFieldCodec{
			FieldName: f.Name,
			FieldType: f.Type,
			EncLines:  enc,
			DecLines:  dec,
		})
	}
	return out, nil
}

func (h *WasmHelper) buildWasmTopicFuncData(structs []structInfo, aliases map[string]string, refAliases map[string]typeRef) ([]WasmTopicFuncData, error) {
	structNames := make(map[string]bool, len(structs))
	for _, s := range structs {
		structNames[s.Name] = true
	}
	var result []WasmTopicFuncData
	for _, s := range structs {
		if s.KeyName == "" {
			continue
		}
		fd := WasmTopicFuncData{
			CtorName:   h.topicFuncNameFor(s),
			TypeName:   h.topicTypeName(s.Name),
			StructName: s.Name,
			KeyName:    s.KeyName,
		}
		for _, f := range s.Fields {
			fd.Fields = append(fd.Fields, TopicFieldData{Name: f.Name, Type: f.Type})
		}
		codecs, err := h.buildPerFieldCodecs(s, structNames, aliases, refAliases)
		if err != nil {
			return nil, fmt.Errorf("struct %s: %w", s.Name, err)
		}
		fd.FieldCodecs = codecs
		result = append(result, fd)
	}
	return result, nil
}

func (h *WasmHelper) buildServerTopicFuncData(structs []structInfo) []ServerTopicFuncData {
	var result []ServerTopicFuncData
	for _, s := range structs {
		if s.KeyName == "" {
			continue
		}
		fd := ServerTopicFuncData{
			CtorName:   h.topicFuncNameFor(s),
			TypeName:   h.topicTypeName(s.Name),
			StructName: s.Name,
		}
		for _, f := range s.Fields {
			fd.Fields = append(fd.Fields, TopicFieldData{Name: f.Name, Type: f.Type})
		}
		result = append(result, fd)
	}
	return result
}

func (h *WasmHelper) buildMountFnData(structs []structInfo) []MountFnData {
	var result []MountFnData
	for _, s := range structs {
		if s.KeyName == "" {
			continue
		}
		compressionConst := "routes.GZIP"
		if s.Compression == WasmCompressionBrotli {
			compressionConst = "routes.BROTLI"
		}
		result = append(result, MountFnData{
			FuncName:         h.componentFuncNameFor(s),
			WasmName:         "topic-" + s.KeyName,
			Compression:      s.Compression,
			CompressionConst: compressionConst,
		})
	}
	return result
}
