package helpers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// closedServerURL returns the URL of a server that has already been shut down,
// so any request to it fails fast with a connection error (no retry delays of
// consequence beyond the test's own tolerance).
func closedServerURL(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()
	return url
}

func TestTryDownload_ConnectionError(t *testing.T) {
	h := linuxAmd64Helper()
	if _, err := h.tryDownload(closedServerURL(t)); err == nil {
		t.Error("expected connection error from closed server")
	}
}

func TestFetchExpectedChecksum_ConnectionError(t *testing.T) {
	h := linuxAmd64Helper()
	if _, err := h.fetchExpectedChecksum(closedServerURL(t), "x.tar.gz"); err == nil {
		t.Error("expected connection error fetching checksum")
	}
}

func TestFetchExpectedChecksum_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "gone", http.StatusGone)
	}))
	defer srv.Close()
	h := linuxAmd64Helper()
	if _, err := h.fetchExpectedChecksum(srv.URL, "x.tar.gz"); err == nil {
		t.Error("expected error for non-200 checksum response")
	}
}

// NOTE: The checksum *parsing* test cases (happy path, comment/blank skipping,
// asterisk-prefix stripping, not-found, CRLF) live in wasm_binary_test.go, which
// is the authoritative home for parsing behavior. This file keeps only the
// HTTP-level cases (connection error, non-200) that exercise the fetching
// wrapper around the parser.

// TestDownloadToTemp_AllRetriesFail drives the retry loop to exhaustion against
// a closed server. The loop sleeps 2s then 4s between the three attempts, so
// this test takes ~6s — acceptable but the only way to cover the retry path
// hermetically.
func TestDownloadToTemp_AllRetriesFail(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow retry test in -short mode")
	}
	h := linuxAmd64Helper()
	if _, err := h.downloadToTemp(closedServerURL(t)); err == nil {
		t.Error("expected downloadToTemp to fail after all retries")
	}
}

// TestEnvironWithWarn_WasmOptPresent exercises the branch where a managed
// binaryen binary exists, so WASMOPT=false is NOT appended.
func TestEnvironWithWarn_WasmOptPresent(t *testing.T) {
	emptyDir := t.TempDir() // ensure system wasm-opt is not picked up
	t.Setenv("PATH", emptyDir)
	tmp := t.TempDir()
	t.Setenv("GOTHIC_CLI_CACHE_DIR", tmp)
	h := linuxAmd64Helper()

	// Place a managed wasm-opt where BinaryenBinary() points.
	bbin := h.BinaryenBinary()
	if err := os.MkdirAll(filepath.Dir(bbin), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(bbin, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatalf("write managed wasm-opt: %v", err)
	}

	var once sync.Once
	env := h.EnvironWithWarn(&once)
	for _, e := range env {
		if e == "WASMOPT=false" {
			t.Error("did not expect WASMOPT=false when managed wasm-opt exists")
		}
	}
}
