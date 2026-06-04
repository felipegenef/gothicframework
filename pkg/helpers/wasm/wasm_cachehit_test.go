package helpers

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// These tests cover the hermetic *cache-hit early-return* prologues of
// GeneratePage and buildTopicManager. They never reach the toolchain-invoking
// build steps (which stay integration-only); they verify that an up-to-date
// cache entry plus an existing output file short-circuits the build.

func TestGeneratePage_CacheHitSkipsBuild(t *testing.T) {
	dir := withTempCwd(t)
	srcPath := filepath.Join(dir, "page.go")
	if err := os.WriteFile(srcPath, []byte("package x\n"), 0644); err != nil {
		t.Fatalf("write src: %v", err)
	}

	outDir := filepath.Join(dir, "public", "wasm")
	if err := os.MkdirAll(outDir, 0755); err != nil {
		t.Fatalf("mkdir outDir: %v", err)
	}

	h := DefaultWasmHelper()
	h.cache = loadWasmCache()

	page := WasmPage{
		SourceFile:  srcPath,
		OutputName:  "counter",
		Compression: WasmCompressionGzip,
	}

	// Pre-create the output file and seed the cache with the matching hash so
	// the up-to-date branch fires.
	outFile := filepath.Join(outDir, page.OutputName+".wasm.gz")
	if err := os.WriteFile(outFile, []byte("prebuilt"), 0644); err != nil {
		t.Fatalf("write out file: %v", err)
	}
	hash := h.pageInputHash(page)
	h.cache.update(page.OutputName, hash)

	if err := h.GeneratePage(page, outDir, &sync.Once{}); err != nil {
		t.Fatalf("GeneratePage cache-hit: %v", err)
	}
	// The prebuilt file must still be there (not rebuilt).
	if data, _ := os.ReadFile(outFile); string(data) != "prebuilt" {
		t.Errorf("expected prebuilt file untouched on cache hit, got %q", data)
	}
}

// TestGeneratePage_SourceChangeInvalidatesCache proves the dangerous direction:
// when the page's source-file content changes, pageInputHash must differ from
// the cached hash so the build is NOT skipped. A false cache hit here would ship
// a stale WASM that ignores the user's edits.
func TestGeneratePage_SourceChangeInvalidatesCache(t *testing.T) {
	dir := withTempCwd(t)
	srcPath := filepath.Join(dir, "page.go")
	if err := os.WriteFile(srcPath, []byte("package x\n\nvar Counter = 1\n"), 0644); err != nil {
		t.Fatalf("write src: %v", err)
	}

	h := DefaultWasmHelper()
	h.cache = loadWasmCache()

	page := WasmPage{
		SourceFile:  srcPath,
		OutputName:  "counter",
		Compression: WasmCompressionGzip,
	}

	// Seed the cache with the hash of the ORIGINAL source.
	originalHash := h.pageInputHash(page)
	h.cache.update(page.OutputName, originalHash)
	if !h.cache.upToDate(page.OutputName, originalHash) {
		t.Fatalf("sanity: original hash should be up-to-date")
	}

	// Rewrite the source with a meaningful change.
	if err := os.WriteFile(srcPath, []byte("package x\n\nvar Counter = 999\n"), 0644); err != nil {
		t.Fatalf("rewrite src: %v", err)
	}

	newHash := h.pageInputHash(page)
	if newHash == originalHash {
		t.Fatal("expected pageInputHash to change after source content change")
	}
	// The cache entry (still the original hash) must now be considered stale, so
	// the build would proceed rather than short-circuit.
	if h.cache.upToDate(page.OutputName, newHash) {
		t.Error("expected cache to be stale after source change → build must not be skipped")
	}
}

func TestBuildTopicManager_CacheHitSkipsBuild(t *testing.T) {
	setupTopicProject(t)
	outDir := filepath.Join(t.TempDir(), "out")
	if err := os.MkdirAll(outDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	h := DefaultWasmHelper()
	h.cache = loadWasmCache()

	s := structInfo{Name: "Page", KeyName: "page", Compression: WasmCompressionGzip}
	wasmName := "topic-" + s.KeyName
	outFile := filepath.Join(outDir, wasmName+".wasm.gz")
	if err := os.WriteFile(outFile, []byte("prebuilt-topic"), 0644); err != nil {
		t.Fatalf("write out: %v", err)
	}
	hash := h.topicManagerInputHash(s)
	h.cache.update(wasmName, hash)

	if err := h.buildTopicManager(s, nil, []structInfo{s}, map[string]string{}, nil, outDir, &sync.Once{}); err != nil {
		t.Fatalf("buildTopicManager cache-hit: %v", err)
	}
	if data, _ := os.ReadFile(outFile); string(data) != "prebuilt-topic" {
		t.Errorf("expected prebuilt topic file untouched, got %q", data)
	}
}
