package helpers

import "testing"

func TestParseGothicTag_Basic(t *testing.T) {
	h := DefaultWasmHelper()
	if got := h.parseGothicTag(`gothic:"i32"`); got != "i32" {
		t.Errorf("parseGothicTag: got %q, want %q", got, "i32")
	}
	if got := h.parseGothicTag(`json:"x" gothic:"skip"`); got != "skip" {
		t.Errorf("parseGothicTag mixed: got %q, want %q", got, "skip")
	}
	if got := h.parseGothicTag(`json:"x"`); got != "" {
		t.Errorf("parseGothicTag missing: got %q, want empty", got)
	}
}

func TestParseGothicTag_Empty(t *testing.T) {
	h := DefaultWasmHelper()
	if got := h.parseGothicTag(""); got != "" {
		t.Errorf("parseGothicTag empty input: got %q, want empty", got)
	}
}

func TestParseNameTag_Basic(t *testing.T) {
	h := DefaultWasmHelper()
	if got := h.parseNameTag(`name:"page"`); got != "page" {
		t.Errorf("parseNameTag: got %q, want %q", got, "page")
	}
	if got := h.parseNameTag(`json:"x" name:"my-key"`); got != "my-key" {
		t.Errorf("parseNameTag mixed: got %q, want %q", got, "my-key")
	}
	if got := h.parseNameTag(`other:"x"`); got != "" {
		t.Errorf("parseNameTag missing: got %q, want empty", got)
	}
}

func TestParseCompressionTag_DefaultsToGzip(t *testing.T) {
	h := DefaultWasmHelper()
	if got := h.parseCompressionTag(""); got != WasmCompressionGzip {
		t.Errorf("parseCompressionTag empty: got %v, want gzip", got)
	}
	if got := h.parseCompressionTag(`json:"x"`); got != WasmCompressionGzip {
		t.Errorf("parseCompressionTag missing: got %v, want gzip", got)
	}
}

func TestParseCompressionTag_Brotli(t *testing.T) {
	h := DefaultWasmHelper()
	if got := h.parseCompressionTag(`compression:"brotli"`); got != WasmCompressionBrotli {
		t.Errorf("parseCompressionTag brotli: got %v, want brotli", got)
	}
	// Case-insensitive.
	if got := h.parseCompressionTag(`compression:"BROTLI"`); got != WasmCompressionBrotli {
		t.Errorf("parseCompressionTag BROTLI upper: got %v, want brotli", got)
	}
}
