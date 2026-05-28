package helpers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	wasmruntime "github.com/felipegenef/gothicframework/v2/pkg/wasm"
)

// Build-output hash cache. Stored at wasmCachePath as a flat
// {<name>: sha256-hex} JSON. Used to skip re-building WASMs whose input
// files have not changed.

const wasmCachePath = ".gothicCli/wasm-cache.json"

// wasmCache persists per-target content hashes so unchanged WASMs are skipped.
// The cache is stored at wasmCachePath and loaded once per GenerateAll invocation.
type wasmCache struct {
	mu     sync.Mutex
	hashes map[string]string
}

func loadWasmCache() *wasmCache {
	c := &wasmCache{hashes: make(map[string]string)}
	if data, err := os.ReadFile(wasmCachePath); err == nil {
		_ = json.Unmarshal(data, &c.hashes)
	}
	return c
}

func (c *wasmCache) upToDate(name, hash string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return hash != "" && c.hashes[name] == hash
}

func (c *wasmCache) update(name, hash string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.hashes[name] = hash
}

func (c *wasmCache) save() {
	c.mu.Lock()
	data, err := json.MarshalIndent(c.hashes, "", "  ")
	c.mu.Unlock()
	if err != nil {
		return
	}
	_ = os.WriteFile(wasmCachePath, data, 0644)
}

// pageInputHash hashes the source file, all topic files, and the page template.
// Any change in these inputs produces a different hash and triggers a rebuild.
func (h *WasmHelper) pageInputHash(page WasmPage) string {
	hh := sha256.New()
	if data, err := os.ReadFile(page.SourceFile); err == nil {
		hh.Write(data)
	}
	// Phase 12: per-symbol hashing for the page's own package. Instead of
	// hashing every hand-written .go file in the page's directory, hash only
	// the formatted source of the AST decls the page's ClientSideState body
	// actually references. UsedDeclSources is pre-sorted by the scanner.
	for _, src := range page.UsedDeclSources {
		io.WriteString(hh, src)
		io.WriteString(hh, "\x00")
	}
	// Hash hand-written files in every local package contributing helpers to
	// this page, so changes in cross-package dependencies invalidate the cache.
	// page.LocalPackageDirs is pre-sorted by the scanner for determinism.
	for _, dir := range page.LocalPackageDirs {
		h.feedHandwrittenPackageFiles(hh, dir, "")
	}

	h.feedEmbeddedTemplate(hh, tmplWasmPageMain)
	h.feedRuntimeFS(hh)
	hh.Write([]byte{byte(page.Compression)})
	hh.Write([]byte{byte(page.Compiler)})
	return hex.EncodeToString(hh.Sum(nil))
}

// feedHandwrittenPackageFiles hashes hand-written .go files in the page's
// package directory so changes to sibling files (e.g. state.go) invalidate the
// per-page WASM cache. Generated files (*_templ.go, *_gen.go), test files
// (*_test.go), and the page's own SourceFile (exclude) are skipped. Files are
// processed in alphabetical order for determinism.
func (h *WasmHelper) feedHandwrittenPackageFiles(hh io.Writer, dir string, exclude string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	absExclude, err := filepath.Abs(exclude)
	if err != nil {
		absExclude = exclude
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".go") {
			continue
		}
		if strings.HasSuffix(name, "_templ.go") ||
			strings.HasSuffix(name, "_gen.go") ||
			strings.HasSuffix(name, "_test.go") {
			continue
		}
		full := filepath.Join(dir, name)
		absFull, err := filepath.Abs(full)
		if err != nil {
			absFull = full
		}
		if absFull == absExclude {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		hh.Write([]byte(name))
		hh.Write([]byte{0})
		hh.Write(data)
	}
}

// topicManagerInputHash hashes the topic's source directory, the manager template,
// and the specific configuration (name, compression, compiler).
func (h *WasmHelper) topicManagerInputHash(s structInfo) string {
	hh := sha256.New()
	if sourceDir, _, ok := resolveTopicSourceDir(); ok {
		h.feedHandwrittenPackageFiles(hh, sourceDir, "")
	}
	h.feedEmbeddedTemplate(hh, tmplTopicManagerMain)
	h.feedRuntimeFS(hh)
	hh.Write([]byte(s.Name))
	hh.Write([]byte{byte(s.Compression)})
	hh.Write([]byte{byte(s.Compiler)})
	return hex.EncodeToString(hh.Sum(nil))
}

func (h *WasmHelper) feedTopicFiles(hh io.Writer) {
	sourceDir, genFile, ok := resolveTopicSourceDir()
	if !ok {
		return
	}
	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		return
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".go") && e.Name() != genFile {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		if data, err := os.ReadFile(filepath.Join(sourceDir, name)); err == nil {
			hh.Write(data)
		}
	}
}

// feedRuntimeFS hashes the embedded WASM runtime sources so any change to the
// runtime (events.go, topic.go, dom.go, etc.) invalidates the per-page WASM
// cache. Files are walked in sorted order for deterministic hashing.
func (h *WasmHelper) feedRuntimeFS(hh io.Writer) {
	var paths []string
	_ = fs.WalkDir(wasmruntime.RuntimeFS, "wasm-runtime/runtime", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		paths = append(paths, path)
		return nil
	})
	sort.Strings(paths)
	for _, p := range paths {
		if data, err := wasmruntime.RuntimeFS.ReadFile(p); err == nil {
			hh.Write([]byte(p))
			hh.Write([]byte{0})
			hh.Write(data)
		}
	}
}

func (h *WasmHelper) feedFile(hh io.Writer, path string) {
	if data, err := os.ReadFile(path); err == nil {
		hh.Write(data)
	}
}

// feedEmbeddedTemplate hashes a template that lives inside the CLI binary's
// embed.FS rather than on disk. This is used by the WASM cache so that
// upgrading the CLI (which can change the embedded template bytes) properly
// invalidates per-page and per-manager caches.
func (h *WasmHelper) feedEmbeddedTemplate(hh io.Writer, embedPath string) {
	if data, err := WasmTemplateFS.ReadFile(embedPath); err == nil {
		hh.Write(data)
	}
}
