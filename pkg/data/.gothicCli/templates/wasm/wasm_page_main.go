//go:build js && wasm
// Code generated from {{.SourceFile}} — DO NOT EDIT.

package main

import (
	. "wasm-runtime/runtime"
{{range .StdImports}}	{{.}}
{{end}})

// ── auto-generated codecs (do not edit) ─────────────────────────────────
{{range .Codecs}}
func _encode_{{.Name}}(v {{.Name}}, e *Encoder) {
{{range .Fields}}	{{.EncLine}}
{{end}}}

func _decode_{{.Name}}(d *Decoder) {{.Name}} {
	var v {{.Name}}
{{range .Fields}}	{{.DecLine}}
{{end}}	return v
}

func _encode_slice{{.Name}}(v []{{.Name}}, e *Encoder) {
	e.U32(uint32(len(v)))
	for _, _item := range v { _encode_{{.Name}}(_item, e) }
}

func _decode_slice{{.Name}}(d *Decoder) []{{.Name}} {
	_n := int(d.U32())
	v := make([]{{.Name}}, _n)
	for _i := range v { v[_i] = _decode_{{.Name}}(d) }
	return v
}
{{end}}
{{range .KeyVars}}var {{.StructName}}Key = BinaryKey("{{.KeyName}}", _encode_{{.StructName}}, _decode_{{.StructName}})

{{end}}
{{range .CtxTypes}}type {{.TypeName}} struct {
{{range .Fields}}	{{.Name}} *ContextField[{{.Type}}]
{{end}}	_online  bool
	_pending string
}

{{end}}
{{range .WasmFuncs}}{{$fn := .}}func {{.CtorName}}(initial ...{{.StructName}}) *{{.TypeName}} {
	ctx := &{{.TypeName}}{
{{range .Fields}}		{{.Name}}: NewContextField({{$fn.StructName}}{}.{{.Name}}),
{{end}}	}
	broadcast := func() {
		e := NewEncoder(64)
		_encode_{{$fn.StructName}}({{$fn.StructName}}{
{{range .Fields}}			{{.Name}}: ctx.{{.Name}}.Peek(),
{{end}}		}, e)
		encoded := HexEncode(e.Buf)
		if ctx._online {
			RequestCtxSet("{{$fn.KeyName}}", encoded)
		} else {
			ctx._pending = encoded
		}
	}
{{range .Fields}}	ctx.{{.Name}}.SetBroadcast(broadcast)
{{end}}	if _stored, _ok := ReadCtxStore("{{$fn.KeyName}}"); _ok {
		_init := _decode_{{$fn.StructName}}(&Decoder{Buf: HexDecode(_stored)})
{{range .Fields}}		ctx.{{.Name}}.ApplyExternal(_init.{{.Name}})
{{end}}		ctx._online = true
	}
	ListenCtxOnline("{{$fn.KeyName}}", func(detail string) {
		decoded := _decode_{{$fn.StructName}}(&Decoder{Buf: HexDecode(detail)})
{{range .Fields}}		ctx.{{.Name}}.ApplyExternal(decoded.{{.Name}})
{{end}}		if !ctx._online {
			ctx._online = true
			if ctx._pending != "" {
				RequestCtxSet("{{$fn.KeyName}}", ctx._pending)
				ctx._pending = ""
			}
		}
	})
	ListenCtxEvent("{{$fn.KeyName}}", func(detail string) {
		decoded := _decode_{{$fn.StructName}}(&Decoder{Buf: HexDecode(detail)})
{{range .Fields}}		ctx.{{.Name}}.ApplyExternal(decoded.{{.Name}})
{{end}}	})
	PingUntilOnline("{{$fn.KeyName}}", func() bool { return ctx._online })
	return ctx
}

func (c *{{.TypeName}}) Set(v {{.StructName}}) {
	e := NewEncoder(64)
	_encode_{{.StructName}}(v, e)
	encoded := HexEncode(e.Buf)
	if c._online {
		RequestCtxSet("{{.KeyName}}", encoded)
	} else {
		c._pending = encoded
	}
}

{{end}}
{{range .CtxSnippets}}
{{.}}

{{end}}
func main() {
{{.Body}}}
