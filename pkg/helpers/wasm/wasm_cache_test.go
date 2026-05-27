package helpers

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

// writePageFixture creates a minimal page.go in dir and returns its absolute path.
func writePageFixture(t *testing.T, dir string) string {
	t.Helper()
	src := filepath.Join(dir, "page.go")
	if err := os.WriteFile(src, []byte("package fixtures\nvar Page = 1\n"), 0644); err != nil {
		t.Fatalf("write page.go: %v", err)
	}
	return src
}

// handwrittenHash returns a hex SHA-256 over feedHandwrittenPackageFiles output.
// Isolating just that helper keeps these tests focused and independent of
// runtime/topic/template inputs which require cwd setup.
func handwrittenHash(t *testing.T, dir, exclude string) string {
	t.Helper()
	h := DefaultWasmHelper()
	var buf bytes.Buffer
	h.feedHandwrittenPackageFiles(&buf, dir, exclude)
	sum := sha256.Sum256(buf.Bytes())
	return hex.EncodeToString(sum[:])
}

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

// TestFeedHandwrittenPackageFiles_StateGoInvalidates ensures that adding a
// hand-written sibling state.go to the page's directory changes the hash.
// This is the core stale-WASM-cache bugfix.
func TestFeedHandwrittenPackageFiles_StateGoInvalidates(t *testing.T) {
	dir := t.TempDir()
	src := writePageFixture(t, dir)

	hBefore := handwrittenHash(t, dir, src)

	statePath := filepath.Join(dir, "state.go")
	if err := os.WriteFile(statePath, []byte("package fixtures\nvar S = 1\n"), 0644); err != nil {
		t.Fatalf("write state.go: %v", err)
	}
	hAfter := handwrittenHash(t, dir, src)

	if hBefore == hAfter {
		t.Errorf("expected hash to change when state.go is added; both were %q", hBefore)
	}
}

// TestFeedHandwrittenPackageFiles_ModifyingStateChangesHash ensures that
// editing the contents of a sibling hand-written file invalidates the hash.
func TestFeedHandwrittenPackageFiles_ModifyingStateChangesHash(t *testing.T) {
	dir := t.TempDir()
	src := writePageFixture(t, dir)
	statePath := filepath.Join(dir, "state.go")
	if err := os.WriteFile(statePath, []byte("package fixtures\nvar S = 1\n"), 0644); err != nil {
		t.Fatalf("write state.go: %v", err)
	}
	hBefore := handwrittenHash(t, dir, src)

	if err := os.WriteFile(statePath, []byte("package fixtures\nvar S = 2\n"), 0644); err != nil {
		t.Fatalf("rewrite state.go: %v", err)
	}
	hAfter := handwrittenHash(t, dir, src)

	if hBefore == hAfter {
		t.Errorf("expected hash to change when state.go content changes; both were %q", hBefore)
	}
}

// TestFeedHandwrittenPackageFiles_GenFilesExcluded ensures that *_gen.go
// (generated) sibling files do NOT contribute to the hash.
func TestFeedHandwrittenPackageFiles_GenFilesExcluded(t *testing.T) {
	dir := t.TempDir()
	src := writePageFixture(t, dir)
	hBefore := handwrittenHash(t, dir, src)

	genPath := filepath.Join(dir, "other_gen.go")
	if err := os.WriteFile(genPath, []byte("package fixtures\nvar G = 1\n"), 0644); err != nil {
		t.Fatalf("write other_gen.go: %v", err)
	}
	// Also include _templ.go since it is in the same exclusion family.
	templPath := filepath.Join(dir, "page_templ.go")
	if err := os.WriteFile(templPath, []byte("package fixtures\nvar T = 1\n"), 0644); err != nil {
		t.Fatalf("write page_templ.go: %v", err)
	}
	hAfter := handwrittenHash(t, dir, src)

	if hBefore != hAfter {
		t.Errorf("expected hash to be unchanged when generated files are added; got %q vs %q", hBefore, hAfter)
	}
}

// TestFeedHandwrittenPackageFiles_TestFilesExcluded ensures that *_test.go
// sibling files do NOT contribute to the hash.
func TestFeedHandwrittenPackageFiles_TestFilesExcluded(t *testing.T) {
	dir := t.TempDir()
	src := writePageFixture(t, dir)
	hBefore := handwrittenHash(t, dir, src)

	testPath := filepath.Join(dir, "helper_test.go")
	if err := os.WriteFile(testPath, []byte("package fixtures\nvar X = 1\n"), 0644); err != nil {
		t.Fatalf("write helper_test.go: %v", err)
	}
	hAfter := handwrittenHash(t, dir, src)

	if hBefore != hAfter {
		t.Errorf("expected hash to be unchanged when test files are added; got %q vs %q", hBefore, hAfter)
	}
}

// TestFeedHandwrittenPackageFiles_OrderingDeterministic ensures that two
// directories holding the same hand-written sibling files produce the same
// hash regardless of the order in which the files happened to be written
// (which can influence directory iteration order on some filesystems). The
// alphabetical sort inside feedHandwrittenPackageFiles guarantees this.
func TestFeedHandwrittenPackageFiles_OrderingDeterministic(t *testing.T) {
	contents := map[string][]byte{
		"a_state.go":  []byte("package fixtures\nvar A = 1\n"),
		"m_middle.go": []byte("package fixtures\nvar M = 1\n"),
		"z_last.go":   []byte("package fixtures\nvar Z = 1\n"),
	}

	// Dir 1: write a, m, z.
	dir1 := t.TempDir()
	src1 := writePageFixture(t, dir1)
	for _, name := range []string{"a_state.go", "m_middle.go", "z_last.go"} {
		if err := os.WriteFile(filepath.Join(dir1, name), contents[name], 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	// Dir 2: write z, m, a (reverse order).
	dir2 := t.TempDir()
	src2 := writePageFixture(t, dir2)
	for _, name := range []string{"z_last.go", "m_middle.go", "a_state.go"} {
		if err := os.WriteFile(filepath.Join(dir2, name), contents[name], 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	h1 := handwrittenHash(t, dir1, src1)
	h2 := handwrittenHash(t, dir2, src2)
	if h1 != h2 {
		t.Errorf("expected identical hashes regardless of write order; got %q vs %q", h1, h2)
	}
}
