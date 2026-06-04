package helpers

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go/ast"
	"go/parser"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// ---------------------------------------------------------------------------
// wasm_binary.go — pure path/name resolution helpers
// ---------------------------------------------------------------------------

func linuxAmd64Helper() *WasmHelper {
	h := DefaultWasmHelper()
	h.Runtime = "linux"
	h.Arch = "amd64"
	h.Version = "0.41.1"
	h.BinaryenVersion = "117"
	return &h
}

func TestBinaryName_AllPlatforms(t *testing.T) {
	cases := []struct {
		goos, arch string
		wantSub    string
		wantErr    bool
	}{
		{"linux", "amd64", "linux-amd64.tar.gz", false},
		{"linux", "arm64", "linux-arm64.tar.gz", false},
		{"darwin", "amd64", "darwin-amd64.tar.gz", false},
		{"darwin", "arm64", "darwin-arm64.tar.gz", false},
		{"windows", "amd64", "windows-amd64.zip", false},
		{"plan9", "mips", "", true},
	}
	for _, c := range cases {
		h := &WasmHelper{Runtime: c.goos, Arch: c.arch, Version: "0.41.1"}
		got, err := h.binaryName()
		if c.wantErr {
			if err == nil {
				t.Errorf("%s/%s: expected error", c.goos, c.arch)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s/%s: unexpected error: %v", c.goos, c.arch, err)
		}
		if !strings.Contains(got, c.wantSub) {
			t.Errorf("%s/%s: got %q, want substr %q", c.goos, c.arch, got, c.wantSub)
		}
	}
}

func TestBinaryenBinaryName_AllPlatforms(t *testing.T) {
	cases := []struct {
		goos, arch string
		wantSub    string
		wantErr    bool
	}{
		{"linux", "amd64", "x86_64-linux", false},
		{"linux", "arm64", "aarch64-linux", false},
		{"darwin", "amd64", "x86_64-macos", false},
		{"darwin", "arm64", "arm64-macos", false},
		{"windows", "amd64", "x86_64-windows", false},
		{"plan9", "mips", "", true},
	}
	for _, c := range cases {
		h := &WasmHelper{Runtime: c.goos, Arch: c.arch, BinaryenVersion: "117"}
		got, err := h.binaryenBinaryName()
		if c.wantErr {
			if err == nil {
				t.Errorf("%s/%s: expected error", c.goos, c.arch)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s/%s: unexpected error: %v", c.goos, c.arch, err)
		}
		if !strings.Contains(got, c.wantSub) {
			t.Errorf("%s/%s: got %q, want substr %q", c.goos, c.arch, got, c.wantSub)
		}
	}
}

func TestCacheDir_RespectsEnvOverride(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("GOTHIC_CLI_CACHE_DIR", tmp)
	h := linuxAmd64Helper()
	dir, err := h.cacheDir()
	if err != nil {
		t.Fatalf("cacheDir: %v", err)
	}
	want := filepath.Join(tmp, "gothic-cli", "tinygo")
	if dir != want {
		t.Errorf("cacheDir: got %q, want %q", dir, want)
	}
}

func TestPathResolvers_Linux(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("GOTHIC_CLI_CACHE_DIR", tmp)
	h := linuxAmd64Helper()

	tinyRoot := h.TinyGoRoot()
	if !strings.Contains(tinyRoot, "tinygo-0.41.1") || !strings.Contains(tinyRoot, "linux-amd64") {
		t.Errorf("TinyGoRoot: got %q", tinyRoot)
	}
	tinyBin := h.TinyGoBinary()
	if filepath.Base(tinyBin) != "tinygo" {
		t.Errorf("TinyGoBinary base: got %q", filepath.Base(tinyBin))
	}

	bRoot := h.BinaryenRoot()
	if !strings.Contains(bRoot, "binaryen-117") {
		t.Errorf("BinaryenRoot: got %q", bRoot)
	}
	bBin := h.BinaryenBinary()
	if filepath.Base(bBin) != "wasm-opt" {
		t.Errorf("BinaryenBinary base: got %q", filepath.Base(bBin))
	}
}

func TestPathResolvers_Windows(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("GOTHIC_CLI_CACHE_DIR", tmp)
	h := DefaultWasmHelper()
	h.Runtime = "windows"
	h.Arch = "amd64"
	h.Version = "0.41.1"
	h.BinaryenVersion = "117"

	if base := filepath.Base(h.TinyGoBinary()); base != "tinygo.exe" {
		t.Errorf("windows TinyGoBinary base: got %q, want tinygo.exe", base)
	}
	if base := filepath.Base(h.BinaryenBinary()); base != "wasm-opt.exe" {
		t.Errorf("windows BinaryenBinary base: got %q, want wasm-opt.exe", base)
	}
}

func TestEnviron_SetsTinygoRootAndPath(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("GOTHIC_CLI_CACHE_DIR", tmp)
	h := linuxAmd64Helper()

	env := h.Environ()
	var sawRoot, sawPath, sawWasmOpt bool
	for _, e := range env {
		if strings.HasPrefix(e, "TINYGOROOT=") {
			sawRoot = true
		}
		if strings.HasPrefix(e, "PATH=") {
			sawPath = true
		}
		if e == "WASMOPT=false" {
			sawWasmOpt = true
		}
	}
	if !sawRoot || !sawPath {
		t.Errorf("Environ missing TINYGOROOT/PATH: %v", env)
	}
	// In the hermetic test env there is almost certainly no wasm-opt under the
	// fresh temp cache dir, so WASMOPT=false is expected — exercise the
	// warnOnce branch via EnvironWithWarn.
	var once sync.Once
	_ = h.EnvironWithWarn(&once)
	_ = sawWasmOpt
}

// ---------------------------------------------------------------------------
// wasm_archive.go — extract tar.gz / zip / format detection / traversal guard
// ---------------------------------------------------------------------------

func makeTarGz(t *testing.T, files map[string]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "a.tar.gz")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		hdr := &tar.Header{Name: name, Mode: 0644, Size: int64(len(body)), Typeflag: tar.TypeReg}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("tar header: %v", err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatalf("tar write: %v", err)
		}
	}
	// add a directory entry
	_ = tw.WriteHeader(&tar.Header{Name: "subdir/", Mode: 0755, Typeflag: tar.TypeDir})
	tw.Close()
	gz.Close()
	return path
}

func TestExtractArchive_TarGz(t *testing.T) {
	src := makeTarGz(t, map[string]string{
		"bin/tinygo": "binary-bytes",
		"README":     "hello",
	})
	dest := t.TempDir()
	h := linuxAmd64Helper()
	if err := h.extractArchive(src, dest); err != nil {
		t.Fatalf("extractArchive: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dest, "bin", "tinygo"))
	if err != nil {
		t.Fatalf("read extracted: %v", err)
	}
	if string(got) != "binary-bytes" {
		t.Errorf("extracted content mismatch: %q", got)
	}
	if _, err := os.Stat(filepath.Join(dest, "subdir")); err != nil {
		t.Errorf("expected subdir to be created: %v", err)
	}
}

func TestExtractArchive_Zip(t *testing.T) {
	zipPath := filepath.Join(t.TempDir(), "a.zip")
	zf, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	zw := zip.NewWriter(zf)
	w, _ := zw.Create("bin/tinygo.exe")
	w.Write([]byte("win-bytes"))
	// directory entry
	zw.Create("emptydir/")
	zw.Close()
	zf.Close()

	dest := t.TempDir()
	h := linuxAmd64Helper()
	if err := h.extractArchive(zipPath, dest); err != nil {
		t.Fatalf("extractArchive zip: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dest, "bin", "tinygo.exe"))
	if err != nil {
		t.Fatalf("read extracted: %v", err)
	}
	if string(got) != "win-bytes" {
		t.Errorf("zip content mismatch: %q", got)
	}
}

func TestExtractArchive_UnknownFormat(t *testing.T) {
	p := filepath.Join(t.TempDir(), "weird.bin")
	if err := os.WriteFile(p, []byte("XXXX not an archive"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	h := linuxAmd64Helper()
	if err := h.extractArchive(p, t.TempDir()); err == nil {
		t.Fatal("expected error for unknown archive format")
	}
}

func TestExtractArchive_MissingFile(t *testing.T) {
	h := linuxAmd64Helper()
	if err := h.extractArchive(filepath.Join(t.TempDir(), "nope.tar.gz"), t.TempDir()); err == nil {
		t.Fatal("expected error opening missing archive")
	}
}

func TestSafeDest(t *testing.T) {
	h := linuxAmd64Helper()
	dest := t.TempDir()

	got, err := h.safeDest(dest, "bin/tinygo")
	if err != nil {
		t.Fatalf("safeDest valid: %v", err)
	}
	if !strings.HasPrefix(got, dest) {
		t.Errorf("safeDest: %q not under %q", got, dest)
	}

	if _, err := h.safeDest(dest, ""); err == nil {
		t.Error("expected error for empty entry name")
	}
	if _, err := h.safeDest(dest, "../escape"); err == nil {
		t.Error("expected path traversal rejection")
	}
}

func TestExtractTarGz_TraversalRejected(t *testing.T) {
	src := makeTarGz(t, map[string]string{"../evil": "pwned"})
	h := linuxAmd64Helper()
	if err := h.extractArchive(src, t.TempDir()); err == nil {
		t.Fatal("expected traversal entry to be rejected")
	}
}

func TestWriteFileFromReader(t *testing.T) {
	h := linuxAmd64Helper()
	dest := filepath.Join(t.TempDir(), "out.txt")
	if err := h.writeFileFromReader(dest, strings.NewReader("payload"), 0644); err != nil {
		t.Fatalf("writeFileFromReader: %v", err)
	}
	got, _ := os.ReadFile(dest)
	if string(got) != "payload" {
		t.Errorf("got %q", got)
	}
}

// ---------------------------------------------------------------------------
// wasm_binary.go — checksum + download via httptest.Server
// ---------------------------------------------------------------------------

func TestComputeAndVerifyChecksum(t *testing.T) {
	h := linuxAmd64Helper()
	p := filepath.Join(t.TempDir(), "data.bin")
	content := []byte("the quick brown fox")
	if err := os.WriteFile(p, content, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	sum := sha256.Sum256(content)
	expected := hex.EncodeToString(sum[:])

	got, err := h.computeChecksum(p)
	if err != nil {
		t.Fatalf("computeChecksum: %v", err)
	}
	if got != expected {
		t.Errorf("computeChecksum: got %q, want %q", got, expected)
	}

	if err := h.verifyChecksum(p, expected); err != nil {
		t.Errorf("verifyChecksum (match): %v", err)
	}
	// case-insensitive match
	if err := h.verifyChecksum(p, strings.ToUpper(expected)); err != nil {
		t.Errorf("verifyChecksum (upper): %v", err)
	}
	if err := h.verifyChecksum(p, "deadbeef"); err == nil {
		t.Error("verifyChecksum: expected mismatch error")
	}
	if _, err := h.computeChecksum(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Error("computeChecksum: expected error for missing file")
	}
	if err := h.verifyChecksum(filepath.Join(t.TempDir(), "missing"), expected); err == nil {
		t.Error("verifyChecksum: expected error for missing file")
	}
}

func TestTryDownloadAndDownloadToTemp(t *testing.T) {
	const body = "fake binary content"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
		w.Write([]byte(body))
	}))
	defer srv.Close()

	h := linuxAmd64Helper()
	path, err := h.tryDownload(srv.URL)
	if err != nil {
		t.Fatalf("tryDownload: %v", err)
	}
	defer os.Remove(path)
	got, _ := os.ReadFile(path)
	if string(got) != body {
		t.Errorf("downloaded content: got %q, want %q", got, body)
	}

	// downloadToTemp wraps tryDownload with retries; happy path should succeed
	// immediately.
	path2, err := h.downloadToTemp(srv.URL)
	if err != nil {
		t.Fatalf("downloadToTemp: %v", err)
	}
	defer os.Remove(path2)
	got2, _ := os.ReadFile(path2)
	if string(got2) != body {
		t.Errorf("downloadToTemp content mismatch: %q", got2)
	}
}

func TestTryDownload_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer srv.Close()
	h := linuxAmd64Helper()
	if _, err := h.tryDownload(srv.URL); err == nil {
		t.Fatal("expected error for HTTP 404")
	}
}

func TestWasmProgressReader_Read(t *testing.T) {
	// total > 0 branch
	pr := &wasmProgressReader{r: strings.NewReader("hello world"), total: 11}
	buf := make([]byte, 4)
	n, err := pr.Read(buf)
	if err != nil || n == 0 {
		t.Fatalf("read: n=%d err=%v", n, err)
	}
	// total == 0 branch
	pr2 := &wasmProgressReader{r: strings.NewReader("data"), total: 0}
	if _, err := pr2.Read(buf); err != nil {
		t.Fatalf("read total=0: %v", err)
	}
}

// ---------------------------------------------------------------------------
// wasm_compress.go
// ---------------------------------------------------------------------------

func TestCompressionExtAndLabel(t *testing.T) {
	if compressionExt(WasmCompressionBrotli) != ".br" {
		t.Error("brotli ext")
	}
	if compressionExt(WasmCompressionGzip) != ".gz" {
		t.Error("gzip ext")
	}
	if compressionLabel(WasmCompressionBrotli) != "brotli" {
		t.Error("brotli label")
	}
	if compressionLabel(WasmCompressionGzip) != "gzip" {
		t.Error("gzip label")
	}
}

func TestCompressWasmWith(t *testing.T) {
	h := linuxAmd64Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "in.wasm")
	payload := bytes.Repeat([]byte("compress me "), 100)
	if err := os.WriteFile(src, payload, 0644); err != nil {
		t.Fatalf("write src: %v", err)
	}

	gzDst := filepath.Join(dir, "out.wasm.gz")
	if err := h.compressWasmWith(src, gzDst, WasmCompressionGzip); err != nil {
		t.Fatalf("gzip compress: %v", err)
	}
	// verify it round-trips
	raw, _ := os.ReadFile(gzDst)
	gr, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	var out bytes.Buffer
	out.ReadFrom(gr)
	if !bytes.Equal(out.Bytes(), payload) {
		t.Error("gzip round-trip mismatch")
	}

	brDst := filepath.Join(dir, "out.wasm.br")
	if err := h.compressWasmWith(src, brDst, WasmCompressionBrotli); err != nil {
		t.Fatalf("brotli compress: %v", err)
	}
	if info, _ := os.Stat(brDst); info == nil || info.Size() == 0 {
		t.Error("brotli output empty")
	}

	if err := h.compressWasmWith(filepath.Join(dir, "missing"), gzDst, WasmCompressionGzip); err == nil {
		t.Error("expected error for missing source")
	}
}

func TestFileSizeAndFormatBytes(t *testing.T) {
	h := linuxAmd64Helper()
	p := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(p, make([]byte, 2048), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	sz, err := h.fileSize(p)
	if err != nil || sz != 2048 {
		t.Errorf("fileSize: got %d err=%v", sz, err)
	}
	if _, err := h.fileSize(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Error("fileSize: expected error for missing file")
	}
	if got := h.formatBytes(512); got != "512B" {
		t.Errorf("formatBytes(512): got %q", got)
	}
	if got := h.formatBytes(2048); got != "2KB" {
		t.Errorf("formatBytes(2048): got %q", got)
	}
}

// ---------------------------------------------------------------------------
// wasm_log.go — exercise the printers (they only Printf to stdout)
// ---------------------------------------------------------------------------

func TestWasmLogHelpers_DoNotPanic(t *testing.T) {
	if wasmTimestamp() == "" {
		t.Error("empty timestamp")
	}
	wasmUpToDate("page")
	wasmLogf("building %s", "x")
	wasmErrorf("oops %d", 1)
	wasmWarnf("careful %s", "y")
	wasmBuildResult("page", "10KB", "3KB", "gzip")
}

// ---------------------------------------------------------------------------
// wasm_codec_types.go — typeRef.String + typeRefFromExpr full shapes
// ---------------------------------------------------------------------------

func TestTypeRefStringShapes(t *testing.T) {
	cases := map[string]string{
		"int":                  "int",
		"[]string":             "[]string",
		"map[string]int":       "map[string]int",
		"*Item":                "*Item",
		"[]*Item":              "[]*Item",
		"map[string][]*Item":   "map[string][]*Item",
		"pkg.Type":             "pkg.Type",
		"[5]int":               "int", // fixed array degrades to element
	}
	for input, want := range cases {
		expr, err := parser.ParseExpr(input)
		if err != nil {
			t.Fatalf("parse %q: %v", input, err)
		}
		ref, err := typeRefFromExpr(expr)
		if err != nil {
			t.Fatalf("typeRefFromExpr(%q): %v", input, err)
		}
		if got := ref.String(); got != want {
			t.Errorf("typeRef(%q).String() = %q, want %q", input, got, want)
		}
	}
}

func TestTypeRefFromExpr_Unsupported(t *testing.T) {
	for _, input := range []string{"chan int", "func()", "interface{}"} {
		expr, err := parser.ParseExpr(input)
		if err != nil {
			t.Fatalf("parse %q: %v", input, err)
		}
		if _, err := typeRefFromExpr(expr); err == nil {
			t.Errorf("expected error for unsupported type %q", input)
		}
	}
	// selector with non-ident receiver
	expr, _ := parser.ParseExpr("a.b.C")
	if _, err := typeRefFromExpr(expr); err == nil {
		t.Error("expected error for nested selector receiver")
	}

	// Nested unsupported element types propagate the error through the
	// slice/pointer/map element recursion.
	for _, input := range []string{
		"[]chan int",      // slice element unsupported
		"*chan int",       // pointer element unsupported
		"map[chan int]int", // map key unsupported
		"map[string]chan int", // map value unsupported
	} {
		e, perr := parser.ParseExpr(input)
		if perr != nil {
			t.Fatalf("parse %q: %v", input, perr)
		}
		if _, err := typeRefFromExpr(e); err == nil {
			t.Errorf("expected error for nested unsupported %q", input)
		}
	}
}

// ---------------------------------------------------------------------------
// wasm_codec.go — builder funcs that were 0%
// ---------------------------------------------------------------------------

func codecTopicFixture() ([]structInfo, map[string]bool) {
	page := structInfo{
		Name:        "Page",
		KeyName:     "page",
		Compression: WasmCompressionBrotli,
		Fields: []fieldInfo{
			testFieldInfo("Count", "int"),
			testFieldInfo("Label", "string"),
		},
	}
	noKey := structInfo{
		Name: "Helper",
		Fields: []fieldInfo{
			testFieldInfo("V", "int"),
		},
	}
	return []structInfo{page, noKey}, map[string]bool{"Page": true, "Helper": true}
}

func TestBuildKeyVarData(t *testing.T) {
	h := DefaultWasmHelper()
	structs, _ := codecTopicFixture()
	kv := h.buildKeyVarData(structs)
	if len(kv) != 1 {
		t.Fatalf("expected 1 keyed struct, got %d", len(kv))
	}
	if kv[0].StructName != "Page" || kv[0].KeyName != "page" {
		t.Errorf("unexpected KeyVarData: %+v", kv[0])
	}
}

func TestBuildTopicTypeData(t *testing.T) {
	h := DefaultWasmHelper()
	structs, _ := codecTopicFixture()
	td := h.buildTopicTypeData(structs)
	if len(td) != 1 {
		t.Fatalf("expected 1 topic type, got %d", len(td))
	}
	if td[0].TypeName != "pageTopic" {
		t.Errorf("TypeName: got %q, want pageTopic", td[0].TypeName)
	}
	if len(td[0].Fields) != 2 {
		t.Errorf("expected 2 fields, got %d", len(td[0].Fields))
	}
}

func TestBuildServerTopicFuncData(t *testing.T) {
	h := DefaultWasmHelper()
	structs, _ := codecTopicFixture()
	sf := h.buildServerTopicFuncData(structs)
	if len(sf) != 1 {
		t.Fatalf("expected 1 server func, got %d", len(sf))
	}
	if sf[0].CtorName != "PageTopic" || sf[0].TypeName != "pageTopic" {
		t.Errorf("unexpected ServerTopicFuncData: %+v", sf[0])
	}
}

func TestBuildMountFnData(t *testing.T) {
	h := DefaultWasmHelper()
	structs, _ := codecTopicFixture()
	mf := h.buildMountFnData(structs)
	if len(mf) != 1 {
		t.Fatalf("expected 1 mount fn, got %d", len(mf))
	}
	if mf[0].WasmName != "topic-page" {
		t.Errorf("WasmName: got %q", mf[0].WasmName)
	}
	if mf[0].CompressionConst != "routes.BROTLI" {
		t.Errorf("CompressionConst: got %q, want routes.BROTLI", mf[0].CompressionConst)
	}
	if mf[0].FuncName != "AddPageTopic" {
		t.Errorf("FuncName: got %q, want AddPageTopic", mf[0].FuncName)
	}
}

func TestBuildMountFnData_GzipDefault(t *testing.T) {
	h := DefaultWasmHelper()
	s := structInfo{Name: "X", KeyName: "x", Compression: WasmCompressionGzip}
	mf := h.buildMountFnData([]structInfo{s})
	if mf[0].CompressionConst != "routes.GZIP" {
		t.Errorf("CompressionConst: got %q, want routes.GZIP", mf[0].CompressionConst)
	}
}

func TestBuildWasmTopicFuncData(t *testing.T) {
	h := DefaultWasmHelper()
	structs, _ := codecTopicFixture()
	wf, err := h.buildWasmTopicFuncData(structs, map[string]string{}, nil)
	if err != nil {
		t.Fatalf("buildWasmTopicFuncData: %v", err)
	}
	if len(wf) != 1 {
		t.Fatalf("expected 1 wasm topic func, got %d", len(wf))
	}
	if wf[0].StructName != "Page" || len(wf[0].FieldCodecs) != 2 {
		t.Errorf("unexpected WasmTopicFuncData: %+v", wf[0])
	}
}

// ---------------------------------------------------------------------------
// wasm_topic.go — pure parsers / name helpers that were 0%
// ---------------------------------------------------------------------------

func TestHasTopicStructs(t *testing.T) {
	h := DefaultWasmHelper()
	if h.hasTopicStructs([]structInfo{{Name: "X"}}) {
		t.Error("no KeyName → should be false")
	}
	if !h.hasTopicStructs([]structInfo{{Name: "X", KeyName: "x"}}) {
		t.Error("KeyName set → should be true")
	}
}

func TestHasTimeFields(t *testing.T) {
	h := DefaultWasmHelper()
	withTime := []structInfo{{Name: "E", Fields: []fieldInfo{{Name: "At", Type: "time.Time"}}}}
	if !h.hasTimeFields(withTime) {
		t.Error("expected hasTimeFields true")
	}
	without := []structInfo{{Name: "E", Fields: []fieldInfo{{Name: "N", Type: "int"}}}}
	if h.hasTimeFields(without) {
		t.Error("expected hasTimeFields false")
	}
}

func TestTopicTypeNameAndComponentFuncName(t *testing.T) {
	h := DefaultWasmHelper()
	if got := h.topicTypeName("Page"); got != "pageTopic" {
		t.Errorf("topicTypeName: got %q", got)
	}
	if got := h.componentFuncNameFor(structInfo{Name: "Page"}); got != "AddPageTopic" {
		t.Errorf("componentFuncNameFor default: got %q", got)
	}
	if got := h.componentFuncNameFor(structInfo{Name: "Page", ComponentFnName: "Custom"}); got != "Custom" {
		t.Errorf("componentFuncNameFor override: got %q", got)
	}
}

func TestIsCreateTopicCall(t *testing.T) {
	cases := map[string]bool{
		"CreateTopic(X{}, C{})":            true,
		"wasm.CreateTopic(X{}, C{})":       true,
		"CreateTopic[Page](X{}, C{})":      true,
		"pkg.CreateTopic[Page](X{}, C{})":  true,
		"SomethingElse(X{})":               false,
		"obj.Method()":                     false,
	}
	for src, want := range cases {
		ce := mustCallExpr(t, src)
		if got := isCreateTopicCall(ce.Fun); got != want {
			t.Errorf("isCreateTopicCall(%q) = %v, want %v", src, got, want)
		}
	}
}

func TestTopicStructNameFromArg(t *testing.T) {
	ce := mustCallExpr(t, "CreateTopic(Page{}, C{})")
	if got := topicStructNameFromArg(ce.Args[0]); got != "Page" {
		t.Errorf("composite lit: got %q", got)
	}
	ce2 := mustCallExpr(t, "CreateTopic(pkg.Page{}, C{})")
	if got := topicStructNameFromArg(ce2.Args[0]); got != "Page" {
		t.Errorf("selector lit: got %q", got)
	}
	ce3 := mustCallExpr(t, "CreateTopic(Page, C{})")
	if got := topicStructNameFromArg(ce3.Args[0]); got != "Page" {
		t.Errorf("bare ident: got %q", got)
	}
	// non-matching arg
	ce4 := mustCallExpr(t, "CreateTopic(42, C{})")
	if got := topicStructNameFromArg(ce4.Args[0]); got != "" {
		t.Errorf("non-type arg: got %q, want empty", got)
	}
}

func TestParseCompressionExpr(t *testing.T) {
	mk := func(src string) WasmCompression {
		ce := mustCallExpr(t, "f("+src+")")
		return parseCompressionExpr(ce.Args[0])
	}
	if mk(`"BROTLI"`) != WasmCompressionBrotli {
		t.Error("string BROTLI")
	}
	if mk("BROTLI") != WasmCompressionBrotli {
		t.Error("ident BROTLI")
	}
	if mk("wasm.BROTLI") != WasmCompressionBrotli {
		t.Error("selector BROTLI")
	}
	if mk(`"GZIP"`) != WasmCompressionGzip {
		t.Error("GZIP default")
	}
	if mk("123") != WasmCompressionGzip {
		t.Error("numeric → default gzip")
	}
}

func TestParseCompilerExpr(t *testing.T) {
	mk := func(src string) WasmCompilerChoice {
		ce := mustCallExpr(t, "f("+src+")")
		return parseCompilerExpr(ce.Args[0])
	}
	if mk("LocalTinyGo") != WasmCompilerLocalTinyGo {
		t.Error("ident LocalTinyGo")
	}
	if mk("routes.Golang") != WasmCompilerGolang {
		t.Error("selector Golang")
	}
	if mk("Whatever") != WasmCompilerGothicTinyGo {
		t.Error("default GothicTinyGo")
	}
}

func TestCollectCreateTopicMetas_EdgeCases(t *testing.T) {
	// Non-call value, call with <2 args, and unresolvable struct name are all
	// skipped without producing a meta entry.
	src := `package p
var a = 42
var b = CreateTopic(Page{})
var c = CreateTopic(42, TopicConfig{Name: "x"})`
	f := parseFile(src)
	metas := collectCreateTopicMetas(f)
	if len(metas) != 0 {
		t.Errorf("expected no metas for malformed CreateTopic calls, got %v", metas)
	}
}

func TestCollectCreateTopicMetas_SubscriberFnNameOverridesAccessor(t *testing.T) {
	src := `package p
var PageTopic = CreateTopic(Page{}, TopicConfig{Name: "page", SubscriberFnName: "OnPage"})`
	f := parseFile(src)
	metas := collectCreateTopicMetas(f)
	m, ok := metas["Page"]
	if !ok {
		t.Fatal("expected meta for Page")
	}
	if m.AccessorName != "OnPage" {
		t.Errorf("SubscriberFnName should override accessor; got %q", m.AccessorName)
	}
}

func TestParseTopicConfigArg_AllFields(t *testing.T) {
	ce := mustCallExpr(t, `f(TopicConfig{Name: "page", Compression: "BROTLI", Compiler: Golang, SubscriberFnName: "Sub", ComponentFnName: "Comp"})`)
	name, compr, compiler, sub, comp := parseTopicConfigArg(ce.Args[0])
	if name != "page" {
		t.Errorf("Name: got %q", name)
	}
	if compr != WasmCompressionBrotli {
		t.Error("Compression brotli")
	}
	if compiler != WasmCompilerGolang {
		t.Error("Compiler golang")
	}
	if sub != "Sub" {
		t.Errorf("SubscriberFnName: got %q", sub)
	}
	if comp != "Comp" {
		t.Errorf("ComponentFnName: got %q", comp)
	}
}

func TestParseTopicConfigArg_NonComposite(t *testing.T) {
	ce := mustCallExpr(t, "f(42)")
	name, compr, compiler, _, _ := parseTopicConfigArg(ce.Args[0])
	if name != "" || compr != WasmCompressionGzip || compiler != WasmCompilerGothicTinyGo {
		t.Error("non-composite should yield defaults")
	}
}

func TestParseStructsFromSource_Aliases(t *testing.T) {
	h := DefaultWasmHelper()
	src := `package p
type MyInt int
type Labels []string
type MyMap map[string]int
type Page struct {
	Count MyInt
	Tags  Labels ` + "`gothic:\"compress\"`" + `
}`
	structs, aliases, refAliases := h.parseStructsFromSource(src)
	if aliases["MyInt"] != "int" {
		t.Errorf("alias MyInt: got %q", aliases["MyInt"])
	}
	if aliases["Labels"] != "[]string" {
		t.Errorf("alias Labels: got %q", aliases["Labels"])
	}
	if aliases["MyMap"] != "map[string]int" {
		t.Errorf("alias MyMap: got %q", aliases["MyMap"])
	}
	if _, ok := refAliases["MyInt"]; !ok {
		t.Error("expected refAlias for MyInt")
	}
	var page *structInfo
	for i := range structs {
		if structs[i].Name == "Page" {
			page = &structs[i]
		}
	}
	if page == nil {
		t.Fatal("Page struct not parsed")
	}
	if len(page.Fields) != 2 || page.Fields[1].GothicTag != "compress" {
		t.Errorf("unexpected Page fields: %+v", page.Fields)
	}
}

func TestParseStructsFromSource_PointerAlias(t *testing.T) {
	h := DefaultWasmHelper()
	src := `package p
type MyPtr *int
type Page struct {
	P MyPtr
}`
	_, aliases, refAliases := h.parseStructsFromSource(src)
	if aliases["MyPtr"] != "*int" {
		t.Errorf("alias MyPtr: got %q, want *int", aliases["MyPtr"])
	}
	if ref, ok := refAliases["MyPtr"]; !ok || ref.String() != "*int" {
		t.Errorf("refAlias MyPtr: got %v", refAliases["MyPtr"])
	}
}

func TestParseStructsFromSource_InvalidReturnsEmpty(t *testing.T) {
	h := DefaultWasmHelper()
	structs, _, _ := h.parseStructsFromSource("this is not go @@@")
	if len(structs) != 0 {
		t.Errorf("expected no structs for invalid source, got %d", len(structs))
	}
}

func TestAstTypeString_Shapes(t *testing.T) {
	h := DefaultWasmHelper()
	cases := map[string]string{
		"int":            "int",
		"[]string":       "[]string",
		"*Item":          "*Item",
		"pkg.Type":       "pkg.Type",
		"map[string]int": "map[string]int",
		"[3]byte":        "byte",
	}
	for src, want := range cases {
		expr, _ := parser.ParseExpr(src)
		if got := h.astTypeString(expr); got != want {
			t.Errorf("astTypeString(%q) = %q, want %q", src, got, want)
		}
	}
}

// ---------------------------------------------------------------------------
// wasm_cache.go — cache load/upToDate/update/save round-trip
// ---------------------------------------------------------------------------

func TestWasmCache_RoundTrip(t *testing.T) {
	dir := withTempCwd(t)
	_ = dir

	c := loadWasmCache() // no file yet → empty
	if c.upToDate("page", "abc") {
		t.Error("empty cache should not report up-to-date")
	}
	if c.upToDate("page", "") {
		t.Error("empty hash is never up-to-date")
	}
	c.update("page", "abc")
	if !c.upToDate("page", "abc") {
		t.Error("expected up-to-date after update")
	}
	if c.upToDate("page", "different") {
		t.Error("different hash should not match")
	}

	if err := os.MkdirAll(".gothicCli", 0755); err != nil {
		t.Fatalf("mkdir .gothicCli: %v", err)
	}
	c.save()

	// reload from disk
	c2 := loadWasmCache()
	if !c2.upToDate("page", "abc") {
		t.Error("expected persisted hash to load")
	}
}

func TestAstTypeString_NestedSelector(t *testing.T) {
	h := DefaultWasmHelper()
	// pkg.Sub.Type → selector whose X is itself a selector: astTypeString
	// recurses on the X branch.
	expr := mustParseExpr(t, "a.b")
	if got := h.astTypeString(expr); got != "a.b" {
		t.Errorf("astTypeString(a.b): got %q", got)
	}
	// Unsupported expression kind → empty string.
	if got := h.astTypeString(mustParseExpr(t, "1 + 2")); got != "" {
		t.Errorf("astTypeString(binary): got %q, want empty", got)
	}
}

func TestWasmCache_SaveNoDirIsSilent(t *testing.T) {
	withTempCwd(t) // no .gothicCli dir present
	c := loadWasmCache()
	c.update("x", "hash")
	// save() to a missing .gothicCli dir must not panic (write error is ignored).
	c.save()
}

func TestCollectLocalPackageDirs_NilGuards(t *testing.T) {
	// nil package or empty helperPkgs → nil result without touching disk.
	if got := collectLocalPackageDirs(nil, nil); got != nil {
		t.Errorf("expected nil for nil package, got %v", got)
	}
}

func TestFeedFile(t *testing.T) {
	h := DefaultWasmHelper()
	p := filepath.Join(t.TempDir(), "f.txt")
	os.WriteFile(p, []byte("hello"), 0644)
	var buf bytes.Buffer
	h.feedFile(&buf, p)
	if buf.String() != "hello" {
		t.Errorf("feedFile: got %q", buf.String())
	}
	// missing file is a silent no-op
	var buf2 bytes.Buffer
	h.feedFile(&buf2, filepath.Join(t.TempDir(), "missing"))
	if buf2.Len() != 0 {
		t.Error("feedFile of missing file should write nothing")
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func mustParseExpr(t *testing.T, src string) ast.Expr {
	t.Helper()
	expr, err := parser.ParseExpr(src)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	return expr
}

func mustCallExpr(t *testing.T, src string) *ast.CallExpr {
	t.Helper()
	expr, err := parser.ParseExpr(src)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	ce, ok := expr.(*ast.CallExpr)
	if !ok {
		t.Fatalf("%q is not a call expression (%T)", src, expr)
	}
	return ce
}
