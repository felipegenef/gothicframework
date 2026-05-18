package helpers

import (
	"fmt"
	"strings"
)

// Codec line generation. Each function returns the encode/decode lines that
// will be embedded in the generated context_gen.go / wasm_page_main.go for
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

// kindOf classifies resolved (already alias-resolved) for codec dispatch.
// It returns kindUnknown when the type cannot be encoded with the current
// codec; callers should produce a useful error in that case.
func kindOf(resolved string, structs map[string]bool) typeKind {
	if resolved == "[]byte" {
		return kindBytes
	}
	if strings.HasPrefix(resolved, "[]") {
		return kindSlice
	}
	if strings.HasPrefix(resolved, "map[") {
		return kindMap
	}
	if strings.HasPrefix(resolved, "*") {
		return kindPointer
	}
	// Probe primitiveCodec with a dummy var expression — if it returns
	// non-empty, this is a primitive (or time.Time, treated as one).
	if pe, _ := primitiveCodec(resolved, "_"); pe != "" {
		return kindPrimitive
	}
	if structs[resolved] {
		return kindStruct
	}
	return kindUnknown
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

func (h *WasmHelper) codecLines(fi fieldInfo, structNames map[string]bool, aliases map[string]string) (enc, dec string, err error) {
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

	// Resolve type alias to its underlying type for codec selection, while keeping
	// the original type name for cast expressions in the generated code.
	resolvedTyp := resolveType(typ, aliases)
	// effectiveTyp = the form used for shape detection (slice/map/pointer/struct).
	effectiveTyp := typ
	if resolvedTyp != typ {
		effectiveTyp = resolvedTyp
	}

	switch kindOf(effectiveTyp, structNames) {
	case kindBytes:
		return fmt.Sprintf("e.Bytes(v.%s)", n), fmt.Sprintf("v.%s = d.Bytes()", n), nil

	case kindPrimitive:
		pe, pd := primitiveCodec(resolvedTyp, "v."+n)
		if resolvedTyp != typ {
			// Type alias — fix decode to cast back to the alias type.
			pd = fmt.Sprintf("{ _v := %s; v.%s = %s(_v) }", strings.Replace(pd, "v."+n+" = ", "", 1), n, typ)
		}
		return pe, pd, nil

	case kindSlice:
		return h.sliceCodecLines(n, effectiveTyp[2:], structNames, aliases)

	case kindMap:
		return h.mapCodecLines(n, effectiveTyp, structNames, aliases)

	case kindPointer:
		return h.pointerCodecLines(n, effectiveTyp[1:], structNames, aliases)

	case kindStruct:
		// Known struct defined in src/context/ — prefer original name when present,
		// fall back to the resolved name (handles `type MyItem Item`).
		structTyp := typ
		if !structNames[typ] && structNames[effectiveTyp] {
			structTyp = effectiveTyp
		}
		return fmt.Sprintf("_encode_%s(v.%s, e)", structTyp, n),
			fmt.Sprintf("v.%s = _decode_%s(d)", n, structTyp), nil
	}

	return "", "", fmt.Errorf(
		"unsupported type %q — supported: primitives, []T, map[K]V, *T, time.Time, and structs defined in src/context/\n"+
			"  Tip: add `gothic:\"skip\"` to exclude this field from the context wire format",
		typ,
	)
}

func (h *WasmHelper) sliceCodecLines(fieldName, elem string, structNames map[string]bool, aliases map[string]string) (enc, dec string, err error) {
	// [][]T — nested slice
	if strings.HasPrefix(elem, "[]") {
		inner := elem[2:]
		innerEnc, innerDec, err := h.sliceCodecLines("_inner", inner, structNames, aliases)
		if err != nil {
			return "", "", fmt.Errorf("[]%s: %w", elem, err)
		}
		// innerEnc/Dec reference "_inner" — wrap in a helper closure inline
		enc = fmt.Sprintf(
			"{ e.U32(uint32(len(v.%s))); for _, _row := range v.%s { var _inner []%s; _ = _inner; %s } }",
			fieldName, fieldName, inner, strings.ReplaceAll(innerEnc, "v._inner", "_row"))
		dec = fmt.Sprintf(
			"{ _n := int(d.U32()); v.%s = make([][]%s, _n); for _i := range v.%s { var _inner []%s; %s; v.%s[_i] = _inner } }",
			fieldName, inner, fieldName, inner,
			strings.ReplaceAll(innerDec, "v._inner", "_inner"),
			fieldName)
		return enc, dec, nil
	}

	// []KnownStruct
	if structNames[elem] {
		enc = fmt.Sprintf(
			"{ e.U32(uint32(len(v.%s))); for _, _item := range v.%s { _encode_%s(_item, e) } }",
			fieldName, fieldName, elem)
		dec = fmt.Sprintf(
			"{ _n := int(d.U32()); v.%s = make([]%s, _n); for _i := range v.%s { v.%s[_i] = _decode_%s(d) } }",
			fieldName, elem, fieldName, fieldName, elem)
		return enc, dec, nil
	}

	// []primitive (including type aliases over primitives)
	resolvedElem := elem
	if underlying, ok := aliases[elem]; ok {
		resolvedElem = underlying
	}
	if pe, pd := primitiveCodec(resolvedElem, "_item"); pe != "" {
		if resolvedElem != elem {
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

	// []*T — slice of pointers (nil tag byte + value), covering both primitive/alias and struct bases.
	if strings.HasPrefix(elem, "*") {
		base := elem[1:]
		resolvedBase := base
		if underlying, ok := aliases[base]; ok {
			resolvedBase = underlying
		}

		if pe, pd := primitiveCodec(resolvedBase, "_pv"); pe != "" {
			if resolvedBase != base {
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

		structT := base
		if !structNames[base] && structNames[resolvedBase] {
			structT = resolvedBase
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

func (h *WasmHelper) mapCodecLines(fieldName, typ string, structNames map[string]bool, aliases map[string]string) (enc, dec string, err error) {
	// Parse "map[K]V"
	inner := typ[4:] // strip "map["
	bracket := strings.Index(inner, "]")
	if bracket < 0 {
		return "", "", fmt.Errorf("malformed map type %q", typ)
	}
	keyTyp := inner[:bracket]
	valTyp := inner[bracket+1:]

	resolvedKeyTyp := keyTyp
	if underlying, ok := aliases[keyTyp]; ok {
		resolvedKeyTyp = underlying
	}
	resolvedValTyp := valTyp
	if underlying, ok := aliases[valTyp]; ok {
		resolvedValTyp = underlying
	}

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
	} else if strings.HasPrefix(valTyp, "*") {
		// map[K]*V — pointer value: nil tag byte + encoded value.
		baseVal := valTyp[1:]
		resolvedBaseVal := baseVal
		if underlying, ok := aliases[baseVal]; ok {
			resolvedBaseVal = underlying
		}
		if pe, pd := primitiveCodec(resolvedBaseVal, "_pv"); pe != "" {
			if resolvedBaseVal != baseVal {
				rhs := strings.TrimPrefix(pd, "_pv = ")
				pd = fmt.Sprintf("_pv = %s(%s)", baseVal, rhs)
			}
			valEnc = fmt.Sprintf("{ if _v == nil { e.U8(0) } else { e.U8(1); _pv := *_v; %s } }", pe)
			valDec = fmt.Sprintf("if d.U8() != 0 { var _pv %s; %s; _v = &_pv }", baseVal, pd)
		} else {
			structT := baseVal
			if !structNames[baseVal] && structNames[resolvedBaseVal] {
				structT = resolvedBaseVal
			}
			if !structNames[structT] {
				return "", "", fmt.Errorf("map value pointer type %q base type is not a supported primitive or known struct", valTyp)
			}
			valEnc = fmt.Sprintf("{ if _v == nil { e.U8(0) } else { e.U8(1); _encode_%s(*_v, e) } }", structT)
			valDec = fmt.Sprintf("if d.U8() != 0 { _sv := _decode_%s(d); _v = &_sv }", structT)
		}
	} else if structNames[valTyp] {
		valEnc = fmt.Sprintf("_encode_%s(_v, e)", valTyp)
		valDec = fmt.Sprintf("_v = _decode_%s(d)", valTyp)
	} else {
		return "", "", fmt.Errorf("map value type %q is not a supported primitive or known struct", valTyp)
	}

	enc = fmt.Sprintf(
		"{ e.U32(uint32(len(v.%s))); for _k, _v := range v.%s { %s; %s } }",
		fieldName, fieldName, keyEnc, valEnc)
	dec = fmt.Sprintf(
		"{ _n := int(d.U32()); v.%s = make(%s, _n); for _i := 0; _i < _n; _i++ { var _k %s; var _v %s; %s; %s; v.%s[_k] = _v } }",
		fieldName, typ, keyTyp, valTyp, keyDec, valDec, fieldName)
	return enc, dec, nil
}

func (h *WasmHelper) pointerCodecLines(fieldName, baseTyp string, structNames map[string]bool, aliases map[string]string) (enc, dec string, err error) {
	var valEnc, valDec string

	resolvedBase := baseTyp
	if underlying, ok := aliases[baseTyp]; ok {
		resolvedBase = underlying
	}

	if pe, pd := primitiveCodec(resolvedBase, "_pv"); pe != "" {
		valEnc = pe
		valDec = pd
		if resolvedBase != baseTyp {
			// Decode: cast back from the underlying type to the alias.
			rhs := strings.TrimPrefix(valDec, "_pv = ")
			valDec = fmt.Sprintf("_pv = %s(%s)", baseTyp, rhs)
		}
	} else if structNames[baseTyp] {
		valEnc = fmt.Sprintf("_encode_%s(*v.%s, e)", baseTyp, fieldName)
		valDec = fmt.Sprintf("{ _sv := _decode_%s(d); v.%s = &_sv }", baseTyp, fieldName)
		enc = fmt.Sprintf("{ if v.%s == nil { e.U8(0) } else { e.U8(1); %s } }", fieldName, valEnc)
		dec = fmt.Sprintf("{ if d.U8() != 0 { %s } }", valDec)
		return enc, dec, nil
	} else {
		return "", "", fmt.Errorf("pointer element type %q is not a supported primitive or known struct", baseTyp)
	}

	enc = fmt.Sprintf(
		"{ if v.%s == nil { e.U8(0) } else { e.U8(1); _pv := *v.%s; %s } }",
		fieldName, fieldName, valEnc)
	dec = fmt.Sprintf(
		"{ if d.U8() != 0 { var _pv %s; %s; v.%s = &_pv } }",
		baseTyp, valDec, fieldName)
	return enc, dec, nil
}

func (h *WasmHelper) buildCodecData(structs []structInfo, aliases map[string]string) ([]StructCodecData, error) {
	names := make(map[string]bool, len(structs))
	for _, s := range structs {
		names[s.Name] = true
	}
	result := make([]StructCodecData, 0, len(structs))
	for _, s := range structs {
		sd := StructCodecData{Name: s.Name}
		for _, f := range s.Fields {
			enc, dec, err := h.codecLines(f, names, aliases)
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

func (h *WasmHelper) buildCtxTypeData(structs []structInfo) []CtxTypeData {
	var result []CtxTypeData
	for _, s := range structs {
		if s.KeyName == "" {
			continue
		}
		td := CtxTypeData{TypeName: h.ctxTypeName(s.Name)}
		for _, f := range s.Fields {
			td.Fields = append(td.Fields, CtxFieldData{Name: f.Name, Type: f.Type})
		}
		result = append(result, td)
	}
	return result
}

func (h *WasmHelper) buildWasmCtxFuncData(structs []structInfo) []WasmCtxFuncData {
	var result []WasmCtxFuncData
	for _, s := range structs {
		if s.KeyName == "" {
			continue
		}
		fd := WasmCtxFuncData{
			CtorName:   h.ctxFuncName(s.Name),
			TypeName:   h.ctxTypeName(s.Name),
			StructName: s.Name,
			KeyName:    s.KeyName,
		}
		for _, f := range s.Fields {
			fd.Fields = append(fd.Fields, CtxFieldData{Name: f.Name, Type: f.Type})
		}
		result = append(result, fd)
	}
	return result
}

func (h *WasmHelper) buildServerCtxFuncData(structs []structInfo) []ServerCtxFuncData {
	var result []ServerCtxFuncData
	for _, s := range structs {
		if s.KeyName == "" {
			continue
		}
		fd := ServerCtxFuncData{
			CtorName:   h.ctxFuncName(s.Name),
			TypeName:   h.ctxTypeName(s.Name),
			StructName: s.Name,
		}
		for _, f := range s.Fields {
			fd.Fields = append(fd.Fields, CtxFieldData{Name: f.Name, Type: f.Type})
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
			FuncName:         "Add" + h.ctxFuncName(s.Name),
			WasmName:         "ctx-" + s.KeyName,
			Compression:      s.Compression,
			CompressionConst: compressionConst,
		})
	}
	return result
}
