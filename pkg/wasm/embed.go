package wasm

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

//go:embed wasm-runtime/runtime
var RuntimeFS embed.FS

// ExtractRuntime writes the embedded WASM runtime source files into destDir so
// TinyGo can compile against them.  The resulting layout is:
//
//	destDir/
//	  go.mod          (module wasm-runtime, go 1.21)
//	  runtime/
//	    signal.go
//	    effect.go
//	    ...
//
// The caller is responsible for removing destDir when the build is done.
func ExtractRuntime(destDir string) error {
	gomod := "module wasm-runtime\n\ngo 1.21\n"
	if err := os.WriteFile(filepath.Join(destDir, "go.mod"), []byte(gomod), 0644); err != nil {
		return fmt.Errorf("write wasm-runtime go.mod: %w", err)
	}

	return fs.WalkDir(RuntimeFS, "wasm-runtime/runtime", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Strip the leading "wasm-runtime/" prefix to get the destination path.
		rel := strings.TrimPrefix(path, "wasm-runtime/")
		dest := filepath.Join(destDir, filepath.FromSlash(rel))

		if d.IsDir() {
			return os.MkdirAll(dest, 0755)
		}

		data, err := RuntimeFS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read embedded %s: %w", path, err)
		}
		return os.WriteFile(dest, data, 0644)
	})
}
