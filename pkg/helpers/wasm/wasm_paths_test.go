package helpers

import "testing"

func TestWasmOutputName_RootIsIndex(t *testing.T) {
	h := DefaultWasmHelper()
	if got := h.wasmOutputName("/"); got != "index" {
		t.Errorf("wasmOutputName(/): got %q, want %q", got, "index")
	}
	if got := h.wasmOutputName(""); got != "index" {
		t.Errorf("wasmOutputName(empty): got %q, want %q", got, "index")
	}
}

func TestWasmOutputName_SimplePath(t *testing.T) {
	h := DefaultWasmHelper()
	if got := h.wasmOutputName("/counter"); got != "counter" {
		t.Errorf("wasmOutputName(/counter): got %q, want %q", got, "counter")
	}
}

func TestWasmOutputName_NestedPath(t *testing.T) {
	h := DefaultWasmHelper()
	if got := h.wasmOutputName("/blog/post/comments"); got != "blog-post-comments" {
		t.Errorf("wasmOutputName nested: got %q, want %q", got, "blog-post-comments")
	}
}

func TestWasmOutputName_ParamSegment(t *testing.T) {
	h := DefaultWasmHelper()
	// "/blog/{slug}" → "blog-slug"
	if got := h.wasmOutputName("/blog/{slug}"); got != "blog-slug" {
		t.Errorf("wasmOutputName param: got %q, want %q", got, "blog-slug")
	}
}

func TestWasmOutputName_NoLeadingSlash(t *testing.T) {
	h := DefaultWasmHelper()
	if got := h.wasmOutputName("foo/bar"); got != "foo-bar" {
		t.Errorf("wasmOutputName no leading slash: got %q, want %q", got, "foo-bar")
	}
}
