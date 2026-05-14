//go:build js && wasm
// Code generated for context manager — DO NOT EDIT.

package main

import (
	. "wasm-runtime/runtime"
)

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
{{range .CtxSnippets}}
{{.}}

{{end}}
var _ctxState {{.StructName}}

func _broadcastOnline() {
	e := NewEncoder(64)
	_encode_{{.StructName}}(_ctxState, e)
	BroadcastCtxOnline("{{.KeyName}}", HexEncode(e.Buf))
}

func main() {
	if stored, ok := ReadCtxStore("{{.KeyName}}"); ok {
		_ctxState = _decode_{{.StructName}}(&Decoder{Buf: HexDecode(stored)})
	}
	_broadcastOnline()
	ListenCtxPing("{{.KeyName}}", func() { _broadcastOnline() })
	ListenCtxSetReq("{{.KeyName}}", func(detail string) {
		_ctxState = _decode_{{.StructName}}(&Decoder{Buf: HexDecode(detail)})
		e := NewEncoder(64)
		_encode_{{.StructName}}(_ctxState, e)
		BroadcastCtxEncoded("{{.KeyName}}", HexEncode(e.Buf))
	})
	select {}
}
