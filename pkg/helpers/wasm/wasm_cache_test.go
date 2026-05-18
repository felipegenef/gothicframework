package helpers

import (
	"os"
	"path/filepath"
	"testing"
)

// withTempCwd sets cwd to a fresh temporary directory for the duration of the
// test and restores the original working directory on cleanup.
func withTempCwd(t *testing.T) string {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir(%s): %v", dir, err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })
	return dir
}

func TestPageInputHash_SameInputsSameHash(t *testing.T) {
	dir := withTempCwd(t)
	srcPath := filepath.Join(dir, "page.go")
	if err := os.WriteFile(srcPath, []byte("package x\nvar A = 1\n"), 0644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	h := DefaultWasmHelper()
	page := WasmPage{SourceFile: srcPath, Compression: WasmCompressionGzip}

	h1 := h.pageInputHash(page)
	h2 := h.pageInputHash(page)
	if h1 != h2 {
		t.Errorf("expected identical hash for unchanged inputs; got %q vs %q", h1, h2)
	}
	if h1 == "" {
		t.Errorf("hash should not be empty for an existing source file")
	}
}

func TestPageInputHash_DifferentContentChangesHash(t *testing.T) {
	dir := withTempCwd(t)
	srcPath := filepath.Join(dir, "page.go")
	if err := os.WriteFile(srcPath, []byte("package x\nvar A = 1\n"), 0644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	h := DefaultWasmHelper()
	page := WasmPage{SourceFile: srcPath, Compression: WasmCompressionGzip}
	before := h.pageInputHash(page)

	if err := os.WriteFile(srcPath, []byte("package x\nvar A = 2\n"), 0644); err != nil {
		t.Fatalf("rewrite src: %v", err)
	}
	after := h.pageInputHash(page)
	if before == after {
		t.Errorf("expected different hashes after content change; both were %q", before)
	}
}

func TestPageInputHash_DifferentCompressionChangesHash(t *testing.T) {
	dir := withTempCwd(t)
	srcPath := filepath.Join(dir, "page.go")
	if err := os.WriteFile(srcPath, []byte("package x\n"), 0644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	h := DefaultWasmHelper()
	page := WasmPage{SourceFile: srcPath, Compression: WasmCompressionGzip}
	hGz := h.pageInputHash(page)
	page.Compression = WasmCompressionBrotli
	hBr := h.pageInputHash(page)
	if hGz == hBr {
		t.Errorf("expected different hashes for different compression; both were %q", hGz)
	}
}
