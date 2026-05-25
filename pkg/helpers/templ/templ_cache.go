// Package templ provides a content-hash cache for .templ files so that
// `templ generate` is only invoked for files whose contents (or generated
// counterparts) have changed since the last run.
//
// Cache layout: a flat {<relative-path>: sha256-hex} JSON map persisted at
// templCachePath. The format mirrors pkg/helpers/wasm/wasm_cache.go so both
// caches behave consistently and remain easy to inspect by hand.
package templ

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// TemplCachePath is the on-disk location of the templ hash cache.
// Exported so callers (and tests) can override behavior if needed.
const TemplCachePath = ".gothicCli/templ-cache.json"

// Cache holds per-file content hashes of .templ files.
// It is safe for concurrent use; callers typically Load, mutate via Update,
// then Save in a single hot-reload cycle.
type Cache struct {
	mu     sync.Mutex
	path   string
	hashes map[string]string
}

// Load reads the cache at TemplCachePath. Missing or malformed files yield an
// empty cache rather than an error — the cache is an optimization, never a
// correctness boundary.
func Load() *Cache {
	return LoadAt(TemplCachePath)
}

// LoadAt is Load with an explicit path; useful for tests that need isolation.
func LoadAt(path string) *Cache {
	c := &Cache{path: path, hashes: make(map[string]string)}
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &c.hashes)
	}
	return c
}

// HashFile computes the SHA-256 hex digest of a file's contents.
// Returns "" if the file cannot be read so callers can treat the file as dirty.
func HashFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// UpToDate reports whether the cached hash for key matches hash and is non-empty.
func (c *Cache) UpToDate(key, hash string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return hash != "" && c.hashes[key] == hash
}

// Update records hash for key in memory. Call Save to persist.
func (c *Cache) Update(key, hash string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.hashes[key] = hash
}

// Invalidate removes key from the cache (e.g., when the generated _templ.go
// counterpart is missing and we want to force regeneration).
func (c *Cache) Invalidate(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.hashes, key)
}

// Save atomically writes the cache to disk (write to .tmp then rename).
// Failures are silent — the cache is best-effort.
func (c *Cache) Save() error {
	c.mu.Lock()
	data, err := json.MarshalIndent(c.hashes, "", "  ")
	c.mu.Unlock()
	if err != nil {
		return err
	}
	if dir := filepath.Dir(c.path); dir != "" {
		_ = os.MkdirAll(dir, 0755)
	}
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, c.path)
}

// ScanTemplFiles walks root and returns the relative paths of every .templ
// file found. Hidden directories (".git", "node_modules", ".gothicCli") are
// skipped to avoid touching generated or vendored trees.
func ScanTemplFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "node_modules" || name == ".gothicCli" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(d.Name(), ".templ") {
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				rel = path
			}
			files = append(files, rel)
		}
		return nil
	})
	return files, err
}

// DirtyFiles returns the subset of files whose hash differs from the cache
// or whose generated _templ.go counterpart is missing. Files that hash to ""
// (unreadable) are also returned so the caller surfaces the error via templ.
func DirtyFiles(c *Cache, files []string) []string {
	dirty := make([]string, 0, len(files))
	for _, f := range files {
		h := HashFile(f)
		genPath := generatedCounterpart(f)
		if _, err := os.Stat(genPath); err != nil {
			c.Invalidate(f)
			dirty = append(dirty, f)
			continue
		}
		if !c.UpToDate(f, h) {
			dirty = append(dirty, f)
		}
	}
	return dirty
}

// generatedCounterpart returns the path of the _templ.go file that
// `templ generate` produces for a given .templ source.
func generatedCounterpart(templPath string) string {
	return strings.TrimSuffix(templPath, ".templ") + "_templ.go"
}
