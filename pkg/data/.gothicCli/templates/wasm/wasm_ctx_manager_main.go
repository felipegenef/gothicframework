//go:build js && wasm
// Code generated for context manager — DO NOT EDIT.

package main

import (
	. "wasm-runtime/runtime"
{{- if .HasTime}}
	"time"
{{- end}}
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
// ── Per-field capture helpers ────────────────────────────────────────────────
// Each _capture<FieldName>(d) advances d past the field without decoding into a
// receiver struct, and returns a copy of the consumed bytes. Used to slice the
// incoming whole-struct payload into per-field byte slices without allocating
// a full decoded struct (which can be huge for large slices/maps).
{{range .Fields}}
func _capture{{.FieldName}}(d *Decoder) []byte {
	{{.CaptureBody}}
}
{{end}}

var _fields = map[string][]byte{}
var _lastWholeEncoded []byte
var _wholeDirty bool

// _rebuildWhole concatenates all per-field byte slices into _lastWholeEncoded
// and re-anchors _fields[] into the freshly allocated buffer so the previous
// per-field source buffers can be GC'd. Without the re-anchor step, each
// _fields[] slice could keep a different older buffer alive (e.g. a 5 MB image
// buffer survives even after _lastWholeEncoded is reassigned), causing TinyGo
// heap pressure and `unreachable` traps under stress.
func _rebuildWhole() {
	total := 0
{{range .Fields}}	total += len(_fields["{{.FieldName}}"])
{{end}}	buf := make([]byte, 0, total)
{{range .Fields}}	buf = append(buf, _fields["{{.FieldName}}"]...)
{{end}}	_lastWholeEncoded = buf
	// Re-slice _fields[] into the new buffer so all per-field references share
	// a single underlying allocation. Drops references to the old source bufs.
	_captureAllFields(_lastWholeEncoded)
	_wholeDirty = false
}

// _ensureWholeFresh rebuilds _lastWholeEncoded lazily, only when something is
// about to read it. Per-field SetReq handlers only mark the whole buffer dirty
// to avoid allocating a fresh concatenation on every click — under stress with
// large fields (e.g. a 5 MB image), eager rebuild produces enough garbage to
// outpace TinyGo's GC and trip the wasm `unreachable` trap.
func _ensureWholeFresh() {
	if _wholeDirty {
		_rebuildWhole()
	}
}

// _captureAllFields walks detail once and slices it into _fields[] entries.
// detail must remain live for the lifetime of those slices — store it in
// _lastWholeEncoded so subsequent broadcasts/pings reuse the same buffer.
func _captureAllFields(detail []byte) {
	d := &Decoder{Buf: detail}
{{range .Fields}}	_fields["{{.FieldName}}"] = _capture{{.FieldName}}(d)
{{end}}}

func _registerListeners() {
	ListenCtxSetReq("{{.KeyName}}", func(detail string) {
		// Single copy of the incoming payload — _fields[] slices point into it,
		// and the same buffer is broadcast back as the online ack (no re-encode).
		b := []byte(detail)
		_lastWholeEncoded = b
		_captureAllFields(b)
		_wholeDirty = false
		BroadcastCtxOnline("{{.KeyName}}", string(b))
	})
{{range .Fields}}	ListenCtxSetReqField("{{$.KeyName}}", "{{.FieldName}}", func(detail string) {
		b := []byte(detail)
		_fields["{{.FieldName}}"] = b
		BroadcastCtxEncodedField("{{$.KeyName}}", "{{.FieldName}}", string(b))
		// Defer the whole-struct rebuild until something actually reads it.
		_wholeDirty = true
	})
{{end}}}

func _broadcastOnline() {
	_ensureWholeFresh()
	BroadcastCtxOnline("{{.KeyName}}", string(_lastWholeEncoded))
}

func main() {
	if stored, ok := ReadCtxStore("{{.KeyName}}"); ok {
		b := []byte(stored)
		_lastWholeEncoded = b
		_captureAllFields(b)
	} else {
		// Build the zero-value encoded form once, then slice it into _fields.
		var v {{.StructName}}
		e := NewEncoder(64)
		_encode_{{.StructName}}(v, e)
		_lastWholeEncoded = e.Buf
		_captureAllFields(_lastWholeEncoded)
	}
	_registerListeners()
	ListenCtxPing("{{.KeyName}}", func() { _broadcastOnline() })
	_broadcastOnline()
	select {}
}
