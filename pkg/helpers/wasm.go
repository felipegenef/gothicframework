package helpers

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"

	"github.com/andybalholm/brotli"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	wasmruntime "github.com/felipegenef/gothicframework/pkg/wasm"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"
)

var ensureBinaryMu sync.Mutex

const tinyGoVersion = "0.41.1"

// Template file paths in the user's project .gothicCli/templates/ directory.
// These are copied there on init and read at build time — same pattern as routes_gen.go.
const (
	tmplContextGen     = ".gothicCli/templates/wasm/context_gen.go"
	tmplWasmPageMain   = ".gothicCli/templates/wasm/wasm_page_main.go"
	tmplCtxManagerMain = ".gothicCli/templates/wasm/wasm_ctx_manager_main.go"
	wasmCachePath      = ".gothicCli/wasm-cache.json"
)

// ─── Build cache ───────────────────────────────────────────────────────────────

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

// ─── Template data structs ─────────────────────────────────────────────────────

// FieldCodec holds pre-computed codec lines for a single struct field.
type FieldCodec struct {
	Name    string
	EncLine string
	DecLine string
}

// StructCodecData holds codec render data for one struct type.
type StructCodecData struct {
	Name   string
	Fields []FieldCodec
}

// KeyVarData holds data for a BinaryKey var declaration.
type KeyVarData struct {
	StructName string
	KeyName    string
}

// CtxFieldData holds data for one field in a context struct.
type CtxFieldData struct {
	Name string
	Type string
}

// CtxTypeData holds data for a context type struct declaration.
type CtxTypeData struct {
	TypeName string
	Fields   []CtxFieldData
}

// WasmCtxFuncData holds data for one WASM-side context constructor + Set method.
type WasmCtxFuncData struct {
	CtorName   string
	TypeName   string
	StructName string
	KeyName    string
	Fields     []CtxFieldData
}

// ServerCtxFuncData holds data for one server-side context stub.
type ServerCtxFuncData struct {
	CtorName   string
	TypeName   string
	StructName string
	Fields     []CtxFieldData
}

// MountFnData holds data for an AddXxxContext() mount function.
type MountFnData struct {
	FuncName         string
	WasmName         string
	Compression      WasmCompression
	CompressionConst string // "routes.GZIP" or "routes.BROTLI" — used in generated Go code
}

// ContextGenData drives context_gen.go.tmpl.
type ContextGenData struct {
	PkgName     string
	HasCtx      bool
	HasTime     bool // true when any struct field has type time.Time
	Codecs      []StructCodecData
	KeyVars     []KeyVarData
	CtxTypes    []CtxTypeData
	ServerFuncs []ServerCtxFuncData
	MountFns    []MountFnData
}

// WasmPageMainData drives wasm_page_main.go.tmpl.
type WasmPageMainData struct {
	SourceFile  string
	StdImports  []string
	Codecs      []StructCodecData
	KeyVars     []KeyVarData
	CtxTypes    []CtxTypeData
	WasmFuncs   []WasmCtxFuncData
	CtxSnippets []string
	Body        string
}

// WasmCtxManagerMainData drives wasm_ctx_manager_main.go.tmpl.
type WasmCtxManagerMainData struct {
	StructName  string
	KeyName     string
	HasTime     bool // true when any struct field has type time.Time
	Codecs      []StructCodecData
	CtxSnippets []string
}

// ─── Internal struct types ─────────────────────────────────────────────────────

type structInfo struct {
	Name        string
	KeyName     string
	Compression WasmCompression
	Fields      []fieldInfo
}

type fieldInfo struct {
	Name      string
	Type      string
	GothicTag string
}

// ─── WasmHelper ───────────────────────────────────────────────────────────────

// WasmHelper manages the TinyGo toolchain and compiles WASM pages.
// It follows the same struct + method pattern as TailwindHelper and FileBasedRouteHelper.
type WasmHelper struct {
	Template       TemplateHelper
	Runtime        string
	Arch           string
	Version        string
	ConfigOverride string
	cache          *wasmCache
}

// WasmCompression is the compression algorithm for compiled WASM output.
// Mirrors routes.CompressionMethod to avoid a circular import with the helpers/routes package.
type WasmCompression int

const (
	WasmCompressionGzip   WasmCompression = iota // default (routes.GZIP == 0)
	WasmCompressionBrotli WasmCompression = iota // routes.BROTLI == 1
)

// WasmPage describes a single page that has a WASM state function.
type WasmPage struct {
	SourceFile  string
	FuncName    string
	FuncBody    string
	Imports     []string
	HttpPath    string
	OutputName  string
	Compression WasmCompression
}

func NewWasmHelper(goos, goarch string) WasmHelper {
	return WasmHelper{
		Template: NewTemplateHelper(),
		Runtime:  goos,
		Arch:     goarch,
		Version:  tinyGoVersion,
	}
}

// ─── Input hash computation ───────────────────────────────────────────────────

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

// ─── Binary resolution ────────────────────────────────────────────────────────

func (h *WasmHelper) binaryName() (string, error) {
	key := h.Runtime + "/" + h.Arch
	names := map[string]string{
		"linux/amd64":   fmt.Sprintf("tinygo%s.linux-amd64.tar.gz", h.Version),
		"linux/arm64":   fmt.Sprintf("tinygo%s.linux-arm64.tar.gz", h.Version),
		"darwin/amd64":  fmt.Sprintf("tinygo%s.darwin-amd64.tar.gz", h.Version),
		"darwin/arm64":  fmt.Sprintf("tinygo%s.darwin-arm64.tar.gz", h.Version),
		"windows/amd64": fmt.Sprintf("tinygo%s.windows-amd64.zip", h.Version),
	}
	name, ok := names[key]
	if !ok {
		return "", fmt.Errorf("unsupported platform %s/%s for TinyGo", h.Runtime, h.Arch)
	}
	return name, nil
}

func (h *WasmHelper) cacheDir() (string, error) {
	base := os.Getenv("GOTHIC_CLI_CACHE_DIR")
	if base == "" {
		var err error
		base, err = os.UserCacheDir()
		if err != nil {
			return "", fmt.Errorf("failed to determine user cache directory: %w", err)
		}
	}
	return filepath.Join(base, "gothic-cli", "tinygo"), nil
}

func (h *WasmHelper) TinyGoRoot() string {
	dir, err := h.cacheDir()
	if err != nil {
		return ""
	}
	platform := h.Runtime + "-" + h.Arch
	return filepath.Join(dir, "tinygo-"+h.Version, platform, "tinygo")
}

func (h *WasmHelper) TinyGoBinary() string {
	name := "tinygo"
	if h.Runtime == "windows" {
		name += ".exe"
	}
	return filepath.Join(h.TinyGoRoot(), "bin", name)
}

func (h *WasmHelper) Environ() []string {
	root := h.TinyGoRoot()
	binDir := filepath.Join(root, "bin")
	return []string{
		"TINYGOROOT=" + root,
		"PATH=" + binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
	}
}

func (h *WasmHelper) EnsureBinary() error {
	if h.ConfigOverride != "" {
		if _, err := os.Stat(h.ConfigOverride); err != nil {
			return fmt.Errorf("wasm binary override not found at %q: %w", h.ConfigOverride, err)
		}
		return nil
	}

	if _, err := os.Stat(h.TinyGoBinary()); err == nil {
		return nil
	}

	ensureBinaryMu.Lock()
	defer ensureBinaryMu.Unlock()

	if _, err := os.Stat(h.TinyGoBinary()); err == nil {
		return nil
	}

	archiveName, err := h.binaryName()
	if err != nil {
		return err
	}

	dir, err := h.cacheDir()
	if err != nil {
		return err
	}

	archiveURL := fmt.Sprintf(
		"https://github.com/tinygo-org/tinygo/releases/download/v%s/%s",
		h.Version, archiveName,
	)
	checksumURL := fmt.Sprintf(
		"https://github.com/tinygo-org/tinygo/releases/download/v%s/checksums.txt",
		h.Version,
	)

	fmt.Fprintf(os.Stderr, "wasm: TinyGo %s not found — downloading for %s/%s...\n",
		h.Version, h.Runtime, h.Arch)

	expected, checksumErr := h.fetchExpectedChecksum(checksumURL, archiveName)
	if checksumErr != nil {
		fmt.Fprintf(os.Stderr, "wasm: WARNING — checksums.txt unavailable (%v); proceeding without pre-verification\n", checksumErr)
	}

	tmpArchive, err := h.downloadToTemp(archiveURL)
	if err != nil {
		return fmt.Errorf("wasm: download TinyGo: %w", err)
	}
	defer os.Remove(tmpArchive)

	if expected != "" {
		if err := h.verifyChecksum(tmpArchive, expected); err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "wasm: checksum OK")
	} else {
		if digest, err := h.computeChecksum(tmpArchive); err == nil {
			platform := h.Runtime + "-" + h.Arch
			localChecksum := filepath.Join(dir, "tinygo-"+h.Version, platform+".sha256")
			_ = os.MkdirAll(filepath.Dir(localChecksum), 0755)
			_ = os.WriteFile(localChecksum, []byte(digest), 0644)
		}
	}

	platform := h.Runtime + "-" + h.Arch
	finalDir := filepath.Join(dir, "tinygo-"+h.Version, platform)
	tmpDir := finalDir + ".tmp"

	os.RemoveAll(tmpDir)
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return fmt.Errorf("wasm: mkdir: %w", err)
	}

	fmt.Fprintln(os.Stderr, "wasm: extracting TinyGo toolchain...")
	if err := h.extractArchive(tmpArchive, tmpDir); err != nil {
		os.RemoveAll(tmpDir)
		return fmt.Errorf("wasm: extract: %w", err)
	}

	os.RemoveAll(finalDir)
	if err := os.Rename(tmpDir, finalDir); err != nil {
		os.RemoveAll(tmpDir)
		return fmt.Errorf("wasm: install: %w", err)
	}

	fmt.Fprintf(os.Stderr, "wasm: TinyGo %s ready at %s\n", h.Version, h.TinyGoRoot())
	return nil
}

// ─── Page scanning ─────────────────────────────────────────────────────────────

var pageStateInlineRe = regexp.MustCompile(`(?m)ClientSideState:\s*func\s*\(\s*\)\s*\{`)
var pageStateNamedRe = regexp.MustCompile(`(?m)ClientSideState:\s*(\w+)`)
var wasmCompressionRe = regexp.MustCompile(`(?m)WasmCompression:\s*routes\.(\w+)`)

func (h *WasmHelper) ScanPages(pagesDir, componentsDir string) ([]WasmPage, error) {
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
	raw, err := os.ReadFile(path)
	if err != nil {
		return WasmPage{}, false, fmt.Errorf("read %s: %w", path, err)
	}
	content := string(raw)

	var body string

	if loc := pageStateInlineRe.FindStringIndex(content); loc != nil {
		openBrace := loc[1] - 1
		body = h.extractFuncBody(content, openBrace)
		if body == "" {
			return WasmPage{}, false, fmt.Errorf("wasm: could not extract inline ClientSideState body in %s", path)
		}
	} else if m := pageStateNamedRe.FindStringSubmatch(content); m != nil {
		funcName := m[1]
		funcRe := regexp.MustCompile(`(?m)^func\s+` + regexp.QuoteMeta(funcName) + `\s*\(\s*\)\s*\{`)
		floc := funcRe.FindStringIndex(content)
		if floc == nil {
			return WasmPage{}, false, fmt.Errorf("wasm: ClientSideState func %s not found in %s", funcName, path)
		}
		openBrace := floc[1] - 1
		body = h.extractFuncBody(content, openBrace)
		if body == "" {
			return WasmPage{}, false, fmt.Errorf("wasm: could not extract body of %s in %s", funcName, path)
		}
	} else {
		return WasmPage{}, false, nil
	}

	stdImports := h.filterStdImports(content, body)
	httpPath := h.normalizeWasmHttpPath(path)
	outputName := h.wasmOutputName(httpPath)

	compression := WasmCompressionGzip
	if m := wasmCompressionRe.FindStringSubmatch(content); m != nil && m[1] == "BROTLI" {
		compression = WasmCompressionBrotli
	}

	return WasmPage{
		SourceFile:  path,
		FuncBody:    body,
		Imports:     stdImports,
		HttpPath:    httpPath,
		OutputName:  outputName,
		Compression: compression,
	}, true, nil
}

// ─── Build pipeline ────────────────────────────────────────────────────────────

func (h *WasmHelper) GeneratePage(page WasmPage, outDir string) error {
	compressedExt := compressionExt(page.Compression)
	var hash string
	if h.cache != nil {
		hash = h.pageInputHash(page)
		outPath := filepath.Join(outDir, page.OutputName+".wasm"+compressedExt)
		if h.cache.upToDate(page.OutputName, hash) {
			if _, err := os.Stat(outPath); err == nil {
				fmt.Printf("wasm: %s → up to date\n", page.OutputName)
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
	if err := h.writeWasmMain(page.SourceFile, body, page.Imports, ctxSnippets, ctxStructs, ctxAliases, mainPath); err != nil {
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
	fmt.Printf("wasm: %s → %s → %s (%s)\n",
		page.OutputName, h.formatBytes(wasmSize), h.formatBytes(finalSize), compressionLabel(page.Compression))
	if hash != "" {
		h.cache.update(page.OutputName, hash)
	}
	return nil
}

func (h *WasmHelper) GenerateAll(pages []WasmPage, outDir string) error {
	if err := h.EnsureBinary(); err != nil {
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
		fmt.Fprintf(os.Stderr, "wasm: context manager build failed: %v\n", err)
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
	src := filepath.Join(h.TinyGoRoot(), "targets", "wasm_exec.js")
	dst := filepath.Join(destDir, "wasm_exec.js")

	srcInfo, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("wasm: wasm_exec.js not found at %s: %w", src, err)
	}

	if dstInfo, err := os.Stat(dst); err == nil {
		if dstInfo.Size() == srcInfo.Size() {
			return nil
		}
	}

	in, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("wasm: read wasm_exec.js: %w", err)
	}
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}
	return os.WriteFile(dst, in, 0644)
}

// ─── Context manager build ─────────────────────────────────────────────────────

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
				fmt.Printf("wasm: %s → up to date\n", wasmName)
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
	if err := h.Template.UpdateFromTemplate(tmplCtxManagerMain, mainPath, WasmCtxManagerMainData{
		StructName:  s.Name,
		KeyName:     s.KeyName,
		HasTime:     h.hasTimeFields(allStructs),
		Codecs:      codecs,
		CtxSnippets: snippets,
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
	cmd := exec.Command(tinygo, "build", "-no-debug", "-opt=z", "-o", absOutFile, "-target", "wasm", pkg)
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
	fmt.Printf("wasm: %s → %s → %s (%s)\n", wasmName, h.formatBytes(wasmSize), h.formatBytes(compSize), compressionLabel(compression))
	if hash != "" {
		h.cache.update(wasmName, hash)
	}
	return nil
}

// ─── WASM main.go generation ──────────────────────────────────────────────────

func (h *WasmHelper) writeWasmMain(src, body string, stdImports []string, ctxSnippets []string, ctxStructs []structInfo, aliases map[string]string, dest string) error {
	codecs, err := h.buildCodecData(ctxStructs, aliases)
	if err != nil {
		return fmt.Errorf("wasm: codec: %w", err)
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
		WasmFuncs:   h.buildWasmCtxFuncData(ctxStructs),
		CtxSnippets: ctxSnippets,
		Body:        indented.String(),
	})
}

// ─── Context snippet collection ───────────────────────────────────────────────

var importBlockRe = regexp.MustCompile(`(?s)import\s*\([^)]*\)|import\s+(?:\.\s+|[\w]+\s+)?"[^"]+"`)
var pkgDeclRe = regexp.MustCompile(`(?m)^package\s+\S+.*\n?`)
var autoKeyRe = regexp.MustCompile(`AutoKey\[(\[\][\w]+|[\w]+)\]\("([^"]+)"\)`)

// collectContextSnippets reads src/context/*.go, parses struct definitions,
// generates context_gen.go (server side), and returns inlinable user code
// snippets and the parsed structs for template rendering.
func (h *WasmHelper) collectContextSnippets() (snippets []string, structs []structInfo, aliases map[string]string) {
	entries, err := os.ReadDir("src/context")
	if err != nil {
		return nil, nil, nil
	}

	type rawFile struct{ name, src string }
	var files []rawFile
	var allStructs []structInfo
	allAliases := make(map[string]string)
	pkgName := "gothicwasm"

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || e.Name() == "context_gen.go" {
			continue
		}
		data, err := os.ReadFile(filepath.Join("src/context", e.Name()))
		if err != nil {
			continue
		}
		src := string(data)
		if fset := token.NewFileSet(); pkgName == "gothicwasm" {
			if pf, err := parser.ParseFile(fset, "", src, 0); err == nil && pf.Name != nil {
				pkgName = pf.Name.Name
			}
		}
		structs, aliases := h.parseStructsFromSource(src)
		allStructs = append(allStructs, structs...)
		for k, v := range aliases {
			allAliases[k] = v
		}
		files = append(files, rawFile{e.Name(), src})
	}

	seenKeys := map[string]string{}
	for _, s := range allStructs {
		if s.KeyName == "" {
			continue
		}
		if prev, exists := seenKeys[s.KeyName]; exists {
			fmt.Fprintf(os.Stderr,
				"error: duplicate context key name %q — used by both %s and %s in src/context/.\n"+
					"  Each context struct must have a unique key name.\n",
				s.KeyName, prev, s.Name)
			os.Exit(1)
		}
		seenKeys[s.KeyName] = s.Name
	}

	h.writeContextKeyStubs(allStructs, allAliases, pkgName)

	for _, f := range files {
		src := f.src
		src = pkgDeclRe.ReplaceAllLiteralString(src, "")
		src = importBlockRe.ReplaceAllLiteralString(src, "")
		src = h.rewriteAutoKeys(src)
		src = strings.TrimSpace(src)
		if src != "" {
			snippets = append(snippets, "// --- from src/context/"+f.name+" ---\n"+src)
		}
	}
	return snippets, allStructs, allAliases
}

func (h *WasmHelper) writeContextKeyStubs(structs []structInfo, aliases map[string]string, pkgName string) {
	if len(structs) == 0 {
		_ = os.Remove("src/context/context_gen.go")
		return
	}

	codecs, err := h.buildCodecData(structs, aliases)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: context codec: %v\n", err)
		os.Exit(1)
	}

	data := ContextGenData{
		PkgName:     pkgName,
		HasCtx:      h.hasCtxStructs(structs),
		HasTime:     h.hasTimeFields(structs),
		Codecs:      codecs,
		KeyVars:     h.buildKeyVarData(structs),
		CtxTypes:    h.buildCtxTypeData(structs),
		ServerFuncs: h.buildServerCtxFuncData(structs),
		MountFns:    h.buildMountFnData(structs),
	}

	_ = h.Template.UpdateFromTemplate(tmplContextGen, "src/context/context_gen.go", data)
}

// ─── Data struct builders ─────────────────────────────────────────────────────

func (h *WasmHelper) hasCtxStructs(structs []structInfo) bool {
	for _, s := range structs {
		if s.KeyName != "" {
			return true
		}
	}
	return false
}

func (h *WasmHelper) hasTimeFields(structs []structInfo) bool {
	for _, s := range structs {
		for _, f := range s.Fields {
			if f.Type == "time.Time" {
				return true
			}
		}
	}
	return false
}

func (h *WasmHelper) buildCodecData(structs []structInfo, aliases map[string]string) ([]StructCodecData, error) {
	names := make(map[string]bool, len(structs))
	for _, s := range structs {
		names[s.Name] = true
	}
	result := make([]StructCodecData, 0, len(structs))
	for _, s := range structs {
		sd := StructCodecData{Name: s.Name}
		for _, f := range s.Fields {
			enc, dec, err := h.codecLines(f, names, aliases)
			if err != nil {
				return nil, fmt.Errorf("struct %s field %s: %w", s.Name, f.Name, err)
			}
			sd.Fields = append(sd.Fields, FieldCodec{Name: f.Name, EncLine: enc, DecLine: dec})
		}
		result = append(result, sd)
	}
	return result, nil
}

func (h *WasmHelper) buildKeyVarData(structs []structInfo) []KeyVarData {
	var result []KeyVarData
	for _, s := range structs {
		if s.KeyName == "" {
			continue
		}
		result = append(result, KeyVarData{StructName: s.Name, KeyName: s.KeyName})
	}
	return result
}

func (h *WasmHelper) buildCtxTypeData(structs []structInfo) []CtxTypeData {
	var result []CtxTypeData
	for _, s := range structs {
		if s.KeyName == "" {
			continue
		}
		td := CtxTypeData{TypeName: h.ctxTypeName(s.Name)}
		for _, f := range s.Fields {
			td.Fields = append(td.Fields, CtxFieldData{Name: f.Name, Type: f.Type})
		}
		result = append(result, td)
	}
	return result
}

func (h *WasmHelper) buildWasmCtxFuncData(structs []structInfo) []WasmCtxFuncData {
	var result []WasmCtxFuncData
	for _, s := range structs {
		if s.KeyName == "" {
			continue
		}
		fd := WasmCtxFuncData{
			CtorName:   h.ctxFuncName(s.Name),
			TypeName:   h.ctxTypeName(s.Name),
			StructName: s.Name,
			KeyName:    s.KeyName,
		}
		for _, f := range s.Fields {
			fd.Fields = append(fd.Fields, CtxFieldData{Name: f.Name, Type: f.Type})
		}
		result = append(result, fd)
	}
	return result
}

func (h *WasmHelper) buildServerCtxFuncData(structs []structInfo) []ServerCtxFuncData {
	var result []ServerCtxFuncData
	for _, s := range structs {
		if s.KeyName == "" {
			continue
		}
		fd := ServerCtxFuncData{
			CtorName:   h.ctxFuncName(s.Name),
			TypeName:   h.ctxTypeName(s.Name),
			StructName: s.Name,
		}
		for _, f := range s.Fields {
			fd.Fields = append(fd.Fields, CtxFieldData{Name: f.Name, Type: f.Type})
		}
		result = append(result, fd)
	}
	return result
}

func (h *WasmHelper) buildMountFnData(structs []structInfo) []MountFnData {
	var result []MountFnData
	for _, s := range structs {
		if s.KeyName == "" {
			continue
		}
		compressionConst := "routes.GZIP"
		if s.Compression == WasmCompressionBrotli {
			compressionConst = "routes.BROTLI"
		}
		result = append(result, MountFnData{
			FuncName:         "Add" + h.ctxFuncName(s.Name),
			WasmName:         "ctx-" + s.KeyName,
			Compression:      s.Compression,
			CompressionConst: compressionConst,
		})
	}
	return result
}

// ─── Context naming helpers ───────────────────────────────────────────────────

func (h *WasmHelper) ctxTypeName(structName string) string {
	return strings.ToLower(structName[:1]) + structName[1:] + "Context"
}

func (h *WasmHelper) ctxFuncName(structName string) string { return structName + "Context" }

// ─── Struct parsing ───────────────────────────────────────────────────────────

// parseStructsFromSource parses struct definitions and type aliases from a Go source string.
// typeAliases maps alias name → underlying type string (e.g. "MyInt" → "int").
func (h *WasmHelper) parseStructsFromSource(src string) (structs []structInfo, typeAliases map[string]string) {
	typeAliases = make(map[string]string)
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", src, parser.ParseComments)
	if err != nil {
		return nil, typeAliases
	}
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gd.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			switch t := ts.Type.(type) {
			case *ast.Ident:
				// type MyInt int  — record the alias
				typeAliases[ts.Name.Name] = t.Name
			case *ast.ArrayType, *ast.MapType, *ast.StarExpr:
				// type Labels []string, type MyMap map[K]V, type MyPtr *T
				if s := h.astTypeString(ts.Type); s != "" {
					typeAliases[ts.Name.Name] = s
				}
				_ = t
			case *ast.StructType:
				si := structInfo{Name: ts.Name.Name}
				for _, field := range t.Fields.List {
					typ := h.astTypeString(field.Type)
					tag := ""
					if field.Tag != nil {
						tag = h.parseGothicTag(strings.Trim(field.Tag.Value, "`"))
					}
					if len(field.Names) == 0 && typ == "GothicSharedContext" {
						if field.Tag != nil {
							raw := strings.Trim(field.Tag.Value, "`")
							si.KeyName = h.parseNameTag(raw)
							si.Compression = h.parseCompressionTag(raw)
						}
						continue
					}
					for _, name := range field.Names {
						si.Fields = append(si.Fields, fieldInfo{Name: name.Name, Type: typ, GothicTag: tag})
					}
				}
				structs = append(structs, si)
			}
		}
	}
	return structs, typeAliases
}

func (h *WasmHelper) astTypeString(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.ArrayType:
		if e.Len == nil {
			return "[]" + h.astTypeString(e.Elt)
		}
		return h.astTypeString(e.Elt)
	case *ast.StarExpr:
		return "*" + h.astTypeString(e.X)
	case *ast.SelectorExpr:
		return h.astTypeString(e.X) + "." + e.Sel.Name
	case *ast.MapType:
		return "map[" + h.astTypeString(e.Key) + "]" + h.astTypeString(e.Value)
	}
	return ""
}

func (h *WasmHelper) parseGothicTag(tag string) string {
	for _, part := range strings.Fields(tag) {
		if strings.HasPrefix(part, `gothic:"`) {
			return strings.Trim(strings.TrimPrefix(part, "gothic:"), `"`)
		}
	}
	return ""
}

func (h *WasmHelper) parseNameTag(tag string) string {
	for _, part := range strings.Fields(tag) {
		if strings.HasPrefix(part, `name:"`) {
			return strings.Trim(strings.TrimPrefix(part, "name:"), `"`)
		}
	}
	return ""
}

func (h *WasmHelper) parseCompressionTag(tag string) WasmCompression {
	for _, part := range strings.Fields(tag) {
		if strings.HasPrefix(part, `compression:"`) {
			val := strings.Trim(strings.TrimPrefix(part, "compression:"), `"`)
			if strings.EqualFold(val, "brotli") {
				return WasmCompressionBrotli
			}
		}
	}
	return WasmCompressionGzip
}

// ─── Codec computation ────────────────────────────────────────────────────────

// primitiveCodec returns the encode/decode expressions for a single primitive value
// referenced by variable name varExpr (e.g. "v.Field" or "_item").
// Returns ("", "") if the type is not a known primitive.
func primitiveCodec(typ, varExpr string) (enc, dec string) {
	switch typ {
	case "bool":
		return fmt.Sprintf("e.Bool(bool(%s))", varExpr), fmt.Sprintf("%s = bool(d.Bool())", varExpr)
	case "string":
		return fmt.Sprintf("e.String(string(%s))", varExpr), fmt.Sprintf("%s = string(d.String())", varExpr)
	case "int":
		return fmt.Sprintf("e.I64(int64(%s))", varExpr), fmt.Sprintf("%s = int(d.I64())", varExpr)
	case "int8":
		return fmt.Sprintf("e.I32(int32(%s))", varExpr), fmt.Sprintf("%s = int8(d.I32())", varExpr)
	case "int16":
		return fmt.Sprintf("e.I32(int32(%s))", varExpr), fmt.Sprintf("%s = int16(d.I32())", varExpr)
	case "int32", "rune":
		return fmt.Sprintf("e.I32(int32(%s))", varExpr), fmt.Sprintf("%s = int32(d.I32())", varExpr)
	case "int64":
		return fmt.Sprintf("e.I64(int64(%s))", varExpr), fmt.Sprintf("%s = int64(d.I64())", varExpr)
	case "uint8", "byte":
		return fmt.Sprintf("e.U8(uint8(%s))", varExpr), fmt.Sprintf("%s = uint8(d.U8())", varExpr)
	case "uint16":
		return fmt.Sprintf("e.U16(uint16(%s))", varExpr), fmt.Sprintf("%s = uint16(d.U16())", varExpr)
	case "uint32":
		return fmt.Sprintf("e.U32(uint32(%s))", varExpr), fmt.Sprintf("%s = uint32(d.U32())", varExpr)
	case "uint":
		return fmt.Sprintf("e.U64(uint64(%s))", varExpr), fmt.Sprintf("%s = uint(d.U64())", varExpr)
	case "uint64":
		return fmt.Sprintf("e.U64(uint64(%s))", varExpr), fmt.Sprintf("%s = uint64(d.U64())", varExpr)
	case "float32":
		return fmt.Sprintf("e.F32(float32(%s))", varExpr), fmt.Sprintf("%s = float32(d.F32())", varExpr)
	case "float64":
		return fmt.Sprintf("e.F64(float64(%s))", varExpr), fmt.Sprintf("%s = float64(d.F64())", varExpr)
	case "time.Time":
		return fmt.Sprintf("e.I64(%s.UnixNano())", varExpr),
			fmt.Sprintf("%s = time.Unix(0, d.I64())", varExpr)
	}
	return "", ""
}

func (h *WasmHelper) codecLines(fi fieldInfo, structNames map[string]bool, aliases map[string]string) (enc, dec string, err error) {
	n := fi.Name
	typ := fi.Type
	tag := fi.GothicTag

	if tag == "skip" {
		return "", "", nil
	}

	// Explicit gothic tag overrides — only valid on int/uint fields.
	if tag != "" {
		switch tag {
		case "i32":
			return fmt.Sprintf("e.I32(int32(v.%s))", n), fmt.Sprintf("v.%s = int(d.I32())", n), nil
		case "i64":
			return fmt.Sprintf("e.I64(int64(v.%s))", n), fmt.Sprintf("v.%s = int(d.I64())", n), nil
		case "u32":
			return fmt.Sprintf("e.U32(uint32(v.%s))", n), fmt.Sprintf("v.%s = uint(d.U32())", n), nil
		case "u64":
			return fmt.Sprintf("e.U64(uint64(v.%s))", n), fmt.Sprintf("v.%s = uint(d.U64())", n), nil
		default:
			return "", "", fmt.Errorf("unknown gothic tag %q (valid: skip, i32, i64, u32, u64)", tag)
		}
	}

	// Resolve type alias to its underlying type for codec selection, while keeping
	// the original type name for cast expressions in the generated code.
	resolvedTyp := typ
	if underlying, ok := aliases[typ]; ok {
		resolvedTyp = underlying
	}

	// Use the resolved type for pattern matching (handles complex type aliases like
	// `type Labels []string`, `type MyData []byte`, `type MyItem Item`).
	effectiveTyp := typ
	if resolvedTyp != typ {
		effectiveTyp = resolvedTyp
	}

	// Special case: []byte is a single Bytes call, not a slice loop.
	// Check both original and resolved type so `type MyData []byte` also hits this path.
	if effectiveTyp == "[]byte" {
		return fmt.Sprintf("e.Bytes(v.%s)", n), fmt.Sprintf("v.%s = d.Bytes()", n), nil
	}

	// Primitive field.
	if pe, pd := primitiveCodec(resolvedTyp, "v."+n); pe != "" {
		if resolvedTyp != typ {
			// Type alias — fix decode to cast back to the alias type.
			pd = fmt.Sprintf("{ _v := %s; v.%s = %s(_v) }", strings.Replace(pd, "v."+n+" = ", "", 1), n, typ)
		}
		return pe, pd, nil
	}

	// Slice field.
	if strings.HasPrefix(effectiveTyp, "[]") {
		elem := effectiveTyp[2:]
		return h.sliceCodecLines(n, elem, structNames, aliases)
	}

	// Map field: map[K]V where K is a primitive and V is a primitive or known struct.
	if strings.HasPrefix(effectiveTyp, "map[") {
		return h.mapCodecLines(n, effectiveTyp, structNames, aliases)
	}

	// Pointer field: *T
	if strings.HasPrefix(effectiveTyp, "*") {
		return h.pointerCodecLines(n, effectiveTyp[1:], structNames, aliases)
	}

	// Known struct defined in src/context/ — check both original and resolved name so
	// `type MyItem Item` (a struct-type alias) also dispatches to the right encoder.
	structTyp := typ
	if !structNames[typ] && structNames[effectiveTyp] {
		structTyp = effectiveTyp
	}
	if structNames[structTyp] {
		return fmt.Sprintf("_encode_%s(v.%s, e)", structTyp, n),
			fmt.Sprintf("v.%s = _decode_%s(d)", n, structTyp), nil
	}

	return "", "", fmt.Errorf(
		"unsupported type %q — supported: primitives, []T, map[K]V, *T, time.Time, and structs defined in src/context/\n"+
			"  Tip: add `gothic:\"skip\"` to exclude this field from the context wire format",
		typ,
	)
}

func (h *WasmHelper) sliceCodecLines(fieldName, elem string, structNames map[string]bool, aliases map[string]string) (enc, dec string, err error) {
	// [][]T — nested slice
	if strings.HasPrefix(elem, "[]") {
		inner := elem[2:]
		innerEnc, innerDec, err := h.sliceCodecLines("_inner", inner, structNames, aliases)
		if err != nil {
			return "", "", fmt.Errorf("[]%s: %w", elem, err)
		}
		// innerEnc/Dec reference "_inner" — wrap in a helper closure inline
		enc = fmt.Sprintf(
			"{ e.U32(uint32(len(v.%s))); for _, _row := range v.%s { var _inner []%s; _ = _inner; %s } }",
			fieldName, fieldName, inner, strings.ReplaceAll(innerEnc, "v._inner", "_row"))
		dec = fmt.Sprintf(
			"{ _n := int(d.U32()); v.%s = make([][]%s, _n); for _i := range v.%s { var _inner []%s; %s; v.%s[_i] = _inner } }",
			fieldName, inner, fieldName, inner,
			strings.ReplaceAll(innerDec, "v._inner", "_inner"),
			fieldName)
		return enc, dec, nil
	}

	// []KnownStruct
	if structNames[elem] {
		enc = fmt.Sprintf(
			"{ e.U32(uint32(len(v.%s))); for _, _item := range v.%s { _encode_%s(_item, e) } }",
			fieldName, fieldName, elem)
		dec = fmt.Sprintf(
			"{ _n := int(d.U32()); v.%s = make([]%s, _n); for _i := range v.%s { v.%s[_i] = _decode_%s(d) } }",
			fieldName, elem, fieldName, fieldName, elem)
		return enc, dec, nil
	}

	// []primitive (including type aliases over primitives)
	resolvedElem := elem
	if underlying, ok := aliases[elem]; ok {
		resolvedElem = underlying
	}
	if pe, pd := primitiveCodec(resolvedElem, "_item"); pe != "" {
		if resolvedElem != elem {
			// Decode: primitiveCodec returns the underlying type; cast back to the alias.
			rhs := strings.TrimPrefix(pd, "_item = ")
			pd = fmt.Sprintf("_item = %s(%s)", elem, rhs)
		}
		enc = fmt.Sprintf(
			"{ e.U32(uint32(len(v.%s))); for _, _item := range v.%s { %s } }",
			fieldName, fieldName, pe)
		dec = fmt.Sprintf(
			"{ _n := int(d.U32()); v.%s = make([]%s, _n); for _i := range v.%s { var _item %s; %s; v.%s[_i] = _item } }",
			fieldName, elem, fieldName, elem, pd, fieldName)
		return enc, dec, nil
	}

	// []*T — slice of pointers (nil tag byte + value), covering both primitive/alias and struct bases.
	if strings.HasPrefix(elem, "*") {
		base := elem[1:]
		resolvedBase := base
		if underlying, ok := aliases[base]; ok {
			resolvedBase = underlying
		}

		if pe, pd := primitiveCodec(resolvedBase, "_pv"); pe != "" {
			if resolvedBase != base {
				rhs := strings.TrimPrefix(pd, "_pv = ")
				pd = fmt.Sprintf("_pv = %s(%s)", base, rhs)
			}
			itemEnc := fmt.Sprintf("if _item == nil { e.U8(0) } else { e.U8(1); _pv := *_item; %s }", pe)
			itemDec := fmt.Sprintf("if d.U8() != 0 { var _pv %s; %s; v.%s[_i] = &_pv }", base, pd, fieldName)
			enc = fmt.Sprintf(
				"{ e.U32(uint32(len(v.%s))); for _, _item := range v.%s { %s } }",
				fieldName, fieldName, itemEnc)
			dec = fmt.Sprintf(
				"{ _n := int(d.U32()); v.%s = make([]%s, _n); for _i := range v.%s { %s } }",
				fieldName, elem, fieldName, itemDec)
			return enc, dec, nil
		}

		structT := base
		if !structNames[base] && structNames[resolvedBase] {
			structT = resolvedBase
		}
		if structNames[structT] {
			itemEnc := fmt.Sprintf("if _item == nil { e.U8(0) } else { e.U8(1); _encode_%s(*_item, e) }", structT)
			itemDec := fmt.Sprintf("if d.U8() != 0 { _sv := _decode_%s(d); v.%s[_i] = &_sv }", structT, fieldName)
			enc = fmt.Sprintf(
				"{ e.U32(uint32(len(v.%s))); for _, _item := range v.%s { %s } }",
				fieldName, fieldName, itemEnc)
			dec = fmt.Sprintf(
				"{ _n := int(d.U32()); v.%s = make([]%s, _n); for _i := range v.%s { %s } }",
				fieldName, elem, fieldName, itemDec)
			return enc, dec, nil
		}

		return "", "", fmt.Errorf("slice element pointer type %q base type %q is not a supported primitive or known struct", elem, base)
	}

	return "", "", fmt.Errorf("slice element type %q is not supported", elem)
}

func (h *WasmHelper) mapCodecLines(fieldName, typ string, structNames map[string]bool, aliases map[string]string) (enc, dec string, err error) {
	// Parse "map[K]V"
	inner := typ[4:] // strip "map["
	bracket := strings.Index(inner, "]")
	if bracket < 0 {
		return "", "", fmt.Errorf("malformed map type %q", typ)
	}
	keyTyp := inner[:bracket]
	valTyp := inner[bracket+1:]

	resolvedKeyTyp := keyTyp
	if underlying, ok := aliases[keyTyp]; ok {
		resolvedKeyTyp = underlying
	}
	resolvedValTyp := valTyp
	if underlying, ok := aliases[valTyp]; ok {
		resolvedValTyp = underlying
	}

	keyEnc, keyDec := primitiveCodec(resolvedKeyTyp, "_k")
	if keyEnc == "" {
		return "", "", fmt.Errorf("map key type %q is not a supported primitive", keyTyp)
	}
	if resolvedKeyTyp != keyTyp {
		rhs := strings.TrimPrefix(keyDec, "_k = ")
		keyDec = fmt.Sprintf("_k = %s(%s)", keyTyp, rhs)
	}

	var valEnc, valDec string
	if ve, vd := primitiveCodec(resolvedValTyp, "_v"); ve != "" {
		valEnc, valDec = ve, vd
		if resolvedValTyp != valTyp {
			rhs := strings.TrimPrefix(valDec, "_v = ")
			valDec = fmt.Sprintf("_v = %s(%s)", valTyp, rhs)
		}
	} else if strings.HasPrefix(valTyp, "*") {
		// map[K]*V — pointer value: nil tag byte + encoded value.
		baseVal := valTyp[1:]
		resolvedBaseVal := baseVal
		if underlying, ok := aliases[baseVal]; ok {
			resolvedBaseVal = underlying
		}
		if pe, pd := primitiveCodec(resolvedBaseVal, "_pv"); pe != "" {
			if resolvedBaseVal != baseVal {
				rhs := strings.TrimPrefix(pd, "_pv = ")
				pd = fmt.Sprintf("_pv = %s(%s)", baseVal, rhs)
			}
			valEnc = fmt.Sprintf("{ if _v == nil { e.U8(0) } else { e.U8(1); _pv := *_v; %s } }", pe)
			valDec = fmt.Sprintf("if d.U8() != 0 { var _pv %s; %s; _v = &_pv }", baseVal, pd)
		} else {
			structT := baseVal
			if !structNames[baseVal] && structNames[resolvedBaseVal] {
				structT = resolvedBaseVal
			}
			if !structNames[structT] {
				return "", "", fmt.Errorf("map value pointer type %q base type is not a supported primitive or known struct", valTyp)
			}
			valEnc = fmt.Sprintf("{ if _v == nil { e.U8(0) } else { e.U8(1); _encode_%s(*_v, e) } }", structT)
			valDec = fmt.Sprintf("if d.U8() != 0 { _sv := _decode_%s(d); _v = &_sv }", structT)
		}
	} else if structNames[valTyp] {
		valEnc = fmt.Sprintf("_encode_%s(_v, e)", valTyp)
		valDec = fmt.Sprintf("_v = _decode_%s(d)", valTyp)
	} else {
		return "", "", fmt.Errorf("map value type %q is not a supported primitive or known struct", valTyp)
	}

	enc = fmt.Sprintf(
		"{ e.U32(uint32(len(v.%s))); for _k, _v := range v.%s { %s; %s } }",
		fieldName, fieldName, keyEnc, valEnc)
	dec = fmt.Sprintf(
		"{ _n := int(d.U32()); v.%s = make(%s, _n); for _i := 0; _i < _n; _i++ { var _k %s; var _v %s; %s; %s; v.%s[_k] = _v } }",
		fieldName, typ, keyTyp, valTyp, keyDec, valDec, fieldName)
	return enc, dec, nil
}

func (h *WasmHelper) pointerCodecLines(fieldName, baseTyp string, structNames map[string]bool, aliases map[string]string) (enc, dec string, err error) {
	var valEnc, valDec string

	resolvedBase := baseTyp
	if underlying, ok := aliases[baseTyp]; ok {
		resolvedBase = underlying
	}

	if pe, pd := primitiveCodec(resolvedBase, "_pv"); pe != "" {
		valEnc = pe
		valDec = pd
		if resolvedBase != baseTyp {
			// Decode: cast back from the underlying type to the alias.
			rhs := strings.TrimPrefix(valDec, "_pv = ")
			valDec = fmt.Sprintf("_pv = %s(%s)", baseTyp, rhs)
		}
	} else if structNames[baseTyp] {
		valEnc = fmt.Sprintf("_encode_%s(*v.%s, e)", baseTyp, fieldName)
		valDec = fmt.Sprintf("{ _sv := _decode_%s(d); v.%s = &_sv }", baseTyp, fieldName)
		enc = fmt.Sprintf("{ if v.%s == nil { e.U8(0) } else { e.U8(1); %s } }", fieldName, valEnc)
		dec = fmt.Sprintf("{ if d.U8() != 0 { %s } }", valDec)
		return enc, dec, nil
	} else {
		return "", "", fmt.Errorf("pointer element type %q is not a supported primitive or known struct", baseTyp)
	}

	enc = fmt.Sprintf(
		"{ if v.%s == nil { e.U8(0) } else { e.U8(1); _pv := *v.%s; %s } }",
		fieldName, fieldName, valEnc)
	dec = fmt.Sprintf(
		"{ if d.U8() != 0 { var _pv %s; %s; v.%s = &_pv } }",
		baseTyp, valDec, fieldName)
	return enc, dec, nil
}

// ─── Source rewriting ─────────────────────────────────────────────────────────

func (h *WasmHelper) rewriteAutoKeys(src string) string {
	return autoKeyRe.ReplaceAllStringFunc(src, func(match string) string {
		m := autoKeyRe.FindStringSubmatch(match)
		typ, name := m[1], m[2]
		var encFn, decFn string
		if strings.HasPrefix(typ, "[]") {
			elem := typ[2:]
			encFn = "_encode_slice" + elem
			decFn = "_decode_slice" + elem
		} else {
			encFn = "_encode_" + typ
			decFn = "_decode_" + typ
		}
		return fmt.Sprintf(`BinaryKey[%s]("%s", %s, %s)`, typ, name, encFn, decFn)
	})
}

func (h *WasmHelper) rewriteContextCalls(src string, structs []structInfo) string {
	for _, s := range structs {
		if s.KeyName == "" {
			continue
		}
		ctor := h.ctxFuncName(s.Name)
		src = strings.ReplaceAll(src, "UseContext("+s.Name+"Key, "+s.Name, ctor+"("+s.Name)
		src = strings.ReplaceAll(src, "UseContext("+s.Name, ctor+"("+s.Name)
		src = strings.ReplaceAll(src, "Use"+s.Name+"("+s.Name, ctor+"("+s.Name)
		src = strings.ReplaceAll(src, "Use"+s.Name+"Context("+s.Name, ctor+"("+s.Name)
	}
	return src
}

// ─── Import filtering ─────────────────────────────────────────────────────────

func (h *WasmHelper) filterStdImports(content, body string) []string {
	raw := h.extractImportLines(content)
	var kept []string
	for _, line := range raw {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		path, alias := h.parseImportLine(line)
		if path == "" {
			continue
		}
		first := strings.SplitN(path, "/", 2)[0]
		if strings.Contains(first, ".") {
			continue
		}
		ident := alias
		if ident == "" || ident == "_" || ident == "." {
			parts := strings.Split(path, "/")
			ident = parts[len(parts)-1]
		}
		if strings.Contains(body, ident+".") {
			kept = append(kept, line)
		}
	}
	return kept
}

func (h *WasmHelper) extractImportLines(content string) []string {
	blockRe := regexp.MustCompile(`(?s)import\s*\((.+?)\)`)
	m := blockRe.FindStringSubmatch(content)
	if m == nil {
		return nil
	}
	return strings.Split(m[1], "\n")
}

func (h *WasmHelper) parseImportLine(line string) (path, alias string) {
	line = strings.TrimSpace(line)
	start := strings.Index(line, `"`)
	end := strings.LastIndex(line, `"`)
	if start == -1 || start == end {
		return "", ""
	}
	path = line[start+1 : end]
	alias = strings.TrimSpace(line[:start])
	return path, alias
}

// ─── Path normalization ───────────────────────────────────────────────────────

func (h *WasmHelper) wasmOutputName(httpPath string) string {
	if httpPath == "/" || httpPath == "" {
		return "index"
	}
	s := strings.TrimPrefix(httpPath, "/")
	s = strings.ReplaceAll(s, "/{", "-")
	s = strings.ReplaceAll(s, "}/", "-")
	s = strings.ReplaceAll(s, "}", "")
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, "{", "")
	return s
}

func (h *WasmHelper) normalizeWasmHttpPath(filePath string) string {
	if runtime.GOOS == "windows" {
		filePath = strings.ReplaceAll(filePath, `\`, `/`)
	}
	filePath = strings.TrimSuffix(filePath, "_templ.go")
	filePath = strings.TrimSuffix(filePath, ".go")
	filePath = strings.TrimPrefix(filePath, "src/pages")
	filePath = strings.TrimPrefix(filePath, "src")
	if strings.HasSuffix(filePath, "/index") {
		filePath = strings.TrimSuffix(filePath, "/index")
		if filePath == "" {
			filePath = "/"
		}
	}
	re := regexp.MustCompile(`var_([a-zA-Z0-9_]+)`)
	filePath = re.ReplaceAllString(filePath, `{$1}`)
	return filePath
}

// ─── Func body extraction ─────────────────────────────────────────────────────

func (h *WasmHelper) extractFuncBody(content string, openBrace int) string {
	depth := 0
	i := openBrace
	start, end := -1, -1
	for i < len(content) {
		switch content[i] {
		case '{':
			depth++
			if depth == 1 {
				start = i + 1
			}
		case '}':
			depth--
			if depth == 0 {
				end = i
				goto done
			}
		case '"':
			i++
			for i < len(content) && content[i] != '"' {
				if content[i] == '\\' {
					i++
				}
				i++
			}
		case '`':
			i++
			for i < len(content) && content[i] != '`' {
				i++
			}
		case '/':
			if i+1 < len(content) {
				if content[i+1] == '/' {
					for i < len(content) && content[i] != '\n' {
						i++
					}
				} else if content[i+1] == '*' {
					i += 2
					for i+1 < len(content) {
						if content[i] == '*' && content[i+1] == '/' {
							i++
							break
						}
						i++
					}
				}
			}
		}
		i++
	}
done:
	if start == -1 || end == -1 {
		return ""
	}
	return strings.TrimSpace(content[start:end])
}

// ─── Compression and file utilities ──────────────────────────────────────────

// compressionExt returns the suffix appended after ".wasm" (".gz" or ".br").
func compressionExt(c WasmCompression) string {
	if c == WasmCompressionBrotli {
		return ".br"
	}
	return ".gz"
}

func compressionLabel(c WasmCompression) string {
	if c == WasmCompressionBrotli {
		return "brotli"
	}
	return "gzip"
}

func (h *WasmHelper) compressWasmWith(src, dst string, c WasmCompression) error {
	in, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	if c == WasmCompressionBrotli {
		w := brotli.NewWriterLevel(f, brotli.BestCompression)
		if _, err := w.Write(in); err != nil {
			w.Close()
			return err
		}
		return w.Close()
	}
	w, err := gzip.NewWriterLevel(f, gzip.BestCompression)
	if err != nil {
		return err
	}
	if _, err := w.Write(in); err != nil {
		w.Close()
		return err
	}
	return w.Close()
}

func (h *WasmHelper) fileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

func (h *WasmHelper) formatBytes(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%dB", n)
	}
	return fmt.Sprintf("%dKB", n/1024)
}

// ─── Toolchain download ───────────────────────────────────────────────────────

const (
	wasmMaxRetries      = 3
	wasmDownloadTimeout = 10 * time.Minute
)

func (h *WasmHelper) downloadToTemp(url string) (string, error) {
	var lastErr error
	for attempt := 1; attempt <= wasmMaxRetries; attempt++ {
		path, err := h.tryDownload(url)
		if err == nil {
			return path, nil
		}
		lastErr = err
		if attempt < wasmMaxRetries {
			delay := 2 * time.Second * time.Duration(attempt)
			fmt.Fprintf(os.Stderr, "wasm: attempt %d/%d failed (%v) — retrying in %s\n",
				attempt, wasmMaxRetries, err, delay)
			time.Sleep(delay)
		}
	}
	return "", fmt.Errorf("download failed after %d attempts: %w", wasmMaxRetries, lastErr)
}

func (h *WasmHelper) tryDownload(url string) (string, error) {
	client := &http.Client{Timeout: wasmDownloadTimeout}
	resp, err := client.Get(url) //nolint:noctx
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}

	tmp, err := os.CreateTemp("", "tinygo-download-*")
	if err != nil {
		return "", fmt.Errorf("create temp: %w", err)
	}

	pr := &wasmProgressReader{r: resp.Body, total: resp.ContentLength}
	if _, err := io.Copy(tmp, pr); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", fmt.Errorf("write temp: %w", err)
	}
	fmt.Fprintln(os.Stderr)

	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	return tmp.Name(), nil
}

type wasmProgressReader struct {
	r     io.Reader
	total int64
	read  int64
}

func (p *wasmProgressReader) Read(buf []byte) (int, error) {
	n, err := p.r.Read(buf)
	p.read += int64(n)
	if p.total > 0 {
		pct := 100 * p.read / p.total
		fmt.Fprintf(os.Stderr, "\rwasm: %d%%  (%d MB / %d MB)", pct, p.read>>20, p.total>>20)
	} else {
		fmt.Fprintf(os.Stderr, "\rwasm: %d MB downloaded", p.read>>20)
	}
	return n, err
}

func (h *WasmHelper) fetchExpectedChecksum(checksumURL, filename string) (string, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(checksumURL) //nolint:noctx
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d fetching checksums.txt", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := strings.TrimPrefix(fields[1], "*")
		if name == filename {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("checksum not found for %q", filename)
}

func (h *WasmHelper) verifyChecksum(filePath, expected string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()
	hh := sha256.New()
	if _, err := io.Copy(hh, f); err != nil {
		return fmt.Errorf("hash %s: %w", filePath, err)
	}
	actual := hex.EncodeToString(hh.Sum(nil))
	if !strings.EqualFold(actual, expected) {
		return fmt.Errorf("SHA-256 mismatch\n  expected: %s\n  actual:   %s", expected, actual)
	}
	return nil
}

func (h *WasmHelper) computeChecksum(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	hh := sha256.New()
	if _, err := io.Copy(hh, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(hh.Sum(nil)), nil
}

// ─── Archive extraction ───────────────────────────────────────────────────────

func (h *WasmHelper) extractArchive(archivePath, destDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	magic := make([]byte, 4)
	_, err = f.Read(magic)
	f.Close()
	if err != nil {
		return fmt.Errorf("read magic bytes: %w", err)
	}
	switch {
	case magic[0] == 0x1f && magic[1] == 0x8b:
		return h.extractTarGz(archivePath, destDir)
	case magic[0] == 'P' && magic[1] == 'K':
		return h.extractZip(archivePath, destDir)
	default:
		return fmt.Errorf("unknown archive format (magic: %x)", magic[:2])
	}
}

func (h *WasmHelper) safeDest(destDir, entryName string) (string, error) {
	if entryName == "" {
		return "", fmt.Errorf("empty entry name")
	}
	dest := filepath.Join(destDir, filepath.FromSlash(entryName))
	prefix := filepath.Clean(destDir) + string(os.PathSeparator)
	if !strings.HasPrefix(dest+string(os.PathSeparator), prefix) {
		return "", fmt.Errorf("path traversal rejected: %q", entryName)
	}
	return dest, nil
}

func (h *WasmHelper) extractTarGz(archivePath, destDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("gzip reader: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar next: %w", err)
		}
		dest, err := h.safeDest(destDir, hdr.Name)
		if err != nil {
			return err
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(dest, 0755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
				return err
			}
			mode := hdr.FileInfo().Mode()
			if mode == 0 {
				mode = 0644
			}
			if err := h.writeFileFromReader(dest, tr, mode); err != nil {
				return fmt.Errorf("write %s: %w", hdr.Name, err)
			}
		case tar.TypeSymlink:
			if filepath.IsAbs(hdr.Linkname) {
				return fmt.Errorf("absolute symlink rejected: %q → %q", hdr.Name, hdr.Linkname)
			}
			os.Remove(dest)
			if err := os.Symlink(hdr.Linkname, dest); err != nil {
				return fmt.Errorf("symlink %s: %w", hdr.Name, err)
			}
		case tar.TypeLink:
			linkSrc, err := h.safeDest(destDir, hdr.Linkname)
			if err != nil {
				return fmt.Errorf("hard link source: %w", err)
			}
			os.Remove(dest)
			if err := os.Link(linkSrc, dest); err != nil {
				return fmt.Errorf("hard link %s: %w", hdr.Name, err)
			}
		}
	}
	return nil
}

func (h *WasmHelper) extractZip(archivePath, destDir string) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer r.Close()
	for _, f := range r.File {
		dest, err := h.safeDest(destDir, f.Name)
		if err != nil {
			return err
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(dest, 0755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			return err
		}
		mode := f.Mode()
		if mode == 0 {
			mode = 0644
		}
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("open zip entry %s: %w", f.Name, err)
		}
		writeErr := h.writeFileFromReader(dest, rc, mode)
		rc.Close()
		if writeErr != nil {
			return fmt.Errorf("write %s: %w", f.Name, writeErr)
		}
	}
	return nil
}

func (h *WasmHelper) writeFileFromReader(dest string, r io.Reader, mode os.FileMode) error {
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, r)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

// DefaultWasmHelper creates a WasmHelper using the current runtime's OS and architecture.
func DefaultWasmHelper() WasmHelper {
	return NewWasmHelper(runtime.GOOS, runtime.GOARCH)
}
