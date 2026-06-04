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

// TestPageInputHash_LocalPackageDirsModificationInvalidates ensures that
// modifying a .go file inside a directory listed in page.LocalPackageDirs
// changes the page hash. This is the Phase 5 cross-package cache fix.
func TestPageInputHash_LocalPackageDirsModificationInvalidates(t *testing.T) {
	dir := withTempCwd(t)
	srcPath := filepath.Join(dir, "page.go")
	if err := os.WriteFile(srcPath, []byte("package x\n"), 0644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	pkgDir := filepath.Join(dir, "pkg_helper")
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		t.Fatalf("mkdir pkg_helper: %v", err)
	}
	helperPath := filepath.Join(pkgDir, "helper.go")
	if err := os.WriteFile(helperPath, []byte("package pkg_helper\nvar V = 1\n"), 0644); err != nil {
		t.Fatalf("write helper.go: %v", err)
	}

	h := DefaultWasmHelper()
	page := WasmPage{
		SourceFile:       srcPath,
		Compression:      WasmCompressionGzip,
		LocalPackageDirs: []string{pkgDir},
	}
	before := h.pageInputHash(page)

	if err := os.WriteFile(helperPath, []byte("package pkg_helper\nvar V = 2\n"), 0644); err != nil {
		t.Fatalf("rewrite helper.go: %v", err)
	}
	after := h.pageInputHash(page)
	if before == after {
		t.Errorf("expected hash to change when local package file changes; both were %q", before)
	}
}

// TestPageInputHash_RemovingLocalPackageDirStopsTracking ensures that once a
// directory is removed from LocalPackageDirs, edits there no longer affect the
// page hash.
func TestPageInputHash_RemovingLocalPackageDirStopsTracking(t *testing.T) {
	dir := withTempCwd(t)
	srcPath := filepath.Join(dir, "page.go")
	if err := os.WriteFile(srcPath, []byte("package x\n"), 0644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	pkgDir := filepath.Join(dir, "pkg_helper")
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		t.Fatalf("mkdir pkg_helper: %v", err)
	}
	helperPath := filepath.Join(pkgDir, "helper.go")
	if err := os.WriteFile(helperPath, []byte("package pkg_helper\nvar V = 1\n"), 0644); err != nil {
		t.Fatalf("write helper.go: %v", err)
	}

	h := DefaultWasmHelper()
	pageWithout := WasmPage{
		SourceFile:  srcPath,
		Compression: WasmCompressionGzip,
	}
	before := h.pageInputHash(pageWithout)

	if err := os.WriteFile(helperPath, []byte("package pkg_helper\nvar V = 2\n"), 0644); err != nil {
		t.Fatalf("rewrite helper.go: %v", err)
	}
	after := h.pageInputHash(pageWithout)
	if before != after {
		t.Errorf("expected hash unchanged when dir is not tracked; got %q vs %q", before, after)
	}
}

// TestPageInputHash_LocalPackageDirsOrderIndependent ensures that, since the
// scanner sorts LocalPackageDirs before storing, hashes produced from the same
// (sorted) input list are identical regardless of original discovery order.
// We simulate the scanner contract by sorting inside the test before feeding.
func TestPageInputHash_LocalPackageDirsOrderIndependent(t *testing.T) {
	dir := withTempCwd(t)
	srcPath := filepath.Join(dir, "page.go")
	if err := os.WriteFile(srcPath, []byte("package x\n"), 0644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	mk := func(name, body string) string {
		d := filepath.Join(dir, name)
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatalf("mkdir %s: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(d, "f.go"), []byte(body), 0644); err != nil {
			t.Fatalf("write %s/f.go: %v", name, err)
		}
		return d
	}
	a := mk("a_pkg", "package a_pkg\nvar A = 1\n")
	b := mk("b_pkg", "package b_pkg\nvar B = 1\n")
	c := mk("c_pkg", "package c_pkg\nvar C = 1\n")

	h := DefaultWasmHelper()

	sorted1 := []string{a, b, c}
	sorted2 := []string{c, b, a}
	// Mimic the scanner sorting contract before storing on the Page.
	cp := append([]string(nil), sorted2...)
	sortStrings(cp)

	p1 := WasmPage{SourceFile: srcPath, Compression: WasmCompressionGzip, LocalPackageDirs: sorted1}
	p2 := WasmPage{SourceFile: srcPath, Compression: WasmCompressionGzip, LocalPackageDirs: cp}

	h1 := h.pageInputHash(p1)
	h2 := h.pageInputHash(p2)
	if h1 != h2 {
		t.Errorf("expected identical hashes for same sorted LocalPackageDirs; got %q vs %q", h1, h2)
	}
}

// TestFeedRuntimeFS_ContributesEmbeddedBytes documents the second major cache
// invalidation trigger: upgrading the CLI itself. The runtime sources are baked
// into the binary via wasmruntime.RuntimeFS (an embed.FS package var), so they
// are not injectable as a parameter — they change only when the CLI is rebuilt
// from changed sources. We therefore cannot swap the embedded bytes from a test
// without modifying production code. The strongest hermetic guarantee we can
// give is that feedRuntimeFS actually feeds non-empty, content-bearing bytes
// into the hash: if it ever silently fed nothing (e.g. a broken embed path),
// changing the CLI's runtime would NOT invalidate the cache and stale WASMs
// would ship. This test fails loudly in that scenario.
func TestFeedRuntimeFS_ContributesEmbeddedBytes(t *testing.T) {
	h := DefaultWasmHelper()
	var buf bytes.Buffer
	h.feedRuntimeFS(&buf)
	if buf.Len() == 0 {
		t.Fatal("feedRuntimeFS wrote no bytes; embedded runtime is not part of the hash, so a CLI upgrade would not invalidate the cache")
	}
	// The embedded runtime must include a recognizable runtime source path,
	// proving real content (not just an empty walk) flows into the hash.
	if !bytes.Contains(buf.Bytes(), []byte("wasm-runtime/runtime")) {
		t.Errorf("feedRuntimeFS output missing runtime source paths; got %d bytes without the expected path marker", buf.Len())
	}
}

// TestFeedEmbeddedTemplate_ContributesEmbeddedBytes documents that the embedded
// page/manager templates are part of the cache hash. Like the runtime FS, these
// live in WasmTemplateFS (an embed.FS package var) and change only on CLI
// rebuild, so they are not injectable from a test. We verify the helper feeds
// the real template bytes and that distinct templates produce distinct bytes —
// guaranteeing that a CLI upgrade which alters a template invalidates the cache.
func TestFeedEmbeddedTemplate_ContributesEmbeddedBytes(t *testing.T) {
	h := DefaultWasmHelper()

	var page bytes.Buffer
	h.feedEmbeddedTemplate(&page, tmplWasmPageMain)
	if page.Len() == 0 {
		t.Fatal("feedEmbeddedTemplate wrote no bytes for the page template; a CLI template change would not invalidate the cache")
	}

	var manager bytes.Buffer
	h.feedEmbeddedTemplate(&manager, tmplTopicManagerMain)
	if manager.Len() == 0 {
		t.Fatal("feedEmbeddedTemplate wrote no bytes for the manager template")
	}

	if bytes.Equal(page.Bytes(), manager.Bytes()) {
		t.Error("expected distinct page and manager template bytes to be fed into the hash; identical bytes mean template identity is invisible to the cache")
	}
}

// TestPageInputHash_EmbeddedInputsAreLoadBearing proves end-to-end that the
// embedded runtime FS and embedded page template are actually wired into the
// page hash. We compute the real hash, then recompute a hash that omits exactly
// those two embedded inputs; if the embedded inputs contributed nothing, the two
// hashes would be equal. A divergence proves the embedded bytes are load-bearing
// — i.e. a CLI upgrade that changes them flips the page hash and forces rebuild.
func TestPageInputHash_EmbeddedInputsAreLoadBearing(t *testing.T) {
	dir := withTempCwd(t)
	srcPath := filepath.Join(dir, "page.go")
	if err := os.WriteFile(srcPath, []byte("package x\nvar A = 1\n"), 0644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	h := DefaultWasmHelper()
	page := WasmPage{SourceFile: srcPath, Compression: WasmCompressionGzip}

	real := h.pageInputHash(page)

	// Reconstruct the hash WITHOUT the embedded template and runtime FS inputs,
	// mirroring pageInputHash's input ordering otherwise.
	hh := sha256.New()
	if data, err := os.ReadFile(page.SourceFile); err == nil {
		hh.Write(data)
	}
	for _, src := range page.UsedDeclSources {
		hh.Write([]byte(src))
		hh.Write([]byte{0})
	}
	for _, d := range page.LocalPackageDirs {
		h.feedHandwrittenPackageFiles(hh, d, "")
	}
	// (deliberately skip feedEmbeddedTemplate + feedRuntimeFS)
	hh.Write([]byte{byte(page.Compression)})
	hh.Write([]byte{byte(page.Compiler)})
	without := hex.EncodeToString(hh.Sum(nil))

	if real == without {
		t.Error("page hash is identical with and without embedded template/runtime inputs; embedded CLI bytes are NOT part of the cache key, so a CLI upgrade would not invalidate the cache")
	}
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
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

// Phase 12 — Per-symbol cache hashing tests. These exercise the
// UsedDeclSources path in pageInputHash directly by constructing WasmPage
// values with pre-populated decl sources, bypassing the scanner.

// TestPageInputHash_UsedDeclSources_ValueChangeInvalidates ensures that
// changing the source of a tracked decl (e.g. const value) flips the hash.
func TestPageInputHash_UsedDeclSources_ValueChangeInvalidates(t *testing.T) {
	dir := withTempCwd(t)
	srcPath := filepath.Join(dir, "page.go")
	if err := os.WriteFile(srcPath, []byte("package x\n"), 0644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	h := DefaultWasmHelper()
	pageBefore := WasmPage{
		SourceFile:      srcPath,
		Compression:     WasmCompressionGzip,
		UsedDeclSources: []string{"const CounterStep = 5"},
	}
	pageAfter := WasmPage{
		SourceFile:      srcPath,
		Compression:     WasmCompressionGzip,
		UsedDeclSources: []string{"const CounterStep = 6"},
	}
	before := h.pageInputHash(pageBefore)
	after := h.pageInputHash(pageAfter)
	if before == after {
		t.Errorf("expected hash to change when a tracked decl source changes; both were %q", before)
	}
}

// TestPageInputHash_UsedDeclSources_UnrelatedSymbolsIgnored ensures that
// symbols which aren't in UsedDeclSources do NOT affect the hash. This is the
// whole point of per-symbol hashing — sibling decls the page doesn't use are
// transparent to the cache.
func TestPageInputHash_UsedDeclSources_UnrelatedSymbolsIgnored(t *testing.T) {
	dir := withTempCwd(t)
	srcPath := filepath.Join(dir, "page.go")
	if err := os.WriteFile(srcPath, []byte("package x\n"), 0644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	h := DefaultWasmHelper()
	page := WasmPage{
		SourceFile:      srcPath,
		Compression:     WasmCompressionGzip,
		UsedDeclSources: []string{"const CounterStep = 5"},
	}
	// Writing an unrelated state.go to the same dir must NOT change the hash
	// now that pageInputHash no longer hashes the whole package directory.
	before := h.pageInputHash(page)
	statePath := filepath.Join(dir, "state.go")
	if err := os.WriteFile(statePath, []byte("package x\nconst Unrelated = 42\n"), 0644); err != nil {
		t.Fatalf("write state.go: %v", err)
	}
	after := h.pageInputHash(page)
	if before != after {
		t.Errorf("expected hash unchanged when unrelated sibling decl appears; got %q vs %q", before, after)
	}
}

// TestPageInputHash_UsedDeclSources_HelperBodyChangeInvalidates ensures that
// a func helper's body change (different formatted source) flips the hash.
func TestPageInputHash_UsedDeclSources_HelperBodyChangeInvalidates(t *testing.T) {
	dir := withTempCwd(t)
	srcPath := filepath.Join(dir, "page.go")
	if err := os.WriteFile(srcPath, []byte("package x\n"), 0644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	h := DefaultWasmHelper()
	pageBefore := WasmPage{
		SourceFile:      srcPath,
		Compression:     WasmCompressionGzip,
		UsedDeclSources: []string{"func Foo() int { return 1 }"},
	}
	pageAfter := WasmPage{
		SourceFile:      srcPath,
		Compression:     WasmCompressionGzip,
		UsedDeclSources: []string{"func Foo() int { return 2 }"},
	}
	before := h.pageInputHash(pageBefore)
	after := h.pageInputHash(pageAfter)
	if before == after {
		t.Errorf("expected hash to change when helper body changes; both were %q", before)
	}
}

// TestPageInputHash_UsedDeclSources_Deterministic ensures that repeated hash
// calls over the same WasmPage yield identical results — sort stability and
// hashing should be deterministic.
func TestPageInputHash_UsedDeclSources_Deterministic(t *testing.T) {
	dir := withTempCwd(t)
	srcPath := filepath.Join(dir, "page.go")
	if err := os.WriteFile(srcPath, []byte("package x\n"), 0644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	h := DefaultWasmHelper()
	page := WasmPage{
		SourceFile:  srcPath,
		Compression: WasmCompressionGzip,
		UsedDeclSources: []string{
			"const A = 1",
			"const B = 2",
			"func F() int { return 3 }",
		},
	}
	h1 := h.pageInputHash(page)
	h2 := h.pageInputHash(page)
	h3 := h.pageInputHash(page)
	if h1 != h2 || h2 != h3 {
		t.Errorf("expected deterministic hash across calls; got %q %q %q", h1, h2, h3)
	}
}
