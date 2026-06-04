package helpers

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	templcache "github.com/felipegenef/gothicframework/v2/pkg/helpers/templ"
)

// seedTemplCache writes a .gothicCli/templ-cache.json under workDir mapping
// relPath to the current content hash of srcFile, so DirtyFiles treats it as up
// to date and Render skips the real templ generator.
func seedTemplCache(t *testing.T, workDir, relPath, srcFile string) {
	t.Helper()
	hash := templcache.HashFile(srcFile)
	if hash == "" {
		t.Fatalf("could not hash %s", srcFile)
	}
	cacheDir := filepath.Join(workDir, ".gothicCli")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(map[string]string{relPath: hash})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "templ-cache.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
}

func TestNewTemplHelper(t *testing.T) {
	h := NewTemplHelper()
	_ = h
}

// setGenerate swaps the package-level templ generate seam for the duration of a
// test and returns a restore func. Mirrors setRunner for the runner seam.
func setGenerate(fn func(args []string) error) func() {
	prev := generate
	generate = fn
	return func() { generate = prev }
}

// TestTemplRenderDirtyRegenerates: a .templ file with no cache entry is dirty,
// so Render must invoke the per-file generator (with -f <file>), succeed, and
// then persist the cache so the file is no longer dirty on a subsequent scan.
func TestTemplRenderDirtyRegenerates(t *testing.T) {
	dir := t.TempDir()
	withWorkDir(t, dir)

	templFile := filepath.Join(dir, "page.templ")
	if err := os.WriteFile(templFile, []byte("templ page() {}"), 0644); err != nil {
		t.Fatal(err)
	}

	var calls [][]string
	restore := setGenerate(func(args []string) error {
		calls = append(calls, args)
		// Emulate the real generator producing the _templ.go counterpart, so
		// the post-Render rescan sees the file as clean (DirtyFiles treats a
		// missing counterpart as dirty regardless of cache state).
		if err := os.WriteFile(filepath.Join(dir, "page_templ.go"), []byte("package x"), 0644); err != nil {
			return err
		}
		return nil
	})
	defer restore()

	h := NewTemplHelper()
	if err := h.Render(); err != nil {
		t.Fatalf("Render() unexpected error: %v", err)
	}

	// Exactly one per-file generation for the single dirty file, no fallback.
	if len(calls) != 1 {
		t.Fatalf("expected 1 generate call, got %d: %v", len(calls), calls)
	}
	if got := calls[0]; len(got) != 3 || got[0] != "generate" || got[1] != "-f" || got[2] != "page.templ" {
		t.Errorf("expected [generate -f page.templ], got %v", got)
	}

	// Cache must now be persisted with the file marked clean.
	cachePath := filepath.Join(dir, ".gothicCli", "templ-cache.json")
	if _, err := os.Stat(cachePath); err != nil {
		t.Fatalf("expected cache to be written at %s: %v", cachePath, err)
	}
	cache := templcache.Load()
	files, err := templcache.ScanTemplFiles(".")
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}
	if dirty := templcache.DirtyFiles(cache, files); len(dirty) != 0 {
		t.Errorf("expected no dirty files after Render persisted cache, got %v", dirty)
	}
}

// TestTemplRenderPerFileFailsFallback: when the per-file generator fails on the
// first dirty file, Render must fall back to a full-project generation. Here the
// fallback succeeds, so Render returns nil and the cache is still refreshed.
func TestTemplRenderPerFileFailsFallback(t *testing.T) {
	dir := t.TempDir()
	withWorkDir(t, dir)

	if err := os.WriteFile(filepath.Join(dir, "page.templ"), []byte("templ page() {}"), 0644); err != nil {
		t.Fatal(err)
	}

	var calls [][]string
	restore := setGenerate(func(args []string) error {
		calls = append(calls, args)
		// Fail the per-file invocation (has -f), succeed the full fallback.
		for _, a := range args {
			if a == "-f" {
				return errors.New("per-file unsupported")
			}
		}
		return nil
	})
	defer restore()

	h := NewTemplHelper()
	if err := h.Render(); err != nil {
		t.Fatalf("Render() should recover via fallback, got: %v", err)
	}

	// First call is the failing per-file run, second is the full fallback.
	if len(calls) != 2 {
		t.Fatalf("expected per-file then fallback (2 calls), got %d: %v", len(calls), calls)
	}
	if calls[1][len(calls[1])-1] == "-f" || len(calls[1]) != 1 || calls[1][0] != "generate" {
		t.Errorf("expected fallback call [generate], got %v", calls[1])
	}
}

// TestTemplRenderFallbackError: per-file fails AND the full fallback also fails,
// so Render must surface the fallback error (wrapped with the per-file cause).
func TestTemplRenderFallbackError(t *testing.T) {
	dir := t.TempDir()
	withWorkDir(t, dir)

	if err := os.WriteFile(filepath.Join(dir, "page.templ"), []byte("templ page() {}"), 0644); err != nil {
		t.Fatal(err)
	}

	fallbackErr := errors.New("full generate boom")
	restore := setGenerate(func(args []string) error {
		for _, a := range args {
			if a == "-f" {
				return errors.New("per-file unsupported")
			}
		}
		return fallbackErr
	})
	defer restore()

	h := NewTemplHelper()
	err := h.Render()
	if err == nil {
		t.Fatal("expected error when fallback generation fails, got nil")
	}
	if !errors.Is(err, fallbackErr) {
		t.Errorf("expected wrapped fallback error, got %v", err)
	}
}

// TestTemplRenderNoFiles exercises the fast path: a directory with no .templ
// files scans clean, produces an empty dirty set, and returns without ever
// invoking the real templ generator.
func TestTemplRenderNoFiles(t *testing.T) {
	dir := t.TempDir()
	withWorkDir(t, dir)

	h := NewTemplHelper()
	if err := h.Render(); err != nil {
		t.Fatalf("Render() unexpected error: %v", err)
	}
}

// TestGeneratePerFileEmpty verifies the per-file generator is a no-op for an
// empty dirty list (never touches the templ binary).
func TestGeneratePerFileEmpty(t *testing.T) {
	if err := generatePerFile(nil); err != nil {
		t.Fatalf("generatePerFile(nil) = %v, want nil", err)
	}
	if err := generatePerFile([]string{}); err != nil {
		t.Fatalf("generatePerFile([]) = %v, want nil", err)
	}
}

// TestTemplRenderAllCached: when every .templ file already has a matching
// _templ.go counterpart and a fresh cache entry, Render should skip templ.
func TestTemplRenderAllCached(t *testing.T) {
	dir := t.TempDir()
	withWorkDir(t, dir)

	// Create a .templ file and its generated counterpart so DirtyFiles sees it
	// as up to date once we seed the cache.
	templFile := filepath.Join(dir, "page.templ")
	if err := os.WriteFile(templFile, []byte("templ page() {}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "page_templ.go"), []byte("package x"), 0644); err != nil {
		t.Fatal(err)
	}

	// Seed the cache with the current hash so the file is not dirty.
	// We rely on Render's first scan producing the file as dirty only if the
	// cache is empty; to keep this test from invoking real templ we pre-write
	// the cache file with the matching hash.
	// Compute hash via the same mechanism Render uses.
	// (Import-free: read+sha is internal to templ pkg; instead we accept that an
	// empty cache marks it dirty. To avoid templ, write the cache entry.)
	// Simplest: write cache JSON mapping the relative path to its hash.
	seedTemplCache(t, dir, "page.templ", templFile)

	h := NewTemplHelper()
	if err := h.Render(); err != nil {
		t.Fatalf("Render() unexpected error: %v", err)
	}
}
