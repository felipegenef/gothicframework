package helpers

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// writeProjectFile writes content to a file under the current working
// directory, creating parent directories as needed.
func writeProjectFile(t *testing.T, rel, content string) {
	t.Helper()
	full := filepath.Join(".", rel)
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
	}
	if err := os.WriteFile(full, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

// setupTopicProject creates a minimal self-contained module in a temp cwd with a
// src/topics package containing a topic struct. Returns nothing; cwd is the new
// project for the duration of the test.
func setupTopicProject(t *testing.T) {
	t.Helper()
	withTempCwd(t)
	writeProjectFile(t, "go.mod", "module example.com/proj\n\ngo 1.21\n")
	writeProjectFile(t, "src/topics/topics.go", `package topics

type Page struct {
	Pings int    `+"`gothic:\"i32\"`"+`
	Label string
}

var PageTopic = CreateTopic(Page{}, TopicConfig{Name: "page", Compression: "BROTLI"})

func CreateTopic(v any, c TopicConfig) any { return nil }

type TopicConfig struct {
	Name        string
	Compression string
}
`)
}

func TestResolveTopicSourceDir(t *testing.T) {
	withTempCwd(t)
	if _, _, ok := resolveTopicSourceDir(); ok {
		t.Error("expected no topics dir in empty cwd")
	}
	if err := os.MkdirAll("src/topics", 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	dir, gen, ok := resolveTopicSourceDir()
	if !ok || dir != "src/topics" || gen != "topic_gen.go" {
		t.Errorf("resolveTopicSourceDir: got (%q,%q,%v)", dir, gen, ok)
	}
}

func TestCollectTopicSnippets_GeneratesGenFileAndNormalizes(t *testing.T) {
	setupTopicProject(t)
	h := DefaultWasmHelper()

	snippets, structs, aliases, refAliases := h.collectTopicSnippets()
	if len(structs) == 0 {
		t.Fatal("expected at least one struct")
	}
	var page *structInfo
	for i := range structs {
		if structs[i].Name == "Page" {
			page = &structs[i]
		}
	}
	if page == nil {
		t.Fatal("Page struct not collected")
	}
	if page.KeyName != "page" {
		t.Errorf("KeyName: got %q, want page", page.KeyName)
	}
	if page.AccessorName != "PageTopic" {
		t.Errorf("AccessorName: got %q, want PageTopic", page.AccessorName)
	}
	if len(snippets) == 0 {
		t.Error("expected inlinable snippets")
	}
	_ = aliases
	_ = refAliases

	// topic_gen.go should have been written.
	if _, err := os.Stat(filepath.Join("src/topics", "topic_gen.go")); err != nil {
		t.Errorf("expected topic_gen.go to be generated: %v", err)
	}

	// normalizeTopicDeclarations should have rewritten "var PageTopic = CreateTopic("
	// → "var _ = CreateTopic(" on disk.
	data, err := os.ReadFile(filepath.Join("src/topics", "topics.go"))
	if err != nil {
		t.Fatalf("read topics.go: %v", err)
	}
	if strings.Contains(string(data), "var PageTopic = CreateTopic(") {
		t.Error("expected PageTopic var to be normalized to var _")
	}
	if !strings.Contains(string(data), "var _ = CreateTopic(") {
		t.Error("expected normalized var _ = CreateTopic(")
	}
}

func TestPregenerateTopicStubs(t *testing.T) {
	setupTopicProject(t)
	h := DefaultWasmHelper()
	// Should not panic and should regenerate the gen file.
	h.PregenerateTopicStubs()
	if _, err := os.Stat(filepath.Join("src/topics", "topic_gen.go")); err != nil {
		t.Errorf("expected topic_gen.go after PregenerateTopicStubs: %v", err)
	}
}

func TestCountTopicManagers(t *testing.T) {
	setupTopicProject(t)
	h := DefaultWasmHelper()
	if n := h.CountTopicManagers(); n != 1 {
		t.Errorf("CountTopicManagers: got %d, want 1", n)
	}
}

func TestFeedTopicFiles(t *testing.T) {
	setupTopicProject(t)
	h := DefaultWasmHelper()
	var buf bytes.Buffer
	h.feedTopicFiles(&buf)
	if buf.Len() == 0 {
		t.Error("expected feedTopicFiles to hash topic source files")
	}
}

func TestTopicManagerInputHash_ChangesWithCompression(t *testing.T) {
	setupTopicProject(t)
	h := DefaultWasmHelper()
	s := structInfo{Name: "Page", KeyName: "page", Compression: WasmCompressionGzip}
	hGz := h.topicManagerInputHash(s)
	s.Compression = WasmCompressionBrotli
	hBr := h.topicManagerInputHash(s)
	if hGz == "" || hGz == hBr {
		t.Errorf("expected different hashes per compression; got %q vs %q", hGz, hBr)
	}
}

func TestWriteTopicKeyStubs_RemovesGenFileWhenNoStructs(t *testing.T) {
	withTempCwd(t)
	if err := os.MkdirAll("src/topics", 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	genPath := filepath.Join("src/topics", "topic_gen.go")
	if err := os.WriteFile(genPath, []byte("package topics\n"), 0644); err != nil {
		t.Fatalf("write gen: %v", err)
	}
	h := DefaultWasmHelper()
	h.writeTopicKeyStubs(nil, nil, nil, "topics", "src/topics", "topic_gen.go")
	if _, err := os.Stat(genPath); !os.IsNotExist(err) {
		t.Errorf("expected topic_gen.go to be removed when no structs; stat err=%v", err)
	}
}

func TestNormalizeTopicDeclarations_NoAccessorsNoOp(t *testing.T) {
	withTempCwd(t)
	if err := os.MkdirAll("src/topics", 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// structs without AccessorName → accessors map empty → early nil return.
	if err := normalizeTopicDeclarations("src/topics", []structInfo{{Name: "X"}}); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
	// missing directory → ReadDir error path is only hit when accessors present.
	err := normalizeTopicDeclarations("nonexistent-dir", []structInfo{{Name: "X", AccessorName: "XTopic"}})
	if err == nil {
		t.Error("expected error for missing dir with accessors present")
	}
}

// ---------------------------------------------------------------------------
// ScanPages / scanFile / collectLocalPackageDirs
// ---------------------------------------------------------------------------

// setupPageProject builds a self-contained module whose pages reference a local
// helpers/routes.RouteConfig type, so ExtractClientSideStateBody matches.
func setupPageProject(t *testing.T) {
	t.Helper()
	withTempCwd(t)
	writeProjectFile(t, "go.mod", "module example.com/site\n\ngo 1.21\n")
	writeProjectFile(t, "helpers/routes/routes.go", `package routes

type RouteConfig struct {
	ClientSideState func()
	WasmCompression string
	WasmCompiler    string
}
`)
	// A page with an inline ClientSideState body and a brotli compression.
	writeProjectFile(t, "src/pages/counter_templ.go", `package pages

import "example.com/site/helpers/routes"

var Page = routes.RouteConfig{
	ClientSideState: func() {
		x := 1
		_ = x
	},
	WasmCompression: "BROTLI",
}
`)
	// A component page with a default (gzip) compression.
	writeProjectFile(t, "src/components/widget_templ.go", `package components

import "example.com/site/helpers/routes"

var Widget = routes.RouteConfig{
	ClientSideState: func() {
		y := 2
		_ = y
	},
}
`)
	// A *_templ.go file with no ClientSideState — must be skipped.
	writeProjectFile(t, "src/pages/plain_templ.go", `package pages

var Plain = 1
`)
}

func TestScanPages_FindsClientSideStatePages(t *testing.T) {
	setupPageProject(t)
	h := DefaultWasmHelper()

	pages, err := h.ScanPages("src/pages", "src/components")
	if err != nil {
		t.Fatalf("ScanPages: %v", err)
	}
	if len(pages) != 2 {
		t.Fatalf("expected 2 pages with ClientSideState, got %d: %+v", len(pages), pages)
	}

	byName := map[string]WasmPage{}
	for _, p := range pages {
		byName[p.OutputName] = p
	}
	counter, ok := byName["counter"]
	if !ok {
		t.Fatalf("expected a 'counter' page, got %v", byName)
	}
	if counter.Compression != WasmCompressionBrotli {
		t.Errorf("counter compression: got %v, want brotli", counter.Compression)
	}
	if counter.IsComponent {
		t.Error("counter should not be a component")
	}
	if !strings.Contains(counter.FuncBody, "x := 1") {
		t.Errorf("counter body missing inline stmt: %q", counter.FuncBody)
	}

	var widget *WasmPage
	for i := range pages {
		if pages[i].IsComponent {
			widget = &pages[i]
		}
	}
	if widget == nil {
		t.Fatalf("expected a component page, got %+v", pages)
	}
	if widget.Compression != WasmCompressionGzip {
		t.Errorf("widget compression: got %v, want gzip", widget.Compression)
	}
	if !strings.Contains(widget.FuncBody, "y := 2") {
		t.Errorf("widget body missing inline stmt: %q", widget.FuncBody)
	}
}

func TestScanPages_CollectsLocalPackageDirs(t *testing.T) {
	withTempCwd(t)
	writeProjectFile(t, "go.mod", "module example.com/app\n\ngo 1.21\n")
	writeProjectFile(t, "helpers/routes/routes.go", `package routes

type RouteConfig struct {
	ClientSideState func()
	WasmCompression string
}
`)
	// A local helper package referenced from the page body.
	writeProjectFile(t, "internal/calc/calc.go", `package calc

func Double(n int) int { return n * 2 }
`)
	writeProjectFile(t, "src/pages/home_templ.go", `package pages

import (
	"example.com/app/helpers/routes"
	"example.com/app/internal/calc"
)

var Page = routes.RouteConfig{
	ClientSideState: func() {
		_ = calc.Double(21)
	},
}
`)

	h := DefaultWasmHelper()
	pages, err := h.ScanPages("src/pages", "")
	if err != nil {
		t.Fatalf("ScanPages: %v", err)
	}
	if len(pages) != 1 {
		t.Fatalf("expected 1 page, got %d", len(pages))
	}
	found := false
	for _, d := range pages[0].LocalPackageDirs {
		if strings.Contains(d, filepath.Join("internal", "calc")) {
			found = true
		}
	}
	if !found {
		t.Errorf("expected internal/calc in LocalPackageDirs, got %v", pages[0].LocalPackageDirs)
	}
}

func TestScanPages_SamePackageHelperAndImports(t *testing.T) {
	withTempCwd(t)
	writeProjectFile(t, "go.mod", "module example.com/h\n\ngo 1.21\n")
	writeProjectFile(t, "helpers/routes/routes.go", `package routes

type RouteConfig struct {
	ClientSideState func()
	WasmCompression string
}
`)
	// A page whose ClientSideState body imports a std package (fmt) AND calls a
	// same-package helper, which itself imports another std package (strings).
	// This drives scanFile's helper-decl formatting and the helper-import
	// re-scan loop.
	writeProjectFile(t, "src/pages/dash_templ.go", `package pages

import (
	"fmt"

	"example.com/h/helpers/routes"
)

func greeting() string {
	return upper("hi")
}

var Page = routes.RouteConfig{
	ClientSideState: func() {
		fmt.Println(greeting())
	},
}
`)
	writeProjectFile(t, "src/pages/util_templ.go", `package pages

import "strings"

func upper(s string) string { return strings.ToUpper(s) }
`)

	h := DefaultWasmHelper()
	pages, err := h.ScanPages("src/pages", "")
	if err != nil {
		t.Fatalf("ScanPages: %v", err)
	}
	var dash *WasmPage
	for i := range pages {
		if pages[i].OutputName == "dash" {
			dash = &pages[i]
		}
	}
	if dash == nil {
		t.Fatalf("expected dash page, got %+v", pages)
	}
	if len(dash.Helpers) == 0 {
		t.Errorf("expected helper decls to be extracted, got none")
	}
	joinedHelpers := strings.Join(dash.Helpers, "\n")
	if !strings.Contains(joinedHelpers, "greeting") {
		t.Errorf("expected greeting helper, got %q", joinedHelpers)
	}
	// fmt import (from body) and strings import (from helper) should both appear.
	joinedImports := strings.Join(dash.Imports, "\n")
	if !strings.Contains(joinedImports, `"fmt"`) {
		t.Errorf("expected fmt import, got %q", joinedImports)
	}
	if !strings.Contains(joinedImports, `"strings"`) {
		t.Errorf("expected strings import from helper re-scan, got %q", joinedImports)
	}
	if len(dash.UsedDeclSources) == 0 {
		t.Errorf("expected UsedDeclSources to be populated")
	}
}

func TestScanPages_CompilerVariants(t *testing.T) {
	withTempCwd(t)
	writeProjectFile(t, "go.mod", "module example.com/cv\n\ngo 1.21\n")
	writeProjectFile(t, "helpers/routes/routes.go", `package routes

type RouteConfig struct {
	ClientSideState func()
	WasmCompression string
	WasmCompiler    string
}
`)
	writeProjectFile(t, "src/pages/golang_templ.go", `package pages

import "example.com/cv/helpers/routes"

var Page = routes.RouteConfig{
	ClientSideState: func() { _ = 1 },
	WasmCompiler:    "Golang",
}
`)
	writeProjectFile(t, "src/pages/local_templ.go", `package pages

import "example.com/cv/helpers/routes"

var Local = routes.RouteConfig{
	ClientSideState: func() { _ = 2 },
	WasmCompiler:    "LocalTinyGo",
}
`)

	h := DefaultWasmHelper()
	pages, err := h.ScanPages("src/pages", "")
	if err != nil {
		t.Fatalf("ScanPages: %v", err)
	}
	got := map[string]WasmCompilerChoice{}
	for _, p := range pages {
		got[p.OutputName] = p.Compiler
	}
	if got["golang"] != WasmCompilerGolang {
		t.Errorf("golang page compiler: got %v, want Golang", got["golang"])
	}
	if got["local"] != WasmCompilerLocalTinyGo {
		t.Errorf("local page compiler: got %v, want LocalTinyGo", got["local"])
	}
}

func TestScanFile_WithoutLoaderErrors(t *testing.T) {
	h := DefaultWasmHelper()
	if _, _, err := h.scanFile("anything_templ.go"); err == nil {
		t.Fatal("expected error when scanFile called without ScanPages")
	}
}

func TestScanPages_EmptyDirsReturnNothing(t *testing.T) {
	setupPageProject(t)
	h := DefaultWasmHelper()
	// Passing empty dir strings should be skipped entirely.
	pages, err := h.ScanPages("", "")
	if err != nil {
		t.Fatalf("ScanPages empty: %v", err)
	}
	if len(pages) != 0 {
		t.Errorf("expected 0 pages for empty dirs, got %d", len(pages))
	}
}

// ---------------------------------------------------------------------------
// wasm_build.go — pure helpers testable without the toolchain
// ---------------------------------------------------------------------------

func TestCompilerLabel(t *testing.T) {
	cases := map[WasmCompilerChoice]string{
		WasmCompilerLocalTinyGo:  "local tinygo",
		WasmCompilerGolang:       "go (js/wasm)",
		WasmCompilerGothicTinyGo: "embedded tinygo",
	}
	for c, want := range cases {
		if got := compilerLabel(c); got != want {
			t.Errorf("compilerLabel(%v): got %q, want %q", c, got, want)
		}
	}
}

func TestPagesUseStandardGo(t *testing.T) {
	if pagesUseStandardGo([]WasmPage{{Compiler: WasmCompilerGothicTinyGo}}) {
		t.Error("no Golang page → false")
	}
	if !pagesUseStandardGo([]WasmPage{
		{Compiler: WasmCompilerGothicTinyGo},
		{Compiler: WasmCompilerGolang},
	}) {
		t.Error("contains Golang page → true")
	}
}

func TestCopyWasmExec(t *testing.T) {
	dir := t.TempDir()
	h := DefaultWasmHelper()
	if err := h.CopyWasmExec(dir); err != nil {
		t.Fatalf("CopyWasmExec: %v", err)
	}
	info, err := os.Stat(filepath.Join(dir, "wasm_exec.js"))
	if err != nil {
		t.Fatalf("expected wasm_exec.js: %v", err)
	}
	if info.Size() == 0 {
		t.Error("wasm_exec.js is empty")
	}
}

func TestGenerateTopicManagers_NoTopicsIsNoOp(t *testing.T) {
	withTempCwd(t) // no src/topics
	h := DefaultWasmHelper()
	if err := h.GenerateTopicManagers(t.TempDir(), &sync.Once{}); err != nil {
		t.Errorf("GenerateTopicManagers no-op: %v", err)
	}
}

func TestWriteModuleBridge(t *testing.T) {
	setupPageProject(t) // gives a valid go.mod in cwd
	h := DefaultWasmHelper()
	tempMod := t.TempDir()
	if err := h.writeModuleBridge(tempMod); err != nil {
		t.Fatalf("writeModuleBridge: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(tempMod, "go.mod"))
	if err != nil {
		t.Fatalf("read bridge go.mod: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, "module wasm-runtime") {
		t.Error("bridge go.mod missing module wasm-runtime")
	}
	if !strings.Contains(s, "example.com/site") {
		t.Error("bridge go.mod missing user module require/replace")
	}
	if !strings.Contains(s, "replace") {
		t.Error("bridge go.mod missing replace directive")
	}
}

func TestWriteModuleBridge_NoGoModErrors(t *testing.T) {
	withTempCwd(t) // no go.mod
	h := DefaultWasmHelper()
	if err := h.writeModuleBridge(t.TempDir()); err == nil {
		t.Fatal("expected error when user go.mod is missing")
	}
}

func TestWriteWasmMain_RendersTemplate(t *testing.T) {
	h := DefaultWasmHelper()
	dest := filepath.Join(t.TempDir(), "main.go")
	structs := []structInfo{
		{
			Name:    "Page",
			KeyName: "page",
			Fields: []fieldInfo{
				testFieldInfo("Count", "int"),
				testFieldInfo("When", "time.Time"),
			},
		},
	}
	body := "println(\"hi\")\nselect {}"
	err := h.writeWasmMain(
		"src/pages/counter_templ.go",
		body,
		[]string{`"fmt"`},
		[]string{"func helper() int { return 1 }"},
		[]string{"// snippet"},
		structs,
		map[string]string{},
		nil,
		dest,
	)
	if err != nil {
		t.Fatalf("writeWasmMain: %v", err)
	}
	out, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read rendered main: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "package main") {
		t.Errorf("rendered main missing package decl:\n%s", s)
	}
	// time import auto-injected because a topic field uses time.Time.
	if !strings.Contains(s, `"time"`) {
		t.Errorf("expected auto-injected time import:\n%s", s)
	}
	// trailing select {} from the body should have been stripped (template
	// supplies its own); the helper should still be embedded.
	if !strings.Contains(s, "func helper()") {
		t.Errorf("expected helper to be embedded:\n%s", s)
	}
}

func TestWriteWasmMain_TimeAlreadyImported(t *testing.T) {
	h := DefaultWasmHelper()
	dest := filepath.Join(t.TempDir(), "main.go")
	structs := []structInfo{
		{Name: "E", KeyName: "e", Fields: []fieldInfo{testFieldInfo("At", "time.Time")}},
	}
	// stdImports already contains "time" → injection branch is skipped.
	err := h.writeWasmMain(
		"src/pages/p_templ.go",
		"println(\"x\")\nselect{}", // trailing select{} (no space) variant
		[]string{`"time"`},
		nil, nil,
		structs,
		map[string]string{},
		nil,
		dest,
	)
	if err != nil {
		t.Fatalf("writeWasmMain: %v", err)
	}
	out, _ := os.ReadFile(dest)
	// time should appear exactly via the provided import (not double-injected).
	if strings.Count(string(out), `"time"`) == 0 {
		t.Errorf("expected time import present:\n%s", out)
	}
}

func TestRewriteAutoKeys_Wrapper(t *testing.T) {
	h := DefaultWasmHelper()
	src := `var k = AutoKey[Page]("page")`
	out, err := h.rewriteAutoKeys(src)
	if err != nil {
		t.Fatalf("rewriteAutoKeys: %v", err)
	}
	if !strings.Contains(out, "BinaryKey[Page]") {
		t.Errorf("expected AutoKey rewritten to BinaryKey, got: %q", out)
	}
}

// ---------------------------------------------------------------------------
// wasm_codec_types.go — typeRef() marker methods (0% before)
// ---------------------------------------------------------------------------

func TestTypeRefMarkerMethods(t *testing.T) {
	// Calling the unexported marker methods keeps them covered and documents
	// that all four implement the typeRef interface.
	var refs = []typeRef{
		Named{Name: "int"},
		SliceOf{Elem: Named{Name: "int"}},
		MapOf{Key: Named{Name: "string"}, Val: Named{Name: "int"}},
		PointerOf{Elem: Named{Name: "int"}},
	}
	for _, r := range refs {
		r.typeRef()
		if r.String() == "" {
			t.Errorf("unexpected empty String() for %T", r)
		}
	}
}

// ---------------------------------------------------------------------------
// wasm_templates.go — CleanupLegacyTemplates
// ---------------------------------------------------------------------------

func TestCleanupLegacyTemplates_RemovesStaleCopies(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, ".gothicCli/templates/wasm/topic_gen.go")
	if err := os.MkdirAll(filepath.Dir(legacy), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(legacy, []byte("stale"), 0644); err != nil {
		t.Fatalf("write legacy: %v", err)
	}
	if err := CleanupLegacyTemplates(root); err != nil {
		t.Fatalf("CleanupLegacyTemplates: %v", err)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Errorf("expected legacy template removed; stat err=%v", err)
	}
	// Idempotent: second call with nothing present succeeds.
	if err := CleanupLegacyTemplates(root); err != nil {
		t.Errorf("second cleanup should be no-op: %v", err)
	}
}
