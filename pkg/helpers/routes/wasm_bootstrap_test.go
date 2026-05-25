package helpers

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
)

// TestInjectGothicScope_FullPage verifies a full-page HTML doc is stamped on its
// <body> tag with both data-gothic-wasm and data-gothic-inst attributes.
func TestInjectGothicScope_FullPage(t *testing.T) {
	in := []byte(`<html><head></head><body><h1>hi</h1></body></html>`)
	out, inst := injectGothicScope(in, "components-pingmirror")

	if inst == "" {
		t.Fatal("expected a non-empty instance id")
	}
	if !bytes.Contains(out, []byte(`<body data-gothic-wasm="components-pingmirror" data-gothic-inst="`+inst+`">`)) {
		t.Errorf("expected <body> to carry the gothic-wasm + gothic-inst attributes, got: %s", out)
	}
	// The original document tags must still be present and unaltered.
	if !bytes.Contains(out, []byte(`<h1>hi</h1>`)) {
		t.Errorf("body content was lost: %s", out)
	}
	if !bytes.Contains(out, []byte(`</body></html>`)) {
		t.Errorf("closing tags were lost: %s", out)
	}
}

// TestInjectGothicScope_Fragment verifies HTML with no <body> is wrapped in a
// display-contents <div> that carries the instance id.
func TestInjectGothicScope_Fragment(t *testing.T) {
	in := []byte(`<section>hello</section>`)
	out, inst := injectGothicScope(in, "components-pingmirror")

	if inst == "" {
		t.Fatal("expected a non-empty instance id")
	}
	expectedOpen := `<div data-gothic-wasm="components-pingmirror" data-gothic-inst="` + inst + `" style="display:contents">`
	if !bytes.HasPrefix(out, []byte(expectedOpen)) {
		t.Errorf("expected output to open with %q, got %s", expectedOpen, out)
	}
	if !bytes.HasSuffix(out, []byte(`</div>`)) {
		t.Errorf("expected output to close with </div>, got %s", out)
	}
	if !bytes.Contains(out, []byte(`<section>hello</section>`)) {
		t.Errorf("inner content was lost: %s", out)
	}
}

// TestInjectGothicScope_UniqueInstancePerCall guards the duplicate-component
// contract: two calls with the SAME wasmName must produce different
// data-gothic-inst values so the JS selector can disambiguate duplicate
// components on the same page.
func TestInjectGothicScope_UniqueInstancePerCall(t *testing.T) {
	in := []byte(`<section>x</section>`)
	collisions := 0
	const n = 50
	seen := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		_, inst := injectGothicScope(in, "components-pingmirror")
		if _, dup := seen[inst]; dup {
			collisions++
		}
		seen[inst] = struct{}{}
	}
	// A handful of collisions in a 32-bit random space is statistically
	// unexpected at n=50 but not impossible. Anything above 2 is a real bug.
	if collisions > 2 {
		t.Errorf("expected near-zero instance id collisions across %d calls, got %d", n, collisions)
	}
	// And we must have at least two distinct ids — anything else means the rng
	// is degenerate.
	if len(seen) < 2 {
		t.Errorf("expected at least 2 distinct instance ids, got %d", len(seen))
	}
}

// TestInjectWasmBootstrap_FullPage verifies the bootstrap is spliced before
// </body>, the wasm filename uses the correct extension per compression, and
// the JS selector targets the body that carries the wasm attribute.
func TestInjectWasmBootstrap_FullPage_Gzip(t *testing.T) {
	in := []byte(`<html><body data-gothic-wasm="counter" data-gothic-inst="abc">x</body></html>`)
	out := injectWasmBootstrap(in, "counter", GZIP, GothicTinyGo, "abc")

	if !bytes.Contains(out, []byte(`</body></html>`)) {
		t.Errorf("expected the doc to keep its closing tags: %s", out)
	}
	if !bytes.Contains(out, []byte(`</script></body>`)) {
		t.Errorf("expected the <script> block to sit immediately before </body>: %s", out)
	}
	if !bytes.Contains(out, []byte(`document.querySelector('body[data-gothic-wasm="counter"]')`)) {
		t.Errorf("expected the JS selector for the full-page wasm anchor: %s", out)
	}
	if !bytes.Contains(out, []byte(`'/public/wasm/'+wn+'.wasm.gz'`)) {
		t.Errorf("expected gzip extension in the WASM fetch URL: %s", out)
	}
	if !bytes.Contains(out, []byte(`window.__gothic_set`)) {
		t.Errorf("expected __gothic_set registration in bootstrap so dispatchDirect works: %s", out)
	}
}

func TestInjectWasmBootstrap_FullPage_Brotli(t *testing.T) {
	in := []byte(`<html><body>x</body></html>`)
	out := injectWasmBootstrap(in, "counter", BROTLI, GothicTinyGo, "abc")
	if !bytes.Contains(out, []byte(`'/public/wasm/'+wn+'.wasm.br'`)) {
		t.Errorf("expected brotli extension in the WASM fetch URL: %s", out)
	}
}

// TestInjectWasmBootstrap_Fragment verifies fragments get the bootstrap
// appended (no </body> replacement) and the findEl expression carries the
// data-gothic-inst selector so duplicate-component pages resolve correctly.
func TestInjectWasmBootstrap_Fragment(t *testing.T) {
	in := []byte(`<div data-gothic-wasm="components-pingmirror" data-gothic-inst="deadbeef" style="display:contents">x</div>`)
	out := injectWasmBootstrap(in, "components-pingmirror", GZIP, GothicTinyGo, "deadbeef")

	if !bytes.HasSuffix(out, []byte(`</script>`)) {
		t.Errorf("expected fragment output to end with </script>, got: %s", out)
	}
	// The findEl expression must reference both the wasm name AND the per-instance id.
	if !bytes.Contains(out, []byte(`document.querySelector('[data-gothic-wasm="components-pingmirror"][data-gothic-inst="deadbeef"]')`)) {
		t.Errorf("expected the fragment selector to scope by data-gothic-inst, got: %s", out)
	}
	// And the previousElementSibling fast path must still be there.
	if !bytes.Contains(out, []byte(`document.currentScript&&document.currentScript.previousElementSibling`)) {
		t.Errorf("expected previousElementSibling fast path in fragment bootstrap, got: %s", out)
	}
}

// TestInjectWasmEnvelope_EndToEnd is the high-level integration test for the
// helper that callers actually use. It must thread the SAME instance id from
// the wrapper through to the JS selector.
func TestInjectWasmEnvelope_EndToEnd_Fragment(t *testing.T) {
	in := []byte(`<section>hi</section>`)
	out := injectWasmEnvelope(in, "components-pingmirror", GZIP, GothicTinyGo)

	// Extract the instance id from the wrapper and from the bootstrap script;
	// they MUST be the same value, otherwise duplicate components on the same
	// page will end up sharing state again.
	re := regexp.MustCompile(`data-gothic-inst="([0-9a-f]+)"`)
	matches := re.FindAllSubmatch(out, -1)
	if len(matches) < 2 {
		t.Fatalf("expected at least two data-gothic-inst occurrences (wrapper + selector), got %d in: %s", len(matches), out)
	}
	first := string(matches[0][1])
	for i, m := range matches {
		if string(m[1]) != first {
			t.Fatalf("instance id mismatch between wrapper and bootstrap (match %d = %q, expected %q): %s", i, string(m[1]), first, out)
		}
	}
}

// TestInjectWasmBootstrap_CtxSetPersistentBuffer verifies the __gothic_topic.set
// implementation uses a persistent per-key Uint8Array backed by a doubling
// ArrayBuffer, instead of allocating a fresh Uint8Array via .slice() per
// broadcast. The TinyGo wasm_exec bridge only finalizes string refs, so every
// per-broadcast Uint8Array leaks into the _values[] slot table — keeping a
// persistent buffer means each key occupies exactly one slot for its lifetime.
func TestInjectWasmBootstrap_CtxSetPersistentBuffer(t *testing.T) {
	in := []byte(`<html><body>x</body></html>`)
	out := injectWasmBootstrap(in, "counter", GZIP, GothicTinyGo, "abc")

	wants := [][]byte{
		[]byte(`var _bufs={};`),
		[]byte(`var _views={};`),
		[]byte(`byteLen<128?128:byteLen*2`),
		[]byte(`view.set(src)`),
		[]byte(`_state[keyName]=view`),
	}
	for _, w := range wants {
		if !bytes.Contains(out, w) {
			t.Errorf("expected bootstrap to contain %q, got: %s", w, out)
		}
	}
}

// TestInjectWasmBootstrap_CtxSetNoSlice is a regression guard: the old
// implementation allocated a fresh Uint8Array per broadcast via .slice(),
// which leaked through TinyGo's wasm_exec _values[] slot table on every
// context update. If this substring ever reappears, the leak is back.
func TestInjectWasmBootstrap_CtxSetNoSlice(t *testing.T) {
	in := []byte(`<html><body>x</body></html>`)
	out := injectWasmBootstrap(in, "counter", GZIP, GothicTinyGo, "abc")

	bad := []byte(`new Uint8Array(inst.exports.memory.buffer,offset,byteLen).slice()`)
	if bytes.Contains(out, bad) {
		t.Errorf("bootstrap must not allocate a fresh Uint8Array per set() via .slice(); found %q in: %s", bad, out)
	}
}

// TestInjectWasmBootstrap_FindScopeHelperInjected verifies the bootstrap
// installs window.__gothicFindScope, which moves the per-click DOM walk out
// of Go and into JS. The Go side calls this helper and reads back a string,
// which is the only js.Value kind TinyGo's wasm_exec finalizes — boxing a
// MouseEvent / Element through js.Value would leak a _values[] slot per
// click because those refs are never released.
func TestInjectWasmBootstrap_FindScopeHelperInjected(t *testing.T) {
	in := []byte(`<html><body>x</body></html>`)
	out := injectWasmBootstrap(in, "counter", GZIP, GothicTinyGo, "abc")

	wants := [][]byte{
		[]byte(`window.__gothicFindScope=function()`),
		[]byte(`e.target.closest('[data-gothic-scope]')`),
		[]byte(`el.dataset.gothicScope`),
	}
	for _, w := range wants {
		if !bytes.Contains(out, w) {
			t.Errorf("expected bootstrap to contain %q, got: %s", w, out)
		}
	}
}

// TestInjectWasmBootstrap_FindScopeOnlyDeclaredOnce guards the per-page-once
// install semantics: the helper is declared inside the if(!window.__gothic_topic)
// guard and gated by its own if(!window.__gothicFindScope) check, so multiple
// WASM modules on the same page must not redeclare it.
func TestInjectWasmBootstrap_FindScopeOnlyDeclaredOnce(t *testing.T) {
	in := []byte(`<html><body>x</body></html>`)
	out := injectWasmBootstrap(in, "counter", GZIP, GothicTinyGo, "abc")

	needle := []byte(`if(!window.__gothicFindScope)`)
	if got := bytes.Count(out, needle); got != 1 {
		t.Errorf("expected %q to appear exactly once in bootstrap, got %d in: %s", needle, got, out)
	}
}

// TestInjectWasmEnvelope_UniqueAcrossCalls confirms the duplicate-component
// regression contract: two renders of the same component produce envelopes
// with distinct instance ids embedded BOTH on the wrapper AND inside the JS
// selector.
func TestInjectWasmEnvelope_UniqueAcrossCalls(t *testing.T) {
	in := []byte(`<section>hi</section>`)
	a := injectWasmEnvelope(in, "components-pingmirror", GZIP, GothicTinyGo)
	b := injectWasmEnvelope(in, "components-pingmirror", GZIP, GothicTinyGo)

	re := regexp.MustCompile(`data-gothic-inst="([0-9a-f]+)"`)
	matchA := re.FindSubmatch(a)
	matchB := re.FindSubmatch(b)
	if matchA == nil || matchB == nil {
		t.Fatalf("missing data-gothic-inst in one of the envelopes\nA:%s\nB:%s", a, b)
	}
	if string(matchA[1]) == string(matchB[1]) {
		t.Errorf("expected different instance ids across calls, got %q for both", matchA[1])
	}
	// And the bootstrap selectors should each carry their own instance id —
	// not the other render's id.
	if !strings.Contains(string(a), `data-gothic-inst="`+string(matchA[1])+`"`) {
		t.Errorf("envelope A's selector does not carry envelope A's instance id: %s", a)
	}
	if !strings.Contains(string(b), `data-gothic-inst="`+string(matchB[1])+`"`) {
		t.Errorf("envelope B's selector does not carry envelope B's instance id: %s", b)
	}
}
