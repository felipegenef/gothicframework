package helpers

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	wasmexec "github.com/felipegenef/gothicframework/pkg/data/wasm_exec"
	wasmruntime "github.com/felipegenef/gothicframework/pkg/wasm"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"
)

// Build pipeline: orchestrates TinyGo compile + wasm-opt + compression for
// page WASMs and context-manager WASMs, plus the wasm_exec.js copy. Methods
// here run on the host (not inside WASM).

const (
	tmplWasmPageMain   = ".gothicCli/templates/wasm/wasm_page_main.go"
	tmplCtxManagerMain = ".gothicCli/templates/wasm/wasm_ctx_manager_main.go"
)

func (h *WasmHelper) GeneratePage(page WasmPage, outDir string) error {
	compressedExt := compressionExt(page.Compression)
	var hash string
	if h.cache != nil {
		hash = h.pageInputHash(page)
		outPath := filepath.Join(outDir, page.OutputName+".wasm"+compressedExt)
		if h.cache.upToDate(page.OutputName, hash) {
			if _, err := os.Stat(outPath); err == nil {
				wasmUpToDate(page.OutputName)
				return nil
			}
		}
	}
	// Remove stale files from any previous compression method.
	for _, ext := range []string{".gz", ".br"} {
		if ext != compressedExt {
			os.Remove(filepath.Join(outDir, page.OutputName+".wasm"+ext))
		}
	}

	tempModDir, err := os.MkdirTemp("", "tinygo-runtime-*")
	if err != nil {
		return fmt.Errorf("wasm: mkdirtemp: %w", err)
	}
	defer os.RemoveAll(tempModDir)

	if err := wasmruntime.ExtractRuntime(tempModDir); err != nil {
		return fmt.Errorf("wasm: extract runtime: %w", err)
	}

	genDir, err := os.MkdirTemp(tempModDir, ".gen-")
	if err != nil {
		return fmt.Errorf("wasm: mkdirtemp gen: %w", err)
	}

	mainPath := filepath.Join(genDir, "main.go")
	ctxSnippets, ctxStructs, ctxAliases := h.collectContextSnippets()
	body := h.rewriteContextCalls(page.FuncBody, ctxStructs)
	if err := h.writeWasmMain(page.SourceFile, body, page.Imports, page.Helpers, ctxSnippets, ctxStructs, ctxAliases, mainPath); err != nil {
		return err
	}

	if err := os.MkdirAll(outDir, 0755); err != nil {
		return fmt.Errorf("wasm: mkdir %s: %w", outDir, err)
	}

	absOutFile, err := filepath.Abs(filepath.Join(outDir, page.OutputName+".wasm"))
	if err != nil {
		return err
	}

	tinygo := h.TinyGoBinary()
	if h.ConfigOverride != "" {
		tinygo = h.ConfigOverride
	}

	pkg := "./" + filepath.Base(genDir) + "/"
	cmd := exec.Command(tinygo,
		"build", "-no-debug", "-opt=z",
		"-o", absOutFile,
		"-target", "wasm",
		"-gc", "conservative",
		pkg,
	)
	cmd.Dir = tempModDir
	cmd.Env = append(os.Environ(), h.Environ()...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("wasm: tinygo build %s: %w", page.OutputName, err)
	}

	wasmSize, _ := h.fileSize(absOutFile)

	if _, err := exec.LookPath("wasm-opt"); err == nil {
		tmp := absOutFile + ".opt"
		opt := exec.Command("wasm-opt", "-Oz", "--strip-debug", "-o", tmp, absOutFile)
		if err := opt.Run(); err == nil {
			os.Rename(tmp, absOutFile)
		} else {
			os.Remove(tmp)
		}
	}

	finalFile := absOutFile + compressedExt
	if err := h.compressWasmWith(absOutFile, finalFile, page.Compression); err != nil {
		return fmt.Errorf("wasm: compress %s: %w", page.OutputName, err)
	}
	os.Remove(absOutFile)

	finalSize, _ := h.fileSize(finalFile)
	wasmBuildResult(page.OutputName, h.formatBytes(wasmSize), h.formatBytes(finalSize), compressionLabel(page.Compression))
	if hash != "" {
		h.cache.update(page.OutputName, hash)
	}
	return nil
}

func (h *WasmHelper) GenerateAll(pages []WasmPage, outDir string) error {
	if err := h.EnsureBinary(); err != nil {
		return err
	}
	// Refresh on-disk WASM templates from the embedded copies so projects that
	// were initialised against an older CLI version pick up template fixes
	// (e.g. the trailing `select {}` that keeps TinyGo's main goroutine alive)
	// without needing to re-init.
	if err := h.EnsureWasmTemplates(); err != nil {
		return err
	}
	if len(pages) == 0 {
		return nil
	}

	if err := os.MkdirAll(outDir, 0755); err != nil {
		return fmt.Errorf("wasm: mkdir %s: %w", outDir, err)
	}

	h.cache = loadWasmCache()

	if err := h.GenerateContextManagers(outDir); err != nil {
		wasmErrorf("context manager build failed: %v", err)
	}

	g, gctx := errgroup.WithContext(context.Background())
	sem := semaphore.NewWeighted(int64(runtime.NumCPU()))
	for _, page := range pages {
		page := page
		g.Go(func() error {
			if err := sem.Acquire(gctx, 1); err != nil {
				return err
			}
			defer sem.Release(1)
			return h.GeneratePage(page, outDir)
		})
	}
	if err := g.Wait(); err != nil {
		return err
	}

	h.cache.save()
	return h.CopyWasmExec("public")
}

func (h *WasmHelper) CopyWasmExec(destDir string) error {
	dst := filepath.Join(destDir, "wasm_exec.js")
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("wasm: mkdir wasm_exec dir: %w", err)
	}
	return os.WriteFile(dst, wasmexec.Shim, 0644)
}

func (h *WasmHelper) GenerateContextManagers(outDir string) error {
	snippets, structs, aliases := h.collectContextSnippets()
	if !h.hasCtxStructs(structs) {
		return nil
	}

	if err := os.MkdirAll(outDir, 0755); err != nil {
		return fmt.Errorf("wasm: mkdir %s: %w", outDir, err)
	}
	for _, s := range structs {
		if s.KeyName == "" {
			continue
		}
		if err := h.buildContextManager(s, snippets, structs, aliases, outDir); err != nil {
			return err
		}
	}
	return nil
}

func (h *WasmHelper) buildContextManager(s structInfo, snippets []string, allStructs []structInfo, aliases map[string]string, outDir string) error {
	wasmName := "ctx-" + s.KeyName
	compression := s.Compression
	var hash string
	if h.cache != nil {
		hash = h.ctxManagerInputHash(compression)
		outPath := filepath.Join(outDir, wasmName+".wasm"+compressionExt(compression))
		if h.cache.upToDate(wasmName, hash) {
			if _, err := os.Stat(outPath); err == nil {
				wasmUpToDate(wasmName)
				return nil
			}
		}
	}
	// Remove stale files from any previous compression method.
	for _, ext := range []string{".gz", ".br"} {
		if ext != compressionExt(compression) {
			os.Remove(filepath.Join(outDir, wasmName+".wasm"+ext))
		}
	}

	tempModDir, err := os.MkdirTemp("", "tinygo-ctx-*")
	if err != nil {
		return fmt.Errorf("wasm: mkdirtemp: %w", err)
	}
	defer os.RemoveAll(tempModDir)

	if err := wasmruntime.ExtractRuntime(tempModDir); err != nil {
		return fmt.Errorf("wasm: extract runtime: %w", err)
	}

	genDir, err := os.MkdirTemp(tempModDir, ".gen-")
	if err != nil {
		return fmt.Errorf("wasm: mkdirtemp gen: %w", err)
	}

	mainPath := filepath.Join(genDir, "main.go")
	codecs, err := h.buildCodecData(allStructs, aliases)
	if err != nil {
		return fmt.Errorf("wasm: context codec: %w", err)
	}
	structNames := make(map[string]bool, len(allStructs))
	for _, st := range allStructs {
		structNames[st.Name] = true
	}
	fields, err := h.buildManagerFieldData(s, structNames, aliases)
	if err != nil {
		return fmt.Errorf("wasm: manager fields: %w", err)
	}
	if err := h.Template.UpdateFromTemplate(tmplCtxManagerMain, mainPath, WasmCtxManagerMainData{
		StructName:  s.Name,
		KeyName:     s.KeyName,
		HasTime:     h.hasTimeFields(allStructs),
		Codecs:      codecs,
		CtxSnippets: snippets,
		Fields:      fields,
	}); err != nil {
		return fmt.Errorf("wasm: render context manager main.go: %w", err)
	}

	absOutFile, err := filepath.Abs(filepath.Join(outDir, wasmName+".wasm"))
	if err != nil {
		return err
	}

	tinygo := h.TinyGoBinary()
	if h.ConfigOverride != "" {
		tinygo = h.ConfigOverride
	}

	pkg := "./" + filepath.Base(genDir) + "/"
	cmd := exec.Command(tinygo, "build", "-no-debug", "-opt=z", "-o", absOutFile, "-target", "wasm", "-gc", "conservative", pkg)
	cmd.Dir = tempModDir
	cmd.Env = append(os.Environ(), h.Environ()...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("wasm: tinygo build %s: %w", wasmName, err)
	}

	wasmSize, _ := h.fileSize(absOutFile)

	if _, err := exec.LookPath("wasm-opt"); err == nil {
		tmp := absOutFile + ".opt"
		if opt := exec.Command("wasm-opt", "-Oz", "--strip-debug", "-o", tmp, absOutFile); opt.Run() == nil {
			os.Rename(tmp, absOutFile)
		} else {
			os.Remove(tmp)
		}
	}

	compOutFile := absOutFile + compressionExt(compression) // absOutFile already ends in .wasm
	if err := h.compressWasmWith(absOutFile, compOutFile, compression); err != nil {
		return fmt.Errorf("wasm: compress %s: %w", wasmName, err)
	}
	os.Remove(absOutFile)

	compSize, _ := h.fileSize(compOutFile)
	wasmBuildResult(wasmName, h.formatBytes(wasmSize), h.formatBytes(compSize), compressionLabel(compression))
	if hash != "" {
		h.cache.update(wasmName, hash)
	}
	return nil
}

func (h *WasmHelper) writeWasmMain(src, body string, stdImports []string, helpers []string, ctxSnippets []string, ctxStructs []structInfo, aliases map[string]string, dest string) error {
	codecs, err := h.buildCodecData(ctxStructs, aliases)
	if err != nil {
		return fmt.Errorf("wasm: codec: %w", err)
	}
	wasmFuncs, err := h.buildWasmCtxFuncData(ctxStructs, aliases)
	if err != nil {
		return fmt.Errorf("wasm: ctx func data: %w", err)
	}
	// Inject "time" import when any context struct uses time.Time and the page
	// hasn't already imported it from its own source file.
	if h.hasTimeFields(ctxStructs) {
		hasTime := false
		for _, imp := range stdImports {
			if strings.Contains(imp, `"time"`) {
				hasTime = true
				break
			}
		}
		if !hasTime {
			stdImports = append(stdImports, `"time"`)
		}
	}
	// Strip a trailing `select {}` (or `select{}`) from the user's body before
	// indenting. The wasm_page_main template now always emits a `select {}` at
	// the end of main, so users no longer need to write it themselves. If a
	// user still has the old boilerplate, we remove it here to avoid emitting
	// `select{}` twice (which is a Go compile error: duplicate `select {}`
	// would actually be valid, but the second one would be dead code — keep
	// the template's copy as the canonical one).
	trimmed := strings.TrimRight(body, " \t\r\n")
	if strings.HasSuffix(trimmed, "select{}") {
		trimmed = strings.TrimSuffix(trimmed, "select{}")
	} else if strings.HasSuffix(trimmed, "select {}") {
		trimmed = strings.TrimSuffix(trimmed, "select {}")
	}
	body = strings.TrimRight(trimmed, " \t\r\n")

	var indented strings.Builder
	for _, line := range strings.Split(body, "\n") {
		indented.WriteString("\t" + line + "\n")
	}

	return h.Template.UpdateFromTemplate(tmplWasmPageMain, dest, WasmPageMainData{
		SourceFile:  src,
		StdImports:  stdImports,
		Codecs:      codecs,
		KeyVars:     h.buildKeyVarData(ctxStructs),
		CtxTypes:    h.buildCtxTypeData(ctxStructs),
		WasmFuncs:   wasmFuncs,
		CtxSnippets: ctxSnippets,
		Body:        indented.String(),
		Helpers:     helpers,
	})
}
