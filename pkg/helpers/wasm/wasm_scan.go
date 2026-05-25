package helpers

import (
	"fmt"
	"go/ast"
	"os"
	"path/filepath"
	"strings"

	"github.com/felipegenef/gothicframework/pkg/helpers/wasm/astx"
)

// Page scanning: walk pagesDir and componentsDir looking for *_templ.go files
// that declare a ClientSideState body (either inline or via a named function).
// Extract that body and the page's std imports for the WASM build pipeline.
//
// As of Phase 1 of the AST refactor, extraction is driven by go/packages +
// go/ast rather than regular expressions. A single astx.Loader is constructed
// at the top of ScanPages over the current working directory ("./...") and is
// reused for every scanned file. The loader is cleared via defer so it does
// not leak into later calls.

func (h *WasmHelper) ScanPages(pagesDir, componentsDir string) ([]WasmPage, error) {
	// Initialise the AST loader over the project root. "." is the canonical
	// root used by all CLI commands that invoke ScanPages (deploy, wasm, hot
	// reload). Loading once means TypesInfo is shared across page files in
	// the same package, which is required for cross-file helper resolution.
	loader, err := astx.NewLoader(".")
	if err != nil {
		return nil, fmt.Errorf("wasm: load packages: %w", err)
	}
	h.astLoader = loader
	defer func() { h.astLoader = nil }()

	var pages []WasmPage
	for _, dir := range []string{pagesDir, componentsDir} {
		if dir == "" {
			continue
		}
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return err
			}
			if !strings.HasSuffix(info.Name(), "_templ.go") {
				return nil
			}
			page, found, ferr := h.scanFile(path)
			if ferr != nil {
				return ferr
			}
			if found {
				pages = append(pages, page)
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("wasm: scan %s: %w", dir, err)
		}
	}
	return pages, nil
}

func (h *WasmHelper) scanFile(path string) (WasmPage, bool, error) {
	if h.astLoader == nil {
		return WasmPage{}, false, fmt.Errorf("wasm: scanFile called without an initialised astLoader (call ScanPages)")
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return WasmPage{}, false, fmt.Errorf("wasm: abs %s: %w", path, err)
	}
	entry, err := h.astLoader.Get(absPath)
	if err != nil {
		// File is not in any loaded package — silently skip (e.g. generated
		// files outside the module). This mirrors the old regex behaviour of
		// quietly ignoring files with no match.
		return WasmPage{}, false, nil
	}

	res, found, err := astx.ExtractClientSideStateBody(entry)
	if err != nil {
		return WasmPage{}, false, fmt.Errorf("wasm: extract ClientSideState in %s: %w", path, err)
	}
	if !found {
		return WasmPage{}, false, nil
	}

	helperDecls, _, err := astx.ExtractUsedHelpers(entry.Pkg, res.Body)
	if err != nil {
		return WasmPage{}, false, fmt.Errorf("wasm: extract helpers in %s: %w", path, err)
	}

	importSpecs, err := astx.ExtractUsedImports(entry.Pkg, res.Body)
	if err != nil {
		return WasmPage{}, false, fmt.Errorf("wasm: extract imports in %s: %w", path, err)
	}

	// Format helper decls into Go source strings.
	var helpers []string
	for _, d := range helperDecls {
		src, err := astx.FormatNode(d, h.astLoader.Fset)
		if err != nil {
			return WasmPage{}, false, fmt.Errorf("wasm: format helper in %s: %w", path, err)
		}
		helpers = append(helpers, src)
	}

	// Format body (outer braces stripped by FormatNode for *ast.BlockStmt).
	body, err := astx.FormatNode(res.Body, h.astLoader.Fset)
	if err != nil {
		return WasmPage{}, false, fmt.Errorf("wasm: format body in %s: %w", path, err)
	}

	// Convert import specs to legacy []string format, keeping only std-lib
	// imports (paths with no "." in the first segment). Preserve aliases.
	stdImports := stdImportLines(importSpecs)

	// If any helper references an identifier that needs an external pkg, we
	// also need to look at imports used inside helpers. Re-scan over helper
	// declarations too.
	for _, d := range helperDecls {
		moreImports, err := astx.ExtractUsedImports(entry.Pkg, d)
		if err != nil {
			return WasmPage{}, false, fmt.Errorf("wasm: extract helper imports in %s: %w", path, err)
		}
		for _, line := range stdImportLines(moreImports) {
			if !containsString(stdImports, line) {
				stdImports = append(stdImports, line)
			}
		}
	}

	httpPath := h.normalizeWasmHttpPath(path)
	outputName := h.wasmOutputName(httpPath)

	compression := WasmCompressionGzip
	if res.Compression == "BROTLI" {
		compression = WasmCompressionBrotli
	}

	compiler := WasmCompilerGothicTinyGo
	switch res.Compiler {
	case "LocalTinyGo":
		compiler = WasmCompilerLocalTinyGo
	case "Golang":
		compiler = WasmCompilerGolang
	}

	return WasmPage{
		SourceFile:  path,
		FuncBody:    body,
		Imports:     stdImports,
		Helpers:     helpers,
		HttpPath:    httpPath,
		OutputName:  outputName,
		Compression: compression,
		Compiler:    compiler,
	}, true, nil
}

// stdImportLines filters import specs to keep only standard-library imports
// (those whose first path segment contains no "."), formatted as the
// legacy WasmPage.Imports lines expect: either `"path"` or `alias "path"`.
func stdImportLines(specs []*ast.ImportSpec) []string {
	var out []string
	for _, sp := range specs {
		if sp.Path == nil {
			continue
		}
		path := strings.Trim(sp.Path.Value, "\"")
		first := strings.SplitN(path, "/", 2)[0]
		if strings.Contains(first, ".") {
			continue // third-party
		}
		line := sp.Path.Value // already quoted
		if sp.Name != nil && sp.Name.Name != "" {
			line = sp.Name.Name + " " + line
		}
		out = append(out, line)
	}
	return out
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
