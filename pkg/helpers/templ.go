package helpers

import (
	"context"
	"fmt"
	"os"

	templgen "github.com/a-h/templ/cmd/templ/generatecmd"
	templcache "github.com/felipegenef/gothicframework/pkg/helpers/templ"
)

type TemplHelper struct {
}

func NewTemplHelper() TemplHelper {
	return TemplHelper{}
}

// Render runs `templ generate`, but skips files whose contents are unchanged
// since the last successful run (tracked via .gothicCli/templ-cache.json).
//
// Behavior:
//   - Scans the working directory for .templ files.
//   - If every file is cache-hit (and the matching _templ.go exists), it
//     returns without invoking templ at all.
//   - Otherwise it runs templ per dirty file using the -f flag.
//     If any per-file run fails (e.g. unsupported by the installed templ
//     version), it falls back to a full-project generation.
//   - On success, updates and persists the cache.
func (t *TemplHelper) Render() error {
	cache := templcache.Load()
	files, err := templcache.ScanTemplFiles(".")
	if err != nil {
		// Cache is best-effort — fall back to a full run rather than failing.
		return templgen.Run(context.Background(), os.Stdout, os.Stderr, []string{"generate"})
	}

	dirty := templcache.DirtyFiles(cache, files)
	if len(dirty) == 0 && len(files) > 0 {
		// Everything up to date — skip templ entirely.
		return nil
	}

	if perFileErr := generatePerFile(dirty); perFileErr != nil {
		// Fallback: regenerate everything. We still refresh the cache afterwards
		// so subsequent runs benefit from the optimization.
		if err := templgen.Run(context.Background(), os.Stdout, os.Stderr, []string{"generate"}); err != nil {
			return fmt.Errorf("templ generate (fallback after per-file error %v): %w", perFileErr, err)
		}
	}

	// Refresh hashes for every scanned file and save.
	for _, f := range files {
		if h := templcache.HashFile(f); h != "" {
			cache.Update(f, h)
		}
	}
	_ = cache.Save()
	return nil
}

// generatePerFile invokes `templ generate` once per dirty file using the -f flag.
// If any single invocation fails, the error is returned so the caller can fall
// back to a full-project generation.
func generatePerFile(dirty []string) error {
	for _, f := range dirty {
		if err := templgen.Run(context.Background(), os.Stdout, os.Stderr, []string{"generate", "-f", f}); err != nil {
			return fmt.Errorf("templ generate %s: %w", f, err)
		}
	}
	return nil
}
