package helpers

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	wasmexec "github.com/felipegenef/gothicframework/v2/pkg/data/wasm_exec"
	wasmruntime "github.com/felipegenef/gothicframework/v2/pkg/wasm"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"
)

// Build pipeline: orchestrates TinyGo compile + wasm-opt + compression for
// page WASMs and topic-manager WASMs, plus the wasm_exec.js copy. Methods
// here run on the host (not inside WASM).

// Template source paths inside the embedded FS (WasmTemplateFS). The CLI no
// longer reads these from the user's project tree — they are an implementation
// detail of the build pipeline and ship inside the binary.
const (
	tmplWasmPageMain     = EmbeddedTmplWasmPageMain
	tmplTopicManagerMain = EmbeddedTmplTopicManagerMain
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
	if err := h.writeModuleBridge(tempModDir); err != nil {
		return err
	}

	genDir, err := os.MkdirTemp(tempModDir, ".gen-")
	if err != nil {
		return fmt.Errorf("wasm: mkdirtemp gen: %w", err)
	}

	mainPath := filepath.Join(genDir, "main.go")
	topicSnippets, topicStructs, topicAliases, topicRefAliases := h.collectTopicSnippets()
	body, err := h.rewriteTopicCalls(page.FuncBody, topicStructs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "wasm: rewrite topic calls %s: %v\n", page.SourceFile, err)
		os.Exit(1)
	}
	if err := h.writeWasmMain(page.SourceFile, body, page.Imports, page.Helpers, topicSnippets, topicStructs, topicAliases, topicRefAliases, mainPath); err != nil {
		return err
	}

	if err := os.MkdirAll(outDir, 0755); err != nil {
		return fmt.Errorf("wasm: mkdir %s: %w", outDir, err)
	}

	absOutFile, err := filepath.Abs(filepath.Join(outDir, page.OutputName+".wasm"))
	if err != nil {
		return err
	}

	pkg := "./" + filepath.Base(genDir) + "/"
	cmd, err := h.buildCommandForCompiler(page.Compiler, pkg, absOutFile, tempModDir)
	if err != nil {
		return err
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := runWithVendorFallback(cmd, tempModDir, func() (*exec.Cmd, error) {
		return h.buildCommandForCompiler(page.Compiler, pkg, absOutFile, tempModDir)
	}); err != nil {
		return fmt.Errorf("wasm: build %s (%s): %w", page.OutputName, compilerLabel(page.Compiler), err)
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

// buildCommandForCompiler returns the exec.Cmd that compiles the WASM package
// using the chosen compiler. tempModDir is the temp module root (contains
// go.mod with module name "wasm-runtime") and pkg is the relative import path
// ("./<genDir>/"). The runtime files use `//go:build js && wasm`, so both
// TinyGo's -target=wasm and standard Go's GOOS=js GOARCH=wasm satisfy them.
func (h *WasmHelper) buildCommandForCompiler(choice WasmCompilerChoice, pkg, absOutFile, tempModDir string) (*exec.Cmd, error) {
	switch choice {
	case WasmCompilerLocalTinyGo:
		tinygo, err := exec.LookPath("tinygo")
		if err != nil {
			return nil, fmt.Errorf("wasm: WasmCompiler=LocalTinyGo but tinygo not found in PATH: %w", err)
		}
		cmd := exec.Command(tinygo,
			"build", "-no-debug", "-opt=z",
			"-o", absOutFile,
			"-target", "wasm",
			"-gc", "conservative",
			pkg,
		)
		cmd.Dir = tempModDir
		cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod")
		return cmd, nil

	case WasmCompilerGolang:
		goExe, err := exec.LookPath("go")
		if err != nil {
			return nil, fmt.Errorf("wasm: WasmCompiler=Golang but go not found in PATH: %w", err)
		}
		cmd := exec.Command(goExe,
			"build",
			"-ldflags=-s -w",
			"-trimpath",
			"-o", absOutFile,
			pkg,
		)
		cmd.Dir = tempModDir
		cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm", "GOWORK=off", "GOFLAGS=-mod=mod")
		return cmd, nil

	default: // WasmCompilerGothicTinyGo
		tinygo := h.TinyGoBinary()
		if h.ConfigOverride != "" {
			tinygo = h.ConfigOverride
		}
		cmd := exec.Command(tinygo,
			"build", "-no-debug", "-opt=z",
			"-o", absOutFile,
			"-target", "wasm",
			"-gc", "conservative",
			pkg,
		)
		cmd.Dir = tempModDir
		cmd.Env = append(os.Environ(), h.Environ()...)
		cmd.Env = append(cmd.Env, "GOWORK=off", "GOFLAGS=-mod=mod")
		return cmd, nil
	}
}

// writeModuleBridge writes a go.mod into tempModDir that links back to the
// user's project (CWD) so imports of user-project packages in the generated
// main.go can resolve. Called once per WASM build (per-page and topic-manager).
func (h *WasmHelper) writeModuleBridge(tempModDir string) error {
	const projectRoot = "."
	modulePath, goVersion, err := ReadUserModulePath(projectRoot)
	if err != nil {
		return fmt.Errorf("wasm: read user module: %w", err)
	}
	if err := WriteBridgeGoMod(tempModDir, modulePath, projectRoot, goVersion); err != nil {
		return fmt.Errorf("wasm: write bridge go.mod: %w", err)
	}
	return nil
}

// runWithVendorFallback runs cmd. If it fails, attempts `go mod vendor` inside
// tempModDir and retries by re-creating the command via rebuild. Returns the
// original error if the fallback also fails.
func runWithVendorFallback(cmd *exec.Cmd, tempModDir string, rebuild func() (*exec.Cmd, error)) error {
	origErr := cmd.Run()
	if origErr == nil {
		return nil
	}
	goExe, lookErr := exec.LookPath("go")
	if lookErr != nil {
		return origErr
	}
	vendor := exec.Command(goExe, "mod", "vendor")
	vendor.Dir = tempModDir
	vendor.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod")
	vendor.Stdout = os.Stderr
	vendor.Stderr = os.Stderr
	if err := vendor.Run(); err != nil {
		return origErr
	}
	retry, err := rebuild()
	if err != nil {
		return origErr
	}
	retry.Stdout = os.Stdout
	retry.Stderr = os.Stderr
	if err := retry.Run(); err != nil {
		return origErr
	}
	return nil
}

func compilerLabel(c WasmCompilerChoice) string {
	switch c {
	case WasmCompilerLocalTinyGo:
		return "local tinygo"
	case WasmCompilerGolang:
		return "go (js/wasm)"
	default:
		return "embedded tinygo"
	}
}

// CountTopicManagers returns the number of topic structs that will produce a
// topic-manager WASM binary (i.e. structs that have a KeyName set).
func (h *WasmHelper) CountTopicManagers() int {
	_, structs, _, _ := h.collectTopicSnippets()
	n := 0
	for _, s := range structs {
		if s.KeyName != "" {
			n++
		}
	}
	return n
}

// pagesUseStandardGo returns true if any page uses the standard Go compiler.
// Such pages need the standard-Go wasm_exec.js, which is incompatible with TinyGo's.
func pagesUseStandardGo(pages []WasmPage) bool {
	for _, p := range pages {
		if p.Compiler == WasmCompilerGolang {
			return true
		}
	}
	return false
}

func (h *WasmHelper) GenerateAll(pages []WasmPage, outDir string) error {
	if err := h.EnsureBinary(); err != nil {
		return err
	}
	// One-time migration: remove any pre-v2.17 on-disk template copies the
	// project was seeded with. They are now embedded in the CLI binary and
	// the on-disk versions would be silently ignored, so we delete them so
	// users notice and so the path can never drift back into use.
	if err := CleanupLegacyTemplates("."); err != nil {
		return err
	}
	if len(pages) == 0 {
		return nil
	}

	if err := os.MkdirAll(outDir, 0755); err != nil {
		return fmt.Errorf("wasm: mkdir %s: %w", outDir, err)
	}

	h.cache = loadWasmCache()

	if err := h.GenerateTopicManagers(outDir); err != nil {
		wasmErrorf("topic manager build failed: %v", err)
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
	if err := h.CopyWasmExec("public"); err != nil {
		return err
	}
	// Pages built with the standard Go compiler need the matching wasm_exec.js
	// from GOROOT (TinyGo's shim is ABI-incompatible). Emit it side-by-side as
	// wasm_exec_go.js so the bootstrap layer can pick the right one.
	if pagesUseStandardGo(pages) {
		if err := h.CopyGoWasmExec("public"); err != nil {
			return err
		}
	}
	return nil
}

// CopyGoWasmExec copies the standard Go wasm_exec.js from GOROOT into destDir
// as wasm_exec_go.js. Tries GOROOT/lib/wasm (Go 1.24+) then GOROOT/misc/wasm
// (older versions).
func (h *WasmHelper) CopyGoWasmExec(destDir string) error {
	out, err := exec.Command("go", "env", "GOROOT").Output()
	if err != nil {
		return fmt.Errorf("wasm: go env GOROOT: %w", err)
	}
	goroot := strings.TrimSpace(string(out))
	candidates := []string{
		filepath.Join(goroot, "lib", "wasm", "wasm_exec.js"),
		filepath.Join(goroot, "misc", "wasm", "wasm_exec.js"),
	}
	var srcPath string
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			srcPath = c
			break
		}
	}
	if srcPath == "" {
		return fmt.Errorf("wasm: could not locate wasm_exec.js under %s (looked in lib/wasm and misc/wasm)", goroot)
	}
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("wasm: read %s: %w", srcPath, err)
	}
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("wasm: mkdir %s: %w", destDir, err)
	}
	dst := filepath.Join(destDir, "wasm_exec_go.js")
	return os.WriteFile(dst, data, 0644)
}

func (h *WasmHelper) CopyWasmExec(destDir string) error {
	dst := filepath.Join(destDir, "wasm_exec.js")
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("wasm: mkdir wasm_exec dir: %w", err)
	}
	return os.WriteFile(dst, wasmexec.Shim, 0644)
}

func (h *WasmHelper) GenerateTopicManagers(outDir string) error {
	snippets, structs, aliases, refAliases := h.collectTopicSnippets()
	if !h.hasTopicStructs(structs) {
		return nil
	}

	if err := os.MkdirAll(outDir, 0755); err != nil {
		return fmt.Errorf("wasm: mkdir %s: %w", outDir, err)
	}
	for _, s := range structs {
		if s.KeyName == "" {
			continue
		}
		if err := h.buildTopicManager(s, snippets, structs, aliases, refAliases, outDir); err != nil {
			return err
		}
	}
	return nil
}

func (h *WasmHelper) buildTopicManager(s structInfo, snippets []string, allStructs []structInfo, aliases map[string]string, refAliases map[string]typeRef, outDir string) error {
	wasmName := "topic-" + s.KeyName
	compression := s.Compression
	var hash string
	if h.cache != nil {
		hash = h.topicManagerInputHash(compression)
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

	tempModDir, err := os.MkdirTemp("", "tinygo-topic-*")
	if err != nil {
		return fmt.Errorf("wasm: mkdirtemp: %w", err)
	}
	defer os.RemoveAll(tempModDir)

	if err := wasmruntime.ExtractRuntime(tempModDir); err != nil {
		return fmt.Errorf("wasm: extract runtime: %w", err)
	}
	if err := h.writeModuleBridge(tempModDir); err != nil {
		return err
	}

	genDir, err := os.MkdirTemp(tempModDir, ".gen-")
	if err != nil {
		return fmt.Errorf("wasm: mkdirtemp gen: %w", err)
	}

	mainPath := filepath.Join(genDir, "main.go")
	codecs, err := h.buildCodecData(allStructs, aliases, refAliases)
	if err != nil {
		return fmt.Errorf("wasm: topic codec: %w", err)
	}
	structNames := make(map[string]bool, len(allStructs))
	for _, st := range allStructs {
		structNames[st.Name] = true
	}
	fields, err := h.buildManagerFieldData(s, structNames, aliases, refAliases)
	if err != nil {
		return fmt.Errorf("wasm: manager fields: %w", err)
	}
	if err := h.Template.UpdateFromTemplateFS(WasmTemplateFS, tmplTopicManagerMain, mainPath, WasmTopicManagerMainData{
		StructName:    s.Name,
		KeyName:       s.KeyName,
		HasTime:       h.hasTimeFields(allStructs),
		Codecs:        codecs,
		TopicSnippets: snippets,
		Fields:        fields,
	}); err != nil {
		return fmt.Errorf("wasm: render topic manager main.go: %w", err)
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
	buildCmd := func() (*exec.Cmd, error) {
		c := exec.Command(tinygo, "build", "-no-debug", "-opt=z", "-o", absOutFile, "-target", "wasm", "-gc", "conservative", pkg)
		c.Dir = tempModDir
		c.Env = append(os.Environ(), h.Environ()...)
		c.Env = append(c.Env, "GOWORK=off", "GOFLAGS=-mod=mod")
		return c, nil
	}
	cmd, _ := buildCmd()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := runWithVendorFallback(cmd, tempModDir, buildCmd); err != nil {
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

func (h *WasmHelper) writeWasmMain(src, body string, stdImports []string, helpers []string, topicSnippets []string, topicStructs []structInfo, aliases map[string]string, refAliases map[string]typeRef, dest string) error {
	codecs, err := h.buildCodecData(topicStructs, aliases, refAliases)
	if err != nil {
		return fmt.Errorf("wasm: codec: %w", err)
	}
	wasmFuncs, err := h.buildWasmTopicFuncData(topicStructs, aliases, refAliases)
	if err != nil {
		return fmt.Errorf("wasm: topic func data: %w", err)
	}
	// Inject "time" import when any topic struct uses time.Time and the page
	// hasn't already imported it from its own source file.
	if h.hasTimeFields(topicStructs) {
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

	return h.Template.UpdateFromTemplateFS(WasmTemplateFS, tmplWasmPageMain, dest, WasmPageMainData{
		SourceFile:  src,
		StdImports:  stdImports,
		Codecs:      codecs,
		KeyVars:     h.buildKeyVarData(topicStructs),
		TopicTypes:    h.buildTopicTypeData(topicStructs),
		WasmFuncs:     wasmFuncs,
		TopicSnippets: topicSnippets,
		Body:        indented.String(),
		Helpers:     helpers,
	})
}
