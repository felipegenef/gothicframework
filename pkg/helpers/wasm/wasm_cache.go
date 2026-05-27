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

	wasmruntime "github.com/felipegenef/gothicframework/pkg/wasm"
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
	h.feedHandwrittenPackageFiles(hh, filepath.Dir(page.SourceFile), page.SourceFile)
	h.feedTopicFiles(hh)
	h.feedFile(hh, tmplWasmPageMain)
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

// topicManagerInputHash hashes all topic files, the manager template, and the compression method.
func (h *WasmHelper) topicManagerInputHash(compression WasmCompression) string {
	hh := sha256.New()
	h.feedTopicFiles(hh)
	h.feedFile(hh, tmplTopicManagerMain)
	h.feedRuntimeFS(hh)
	hh.Write([]byte{byte(compression)})
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
