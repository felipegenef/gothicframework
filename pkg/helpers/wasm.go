package helpers

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	wasmruntime "github.com/felipegenef/gothicframework/pkg/wasm"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"
)

var ensureBinaryMu sync.Mutex

const tinyGoVersion = "0.41.1"

// WasmHelper manages the TinyGo toolchain and compiles WASM pages.
// It follows the same download-on-demand pattern as TailwindHelper.
type WasmHelper struct {
	Runtime        string // runtime.GOOS
	Arch           string // runtime.GOARCH
	Version        string // TinyGo version, default "0.41.1"
	ConfigOverride string // absolute path override from gothic-config.json
}

// WasmPage describes a single page that has a WASM state function.
type WasmPage struct {
	SourceFile string   // e.g. src/pages/counter_templ.go
	FuncName   string   // e.g. CounterState
	FuncBody   string   // extracted function body (between the braces)
	Imports    []string // filtered stdlib import lines for generated main.go
	HttpPath   string   // e.g. /counter
	OutputName string   // e.g. counter  (used for public/wasm/counter.wasm.gz)
}

func NewWasmHelper(goos, goarch string) WasmHelper {
	return WasmHelper{
		Runtime: goos,
		Arch:    goarch,
		Version: tinyGoVersion,
	}
}

// ─── Binary resolution ────────────────────────────────────────────────────────

// binaryName returns the archive filename for the current platform+version.
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

// cacheDir returns the OS-appropriate cache directory for TinyGo.
// Respects GOTHIC_CLI_CACHE_DIR env var.
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

// TinyGoRoot returns the TINYGOROOT path for the managed toolchain.
func (h *WasmHelper) TinyGoRoot() string {
	dir, err := h.cacheDir()
	if err != nil {
		return ""
	}
	platform := h.Runtime + "-" + h.Arch
	return filepath.Join(dir, "tinygo-"+h.Version, platform, "tinygo")
}

// TinyGoBinary returns the absolute path to the tinygo executable.
func (h *WasmHelper) TinyGoBinary() string {
	name := "tinygo"
	if h.Runtime == "windows" {
		name += ".exe"
	}
	return filepath.Join(h.TinyGoRoot(), "bin", name)
}

// Environ returns the env vars required to run TinyGo.
// Merge with os.Environ() when spawning a tinygo subprocess.
func (h *WasmHelper) Environ() []string {
	root := h.TinyGoRoot()
	binDir := filepath.Join(root, "bin")
	return []string{
		"TINYGOROOT=" + root,
		"PATH=" + binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
	}
}

// EnsureBinary downloads and installs TinyGo if it is not already cached.
// Resolution order:
// 1. ConfigOverride set → validate + return
// 2. Cached binary exists → return immediately (no network)
// 3. Download from GitHub releases → extract → cache
// Concurrent callers are serialized by ensureBinaryMu so only one download runs.
func (h *WasmHelper) EnsureBinary() error {
	if h.ConfigOverride != "" {
		if _, err := os.Stat(h.ConfigOverride); err != nil {
			return fmt.Errorf("wasm binary override not found at %q: %w", h.ConfigOverride, err)
		}
		return nil
	}

	// Fast path — no lock needed for a read-only existence check.
	if _, err := os.Stat(h.TinyGoBinary()); err == nil {
		return nil // already installed
	}

	// Slow path — serialize concurrent downloads.
	ensureBinaryMu.Lock()
	defer ensureBinaryMu.Unlock()

	// Re-check under the lock: another goroutine may have installed it while we waited.
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

// pageStateInlineRe matches:  PageState: func() {
var pageStateInlineRe = regexp.MustCompile(`(?m)PageState:\s*func\s*\(\s*\)\s*\{`)

// pageStateNamedRe matches:  PageState: FuncName  (named function reference)
var pageStateNamedRe = regexp.MustCompile(`(?m)PageState:\s*(\w+)`)

// ScanPages walks pagesDir and componentsDir for *_templ.go files that have
// a RouteConfig with PageState set, and returns a WasmPage for each.
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

	// Pattern A — inline func literal:  PageState: func() { … }
	if loc := pageStateInlineRe.FindStringIndex(content); loc != nil {
		openBrace := loc[1] - 1 // the '{' at the end of the match
		body = extractFuncBody(content, openBrace)
		if body == "" {
			return WasmPage{}, false, fmt.Errorf("wasm: could not extract inline PageState body in %s", path)
		}
	} else if m := pageStateNamedRe.FindStringSubmatch(content); m != nil {
		// Pattern B — named reference:  PageState: FuncName
		funcName := m[1]
		funcRe := regexp.MustCompile(`(?m)^func\s+` + regexp.QuoteMeta(funcName) + `\s*\(\s*\)\s*\{`)
		floc := funcRe.FindStringIndex(content)
		if floc == nil {
			return WasmPage{}, false, fmt.Errorf("wasm: PageState func %s not found in %s", funcName, path)
		}
		openBrace := floc[1] - 1
		body = extractFuncBody(content, openBrace)
		if body == "" {
			return WasmPage{}, false, fmt.Errorf("wasm: could not extract body of %s in %s", funcName, path)
		}
	} else {
		return WasmPage{}, false, nil // no PageState in this file
	}

	stdImports := filterStdImports(content, body)
	httpPath := normalizeWasmHttpPath(path)
	outputName := wasmOutputName(httpPath)

	return WasmPage{
		SourceFile: path,
		FuncBody:   body,
		Imports:    stdImports,
		HttpPath:   httpPath,
		OutputName: outputName,
	}, true, nil
}

// ─── Build pipeline ────────────────────────────────────────────────────────────

// GeneratePage compiles a single WasmPage to outDir/page.OutputName.wasm.gz.
func (h *WasmHelper) GeneratePage(page WasmPage, outDir string) error {
	// Extract the runtime into a fresh temp module dir.
	tempModDir, err := os.MkdirTemp("", "tinygo-runtime-*")
	if err != nil {
		return fmt.Errorf("wasm: mkdirtemp: %w", err)
	}
	defer os.RemoveAll(tempModDir)

	if err := wasmruntime.ExtractRuntime(tempModDir); err != nil {
		return fmt.Errorf("wasm: extract runtime: %w", err)
	}

	// Write generated main.go into a subdirectory of the module dir.
	genDir, err := os.MkdirTemp(tempModDir, ".gen-")
	if err != nil {
		return fmt.Errorf("wasm: mkdirtemp gen: %w", err)
	}

	mainPath := filepath.Join(genDir, "main.go")
	if err := writeWasmMain(page.SourceFile, page.FuncBody, page.Imports, mainPath); err != nil {
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

	wasmSize, _ := fileSize(absOutFile)

	// Optional wasm-opt pass.
	if _, err := exec.LookPath("wasm-opt"); err == nil {
		tmp := absOutFile + ".opt"
		opt := exec.Command("wasm-opt", "-Oz", "--strip-debug", "-o", tmp, absOutFile)
		if err := opt.Run(); err == nil {
			os.Rename(tmp, absOutFile)
		} else {
			os.Remove(tmp)
		}
	}

	// Gzip compress.
	gzPath := absOutFile + ".gz"
	if err := compressWasm(absOutFile, gzPath); err != nil {
		return fmt.Errorf("wasm: gzip %s: %w", page.OutputName, err)
	}
	os.Remove(absOutFile)

	gzSize, _ := fileSize(gzPath)
	fmt.Printf("wasm: %s → %s → %s (gzip)\n",
		page.OutputName, formatBytes(wasmSize), formatBytes(gzSize))
	return nil
}

// GenerateAll clears outDir and rebuilds all pages in parallel, then copies wasm_exec.js.
func (h *WasmHelper) GenerateAll(pages []WasmPage, outDir string) error {
	if err := h.EnsureBinary(); err != nil {
		return err
	}
	if len(pages) == 0 {
		return nil
	}

	os.RemoveAll(outDir)
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return fmt.Errorf("wasm: mkdir %s: %w", outDir, err)
	}

	g, ctx := errgroup.WithContext(context.Background())
	sem := semaphore.NewWeighted(int64(runtime.NumCPU()))
	for _, page := range pages {
		page := page
		g.Go(func() error {
			if err := sem.Acquire(ctx, 1); err != nil {
				return err
			}
			defer sem.Release(1)
			return h.GeneratePage(page, outDir)
		})
	}
	if err := g.Wait(); err != nil {
		return err
	}
	return h.CopyWasmExec("public")
}

// CopyWasmExec copies wasm_exec.js from the TinyGo cache into destDir.
// Skips the copy if the file is already present and has the same size.
func (h *WasmHelper) CopyWasmExec(destDir string) error {
	src := filepath.Join(h.TinyGoRoot(), "targets", "wasm_exec.js")
	dst := filepath.Join(destDir, "wasm_exec.js")

	srcInfo, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("wasm: wasm_exec.js not found at %s: %w", src, err)
	}

	if dstInfo, err := os.Stat(dst); err == nil {
		if dstInfo.Size() == srcInfo.Size() {
			return nil // already up to date
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

// ─── Code generation helpers ──────────────────────────────────────────────────

func writeWasmMain(src, body string, stdImports []string, dest string) error {
	var sb strings.Builder
	sb.WriteString("//go:build js && wasm\n")
	sb.WriteString("// Code generated from " + src + " — DO NOT EDIT.\n\n")
	sb.WriteString("package main\n\n")
	sb.WriteString("import (\n")
	sb.WriteString("\t. \"wasm-runtime/runtime\"\n")
	for _, imp := range stdImports {
		sb.WriteString("\t" + strings.TrimSpace(imp) + "\n")
	}
	sb.WriteString(")\n\n")
	sb.WriteString("func main() {\n")
	for _, line := range strings.Split(body, "\n") {
		sb.WriteString("\t" + line + "\n")
	}
	sb.WriteString("}\n")
	return os.WriteFile(dest, []byte(sb.String()), 0644)
}

// extractFuncBody returns the trimmed content inside the outermost braces,
// handling nested braces, strings, and comments.
func extractFuncBody(content string, openBrace int) string {
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

// filterStdImports returns import lines from content that are stdlib packages
// whose identifier appears in body.  The wasm-runtime import is always injected
// separately by writeWasmMain and must NOT be in this list.
func filterStdImports(content, body string) []string {
	raw := extractImportLines(content)
	var kept []string
	for _, line := range raw {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		path, alias := parseImportLine(line)
		if path == "" {
			continue
		}
		// Skip any framework or project-specific imports — only stdlib allowed.
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

func extractImportLines(content string) []string {
	blockRe := regexp.MustCompile(`(?s)import\s*\((.+?)\)`)
	m := blockRe.FindStringSubmatch(content)
	if m == nil {
		return nil
	}
	return strings.Split(m[1], "\n")
}

func parseImportLine(line string) (path, alias string) {
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

// wasmOutputName converts an HTTP path to a safe wasm output filename.
func wasmOutputName(httpPath string) string {
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

// normalizeWasmHttpPath converts a *_templ.go file path to an HTTP path.
// Mirrors the logic in FileBasedRouteHelper.normalizeHttpPath.
func normalizeWasmHttpPath(filePath string) string {
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

// ─── Compression and utilities ────────────────────────────────────────────────

func compressWasm(src, dst string) error {
	in, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
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

func fileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

func formatBytes(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%dB", n)
	}
	return fmt.Sprintf("%dKB", n/1024)
}

// ─── Toolchain download internals ─────────────────────────────────────────────

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

// extractArchive detects the archive format from magic bytes and extracts it.
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

func safeDest(destDir, entryName string) (string, error) {
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
		dest, err := safeDest(destDir, hdr.Name)
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
			if err := writeFileFromReader(dest, tr, mode); err != nil {
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
			linkSrc, err := safeDest(destDir, hdr.Linkname)
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
		dest, err := safeDest(destDir, f.Name)
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
		writeErr := writeFileFromReader(dest, rc, mode)
		rc.Close()
		if writeErr != nil {
			return fmt.Errorf("write %s: %w", f.Name, writeErr)
		}
	}
	return nil
}

func writeFileFromReader(dest string, r io.Reader, mode os.FileMode) error {
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
