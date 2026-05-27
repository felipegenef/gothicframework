package helpers

import (
	"fmt"
	"go/ast"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/felipegenef/gothicframework/v2/pkg/helpers/wasm/astx"
	"golang.org/x/tools/go/packages"
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
	for i, dir := range []string{pagesDir, componentsDir} {
		isComponent := i == 1
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
				page.IsComponent = isComponent
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

	helperDecls, helperPkgs, err := astx.ExtractUsedHelpers(entry.Pkg, res.Body)
	if err != nil {
		return WasmPage{}, false, fmt.Errorf("wasm: extract helpers in %s: %w", path, err)
	}

	localPackageDirs := collectLocalPackageDirs(entry.Pkg, helperPkgs)

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
		Compression:      compression,
		Compiler:         compiler,
		LocalPackageDirs: localPackageDirs,
	}, true, nil
}

// collectLocalPackageDirs returns the sorted, de-duplicated absolute directories
// of every local (user-module) package referenced by the helpers extracted from
// the page body. Stdlib, third-party, and vendored packages are skipped. These
// directories feed into the per-page WASM cache hash so a change to any file in
// an imported local package invalidates the cache.
func collectLocalPackageDirs(pagePkg *packages.Package, helperPkgs []*types.PkgName) []string {
	if pagePkg == nil || len(helperPkgs) == 0 {
		return nil
	}
	userModulePath, _, err := ReadUserModulePath(".")
	if err != nil || userModulePath == "" {
		return nil
	}
	prefix := userModulePath + "/"

	seen := make(map[string]struct{}, len(helperPkgs))
	for _, pn := range helperPkgs {
		if pn == nil {
			continue
		}
		imported := pn.Imported()
		if imported == nil {
			continue
		}
		path := imported.Path()
		if path != userModulePath && !strings.HasPrefix(path, prefix) {
			continue
		}
		loadedPkg, ok := pagePkg.Imports[path]
		if !ok || loadedPkg == nil {
			continue
		}
		if loadedPkg.Module != nil && loadedPkg.Module.Path != userModulePath {
			continue
		}
		if len(loadedPkg.GoFiles) == 0 {
			continue
		}
		dir := filepath.Dir(loadedPkg.GoFiles[0])
		if strings.Contains(dir, string(os.PathSeparator)+"vendor"+string(os.PathSeparator)) {
			continue
		}
		absDir, err := filepath.Abs(dir)
		if err != nil {
			absDir = dir
		}
		seen[absDir] = struct{}{}
	}
	if len(seen) == 0 {
		return nil
	}
	dirs := make([]string, 0, len(seen))
	for d := range seen {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)
	return dirs
}

// stdImportLines formats import specs into the legacy WasmPage.Imports line
// format: either `"path"` or `alias "path"`. Previously this filtered out
// non-stdlib imports, but with module bridging the temp build can resolve
// user-project and third-party imports too, so all imports pass through.
func stdImportLines(specs []*ast.ImportSpec) []string {
	var out []string
	for _, sp := range specs {
		if sp.Path == nil {
			continue
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
