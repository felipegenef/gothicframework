package wasm_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	wasm "github.com/felipegenef/gothicframework/v2/pkg/wasm"
)

// TestServerStubs_NoOpsDoNotPanic calls every server-side stub to confirm they
// are inert (no panics) and return their documented zero values. On the server
// these are all no-ops; the real behavior lives in the WASM runtime.
func TestServerStubs_NoOpsDoNotPanic(t *testing.T) {
	// Observable lifecycle.
	o := wasm.CreateObservable(7)
	if o.Get() != 7 {
		t.Errorf("Observable.Get: got %d, want 7", o.Get())
	}
	o.Set(9)
	if o.Get() != 9 {
		t.Errorf("Observable.Set/Get: got %d, want 9", o.Get())
	}

	sub := wasm.Observe(func() {}, o)
	sub.Stop()
	wasm.ObserveWithCleanup(func() func() { return func() {} }, o).Stop()

	// DOM string helpers with documented zero returns.
	if wasm.GetValue("id") != "" {
		t.Error("GetValue stub should return empty string")
	}
	if wasm.GetFileBytes("id") != nil {
		t.Error("GetFileBytes stub should return nil")
	}

	// Fetch family.
	if s, err := wasm.Fetch("http://x", wasm.FetchConfig{Method: "GET"}); s != "" || err != nil {
		t.Errorf("Fetch stub: got (%q,%v)", s, err)
	}
	if b, err := wasm.FetchBytes("http://x"); b != nil || err != nil {
		t.Errorf("FetchBytes stub: got (%v,%v)", b, err)
	}

	// JSValue surface — assert documented zero values.
	v := wasm.JS()
	if v.String() != "" || v.Int() != 0 || v.Float() != 0 || v.Bool() {
		t.Error("JSValue scalar getters should be zero")
	}
	if !v.IsNull() || !v.IsUndefined() || v.Truthy() {
		t.Error("JSValue null/undefined/truthy stubs unexpected")
	}
	if v.Length() != 0 {
		t.Error("JSValue.Length stub should be 0")
	}
	if wasm.CopyBytesToJS(v, []byte{1}) != 0 || wasm.CopyBytesToGo([]byte{0}, v) != 0 {
		t.Error("CopyBytes stubs should return 0")
	}

	// Storage + cookie helpers with documented zero returns.
	if wasm.LocalStorageGet("k") != "" {
		t.Error("LocalStorageGet stub should be empty")
	}
	if wasm.SessionStorageGet("k") != "" {
		t.Error("SessionStorageGet stub should be empty")
	}
	if wasm.CookieGet("k") != "" {
		t.Error("CookieGet stub should be empty")
	}
}

func TestObservableFieldAndSharedTopic(t *testing.T) {
	f := wasm.NewObservableField("init")
	if f.Get() != "init" || f.Peek() != "init" {
		t.Errorf("ObservableField initial: got Get=%q Peek=%q", f.Get(), f.Peek())
	}
	f.SetBroadcast(func() {})
	f.Set("next")
	if f.Get() != "next" {
		t.Errorf("ObservableField.Set: got %q", f.Get())
	}
	f.ApplyExternal("ext")
	if f.Get() != "ext" {
		t.Errorf("ObservableField.ApplyExternal: got %q", f.Get())
	}

	st := &wasm.SharedTopicObservable[int]{}
	st.Set(42)
	if st.Get() != 42 {
		t.Errorf("SharedTopicObservable: got %d", st.Get())
	}
}

// TestTopicKeyRoundTrip exercises BinaryKey (which goes through hexEncode and
// hexDecode internally) plus AutoKey and CreateTopic stubs.
func TestTopicKeyRoundTrip(t *testing.T) {
	key := wasm.BinaryKey[int]("count",
		func(v int, e *wasm.Encoder) { e.I64(int64(v)) },
		func(d *wasm.Decoder) int { return int(d.I64()) },
	)
	if key.Name != "count" {
		t.Errorf("BinaryKey.Name: got %q", key.Name)
	}

	auto := wasm.AutoKey[string]("label")
	if auto.Name != "label" {
		t.Errorf("AutoKey.Name: got %q", auto.Name)
	}

	ctor := wasm.CreateTopic(0, wasm.TopicConfig{Name: "t", Compression: wasm.BROTLI, Compiler: wasm.Golang})
	if ctor() != nil {
		t.Error("CreateTopic stub ctor should return nil")
	}
}

// TestDecoderUnderflow drives the buffer-underflow path through every reader so
// the need() error branch and decErr.Error are covered.
func TestDecoderUnderflow(t *testing.T) {
	d := &wasm.Decoder{Buf: nil}
	if d.U8() != 0 || d.U16() != 0 || d.U32() != 0 || d.U64() != 0 {
		t.Error("readers on empty buffer should return 0")
	}
	if d.Bool() {
		t.Error("Bool on empty buffer should be false")
	}
	if d.Bytes() != nil {
		t.Error("Bytes on empty buffer should be nil")
	}
	if d.String() != "" {
		t.Error("String on empty buffer should be empty")
	}
	if d.Err == nil {
		t.Error("expected Decoder.Err set after underflow")
	}
	if d.Err.Error() == "" {
		t.Error("decErr.Error should be non-empty")
	}
	// Once Err is set, need() short-circuits → further reads stay zero.
	if d.U32() != 0 {
		t.Error("reads after error should stay 0")
	}
}

// TestDecoderFloatsAndInts covers the remaining typed read wrappers.
func TestDecoderFloatsAndInts(t *testing.T) {
	e := wasm.NewEncoder(64)
	e.I32(-5)
	e.I64(-9)
	e.F32(1.5)
	e.F64(2.5)
	d := &wasm.Decoder{Buf: e.Buf}
	if d.I32() != -5 {
		t.Errorf("I32 round-trip")
	}
	if d.I64() != -9 {
		t.Errorf("I64 round-trip")
	}
	if d.F32() != 1.5 {
		t.Errorf("F32 round-trip")
	}
	if d.F64() != 2.5 {
		t.Errorf("F64 round-trip")
	}
}

// ---------------------------------------------------------------------------
// embed.go — ExtractRuntime
// ---------------------------------------------------------------------------

func TestExtractRuntime(t *testing.T) {
	dest := t.TempDir()
	if err := wasm.ExtractRuntime(dest); err != nil {
		t.Fatalf("ExtractRuntime: %v", err)
	}
	// go.mod must be written.
	gomod, err := os.ReadFile(filepath.Join(dest, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	if !strings.Contains(string(gomod), "module wasm-runtime") {
		t.Errorf("go.mod missing module decl:\n%s", gomod)
	}
	// At least one runtime .go file must be extracted under runtime/.
	var count int
	_ = filepath.WalkDir(filepath.Join(dest, "runtime"), func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(path, ".go") {
			count++
		}
		return nil
	})
	if count == 0 {
		t.Error("expected runtime .go files to be extracted")
	}
}

func TestExtractRuntime_BadDest(t *testing.T) {
	// Destination under a path whose parent is a file → MkdirAll/WriteFile fails.
	file := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(file, []byte("x"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := wasm.ExtractRuntime(filepath.Join(file, "sub")); err == nil {
		t.Error("expected error extracting into a path under a regular file")
	}
}
