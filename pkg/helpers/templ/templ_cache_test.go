package templ

import (
	"os"
	"path/filepath"
	"testing"
)

// writeFile is a tiny test helper — creates parent dirs and writes content.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

// TestCacheHitNoDirtyFiles verifies that when every .templ file matches its
// cached hash AND has a generated counterpart on disk, DirtyFiles returns
// an empty slice — meaning the caller can skip `templ generate` entirely.
func TestCacheHitNoDirtyFiles(t *testing.T) {
	dir := t.TempDir()
	templPath := filepath.Join(dir, "page.templ")
	genPath := filepath.Join(dir, "page_templ.go")
	writeFile(t, templPath, "templ Hello() { <div>hi</div> }")
	writeFile(t, genPath, "package x\n")

	cachePath := filepath.Join(dir, "cache.json")
	c := LoadAt(cachePath)
	c.Update(templPath, HashFile(templPath))
	if err := c.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Reload from disk to ensure round-trip works.
	c2 := LoadAt(cachePath)
	dirty := DirtyFiles(c2, []string{templPath})
	if len(dirty) != 0 {
		t.Fatalf("expected no dirty files, got %v", dirty)
	}
}

// TestCacheMissDetectsChange verifies that modifying a .templ file's content
// invalidates the cache entry, causing DirtyFiles to surface it for regen.
func TestCacheMissDetectsChange(t *testing.T) {
	dir := t.TempDir()
	templPath := filepath.Join(dir, "page.templ")
	genPath := filepath.Join(dir, "page_templ.go")
	writeFile(t, templPath, "templ Hello() { <div>hi</div> }")
	writeFile(t, genPath, "package x\n")

	cachePath := filepath.Join(dir, "cache.json")
	c := LoadAt(cachePath)
	c.Update(templPath, HashFile(templPath))

	// Mutate the templ source — hash should no longer match.
	writeFile(t, templPath, "templ Hello() { <div>BYE</div> }")

	dirty := DirtyFiles(c, []string{templPath})
	if len(dirty) != 1 || dirty[0] != templPath {
		t.Fatalf("expected [%s] dirty, got %v", templPath, dirty)
	}
}

// TestMissingGeneratedCounterpartForcesRebuild verifies that even when the
// .templ hash matches the cache, a missing _templ.go file still marks the
// entry as dirty so the generator is re-run.
func TestMissingGeneratedCounterpartForcesRebuild(t *testing.T) {
	dir := t.TempDir()
	templPath := filepath.Join(dir, "page.templ")
	writeFile(t, templPath, "templ Hello() { <div>hi</div> }")
	// Note: no _templ.go file is written.

	cachePath := filepath.Join(dir, "cache.json")
	c := LoadAt(cachePath)
	c.Update(templPath, HashFile(templPath))

	dirty := DirtyFiles(c, []string{templPath})
	if len(dirty) != 1 {
		t.Fatalf("expected dirty due to missing _templ.go, got %v", dirty)
	}
}

// TestScanTemplFilesSkipsIgnoredDirs verifies that .gothicCli, .git, and
// node_modules are not walked — these contain generated/vendored content
// we never want to feed to `templ generate`.
func TestScanTemplFilesSkipsIgnoredDirs(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "src", "a.templ"), "x")
	writeFile(t, filepath.Join(dir, ".gothicCli", "skip.templ"), "x")
	writeFile(t, filepath.Join(dir, "node_modules", "skip.templ"), "x")

	files, err := ScanTemplFiles(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file (src/a.templ), got %v", files)
	}
}
