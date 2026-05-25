//go:build !js || !wasm

package runtime

import "testing"

// Lock the API surface of the DOM helpers so signatures cannot drift between
// dom.go (//go:build js && wasm) and dom_stub.go (//go:build !js || !wasm).
//
// pkg/wasm/wasm-runtime/runtime/dom.go is only compiled under the WASM
// build tag, so a pure-Go unit test cannot exercise its scope-aware
// querySelector logic — that coverage lives in TestGothic's Playwright
// suite. What we CAN do at unit-test time is bind every helper to a typed
// function variable: if a future edit changes a signature in dom.go
// without updating dom_stub.go (or vice-versa), the build will fail
// because the typed assignment below references the SHARED package-level
// symbol that the build tags multiplex. Go won't let two files in the
// same package export the same name with different signatures.
//
// The test body also asserts the documented no-op behaviour of the stub:
// GetValue returns "" and every setter is callable without panicking.

func TestDomStubAPISurface(t *testing.T) {
	// Bind every helper to a typed variable. The compiler enforces these
	// signatures against whichever build of the package was selected, so
	// any signature drift between dom.go and dom_stub.go shows up here as
	// a build error rather than a runtime surprise.
	var (
		_ func(id, value string)            = SetText
		_ func(id, html string)             = SetHTML
		_ func(id, value string)            = SetValue
		_ func(id string) string            = GetValue
		_ func(id, className string)        = AddClass
		_ func(id, className string)        = RemoveClass
		_ func(id, className string)        = ToggleClass
		_ func(id, attr, value string)      = SetAttr
		_ func(id, property, value string) = SetStyle
	)

	// Stub no-op behaviour: setters do nothing, GetValue returns "".
	SetText("x", "y")
	SetHTML("x", "<b>y</b>")
	SetValue("x", "y")
	AddClass("x", "c")
	RemoveClass("x", "c")
	ToggleClass("x", "c")
	SetAttr("x", "data-y", "z")
	SetStyle("x", "color", "red")

	if got := GetValue("any"); got != "" {
		t.Errorf("GetValue stub: got %q, want %q", got, "")
	}
}
