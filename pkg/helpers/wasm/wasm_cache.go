package helpers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
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

// pageInputHash hashes the source file, all context files, and the page template.
// Any change in these inputs produces a different hash and triggers a rebuild.
func (h *WasmHelper) pageInputHash(page WasmPage) string {
	hh := sha256.New()
	if data, err := os.ReadFile(page.SourceFile); err == nil {
		hh.Write(data)
	}
	h.feedContextFiles(hh)
	h.feedFile(hh, tmplWasmPageMain)
	hh.Write([]byte{byte(page.Compression)})
	return hex.EncodeToString(hh.Sum(nil))
}

// ctxManagerInputHash hashes all context files, the manager template, and the compression method.
func (h *WasmHelper) ctxManagerInputHash(compression WasmCompression) string {
	hh := sha256.New()
	h.feedContextFiles(hh)
	h.feedFile(hh, tmplCtxManagerMain)
	hh.Write([]byte{byte(compression)})
	return hex.EncodeToString(hh.Sum(nil))
}

func (h *WasmHelper) feedContextFiles(hh io.Writer) {
	entries, err := os.ReadDir("src/context")
	if err != nil {
		return
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".go") && e.Name() != "context_gen.go" {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		if data, err := os.ReadFile(filepath.Join("src/context", name)); err == nil {
			hh.Write(data)
		}
	}
}

func (h *WasmHelper) feedFile(hh io.Writer, path string) {
	if data, err := os.ReadFile(path); err == nil {
		hh.Write(data)
	}
}
