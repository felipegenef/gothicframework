package helpers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"
)

// --- WasmOutputName ---------------------------------------------------------

func TestWasmOutputName(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/", "index"},
		{"", "index"},
		{"/counter", "counter"},
		{"/user/{id}", "user-id"},
		{"/blog/{slug}/comments", "blog-slug-comments"},
		{"/a/b/c", "a-b-c"},
	}
	for _, tt := range tests {
		if got := WasmOutputName(tt.path); got != tt.want {
			t.Errorf("WasmOutputName(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

// --- wasmInjectedComponent.Render & TopicManagerComponent -------------------

func TestWasmInjectedComponentRender(t *testing.T) {
	inner := mockComponent("<html><body><h1>hi</h1></body></html>")
	comp := &wasmInjectedComponent{
		inner:       inner,
		wasmName:    "counter",
		compression: GZIP,
		compiler:    GothicTinyGo,
	}

	var sb strings.Builder
	if err := comp.Render(context.Background(), &sb); err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	out := sb.String()
	if !strings.Contains(out, "<h1>hi</h1>") {
		t.Errorf("expected inner content preserved, got %q", out)
	}
	// Envelope injects a reference to the wasm asset name.
	if !strings.Contains(out, "counter") {
		t.Errorf("expected wasm name injected into envelope, got %q", out)
	}
}

func TestTopicManagerComponent(t *testing.T) {
	comp := TopicManagerComponent("notifications", BROTLI)
	if comp == nil {
		t.Fatal("expected non-nil component")
	}
	var sb strings.Builder
	if err := comp.Render(context.Background(), &sb); err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	if !strings.Contains(sb.String(), "notifications") {
		t.Errorf("expected topic manager name injected, got %q", sb.String())
	}
}

func TestEmptyComponentRender(t *testing.T) {
	var sb strings.Builder
	if err := (emptyComponent{}).Render(context.Background(), &sb); err != nil {
		t.Fatalf("emptyComponent.Render error: %v", err)
	}
	if sb.Len() != 0 {
		t.Errorf("expected empty output, got %q", sb.String())
	}
}

func TestRegisterRouteWithClientSideState(t *testing.T) {
	resetGlobalCache()
	InitCache(CACHE_CONTROL_HEADERS, nil)
	defer resetGlobalCache()

	config := RouteConfig[string]{
		Type:            DYNAMIC,
		HttpMethod:      GET,
		ClientSideState: func() {},
		WasmCompression: GZIP,
		WasmCompiler:    GothicTinyGo,
		Middleware: func(w http.ResponseWriter, r *http.Request) string {
			return "state"
		},
	}

	r := chi.NewRouter()
	config.RegisterRoute(r, "/stateful", func(props string) templ.Component {
		return mockComponent("<html><body>" + props + "</body></html>")
	})

	req := httptest.NewRequest("GET", "/stateful", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "state") {
		t.Errorf("expected rendered state content, got %q", rec.Body.String())
	}
	// wasmInjectedComponent should have run via the wrapped component.
	if !strings.Contains(rec.Body.String(), "stateful") {
		t.Errorf("expected wasm envelope for route name, got %q", rec.Body.String())
	}
}

// --- wasmAwareFileServer / setup --------------------------------------------

func TestWasmAwareFileServer(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "app.wasm.gz"), []byte("gzipped"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "app.wasm.br"), []byte("brotli"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plain.txt"), []byte("plain"), 0o644); err != nil {
		t.Fatal(err)
	}

	fileServer := http.StripPrefix("/public/", http.FileServer(http.Dir(dir)))
	handler := wasmAwareFileServer(fileServer)

	cases := []struct {
		path        string
		wantType    string
		wantEncoding string
	}{
		{"/public/app.wasm.gz", "application/wasm", "gzip"},
		{"/public/app.wasm.br", "application/wasm", "br"},
		{"/public/plain.txt", "", ""},
	}
	for _, c := range cases {
		req := httptest.NewRequest("GET", c.path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("%s: expected 200, got %d", c.path, rec.Code)
			continue
		}
		if c.wantEncoding != "" {
			if got := rec.Header().Get("Content-Encoding"); got != c.wantEncoding {
				t.Errorf("%s: Content-Encoding = %q, want %q", c.path, got, c.wantEncoding)
			}
			if got := rec.Header().Get("Content-Type"); got != c.wantType {
				t.Errorf("%s: Content-Type = %q, want %q", c.path, got, c.wantType)
			}
		}
	}
}

func TestNoCacheMiddleware(t *testing.T) {
	var sawConditional bool
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") != "" || r.Header.Get("If-Modified-Since") != "" {
			sawConditional = true
		}
		w.WriteHeader(http.StatusOK)
	})

	handler := noCacheMiddleware(next)
	req := httptest.NewRequest("GET", "/public/x.css", nil)
	req.Header.Set("If-None-Match", `"abc"`)
	req.Header.Set("If-Modified-Since", "Mon, 01 Jan 2024 00:00:00 GMT")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if sawConditional {
		t.Error("expected conditional headers to be stripped before next handler")
	}
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "no-store") {
		t.Errorf("expected no-store Cache-Control, got %q", cc)
	}
}

func TestSetupDevMode(t *testing.T) {
	t.Setenv("GOTHIC_MODE", "dev")
	resetGlobalCache()
	defer resetGlobalCache()

	config := AppConfig{
		CacheStrategy:         CACHE_CONTROL_HEADERS,
		LocalDevelopmentCache: IN_MEMORY,
		ServeStaticFiles:      ALL_ENVS,
	}

	r := chi.NewRouter()
	registered := false
	Setup(r, config, func(sub chi.Router) {
		registered = true
		sub.Get("/ping", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("pong"))
		})
	})

	if !registered {
		t.Error("expected registerRoutes callback to run")
	}

	req := httptest.NewRequest("GET", "/ping", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Body.String() != "pong" {
		t.Errorf("expected registered route to respond, got %q", rec.Body.String())
	}
}

// --- compression methods on cache stores ------------------------------------

func TestCompressionMethodsDefaultGZIP(t *testing.T) {
	// Stores with no compression method configured fall back to GZIP.
	mem := &InMemoryCacheStore{config: nil}
	if mem.compressionMethod() != GZIP {
		t.Error("InMemoryCacheStore default compressionMethod should be GZIP")
	}
	memB := &InMemoryCacheStore{config: &CacheConfig{CompressionMethod: BROTLI}}
	if memB.compressionMethod() != BROTLI {
		t.Error("InMemoryCacheStore should honor configured BROTLI")
	}

	files := &LocalFilesCacheStore{config: nil}
	if files.compressionMethod() != GZIP {
		t.Error("LocalFilesCacheStore default compressionMethod should be GZIP")
	}
	filesB := &LocalFilesCacheStore{config: &CacheConfig{CompressionMethod: BROTLI}}
	if filesB.compressionMethod() != BROTLI {
		t.Error("LocalFilesCacheStore should honor configured BROTLI")
	}

	redis := &RedisCacheStore{config: nil}
	if redis.compressionMethod() != GZIP {
		t.Error("RedisCacheStore default compressionMethod should be GZIP")
	}
	if redis.compressionEnabled() {
		t.Error("RedisCacheStore with nil config should report compression disabled")
	}
	redisB := &RedisCacheStore{config: &CacheConfig{Compression: true, CompressionMethod: BROTLI}}
	if redisB.compressionMethod() != BROTLI {
		t.Error("RedisCacheStore should honor configured BROTLI")
	}
	if !redisB.compressionEnabled() {
		t.Error("RedisCacheStore with Compression:true should report enabled")
	}
}

func TestNewRedisCacheStoreRequiresURL(t *testing.T) {
	if _, err := NewRedisCacheStore(nil); err == nil {
		t.Error("expected error for nil config")
	}
	if _, err := NewRedisCacheStore(&CacheConfig{}); err == nil {
		t.Error("expected error for empty RedisURL")
	}
}

func TestCompressRoundTripBrotli(t *testing.T) {
	original := []byte(strings.Repeat("gothic-data-", 100))
	compressed, err := compressData(original, BROTLI)
	if err != nil {
		t.Fatalf("compressData(BROTLI): %v", err)
	}
	out, err := decompressData(compressed, BROTLI)
	if err != nil {
		t.Fatalf("decompressData(BROTLI): %v", err)
	}
	if string(out) != string(original) {
		t.Error("brotli round-trip mismatch")
	}
}

// --- collect* + Render walk over a fixture tree -----------------------------

func writeFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newHelperWithFixtures(t *testing.T) (FileBasedRouteHelper, string) {
	t.Helper()
	root := t.TempDir()
	srcDir := filepath.Join(root, "src")

	// collect* functions resolve import paths via filepath.Rel("src", ...), so the
	// process working directory must be the fixture root for the duration of the test.
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(origWd) })

	// A page that returns a templ.Component with a custom RouteConfig.
	writeFixture(t, filepath.Join(srcDir, "pages", "index_templ.go"), `package pages

import "github.com/a-h/templ"

var IndexConfig = routes.RouteConfig[any]{}

func Index(props any) templ.Component { return nil }
`)
	// A component using the default config (no var declared).
	writeFixture(t, filepath.Join(srcDir, "components", "navbar_templ.go"), `package components

import "github.com/a-h/templ"

func Navbar(props any) templ.Component { return nil }
`)
	// An API route with a handler func (no return).
	writeFixture(t, filepath.Join(srcDir, "api", "health.go"), `package api

import "net/http"

var HealthApi = routes.ApiRouteConfig{}

func Health(w http.ResponseWriter, r *http.Request) {}
`)

	// Use relative folders (the default layout) now that wd == root.
	helper := NewFileBasedRouteHelper()
	helper.PageRoutesFolder = "./src/pages"
	helper.ComponentRoutesFolder = "./src/components"
	helper.ApiRoutesFolder = "./src/api"
	helper.OutputFile = "./routes_gen.go"
	return helper, root
}

func TestCollectInfoFunctions(t *testing.T) {
	helper, _ := newHelperWithFixtures(t)
	helper.Initialize("example.com/mymod")

	if err := helper.collectPageInfo("example.com/mymod"); err != nil {
		t.Fatalf("collectPageInfo: %v", err)
	}
	if err := helper.collectComponentsInfo("example.com/mymod"); err != nil {
		t.Fatalf("collectComponentsInfo: %v", err)
	}
	if err := helper.collectApiRoutesInfo("example.com/mymod"); err != nil {
		t.Fatalf("collectApiRoutesInfo: %v", err)
	}

	if len(helper.TemplateInfo.Routes) != 2 {
		t.Errorf("expected 2 page+component routes, got %d: %+v", len(helper.TemplateInfo.Routes), helper.TemplateInfo.Routes)
	}
	if len(helper.TemplateInfo.ApiRoutes) != 1 {
		t.Errorf("expected 1 api route, got %d", len(helper.TemplateInfo.ApiRoutes))
	}

	// Verify the custom RouteConfig name was picked up.
	var foundIndex bool
	for _, r := range helper.TemplateInfo.Routes {
		if r.FunctionName == "Index" && r.ConfigName == "IndexConfig" {
			foundIndex = true
		}
	}
	if !foundIndex {
		t.Error("expected Index route with custom IndexConfig")
	}
}

func TestCollectInfoMissingFolder(t *testing.T) {
	helper := NewFileBasedRouteHelper()
	helper.PageRoutesFolder = filepath.Join(t.TempDir(), "does-not-exist")
	helper.Initialize("example.com/mymod")

	// filepath.Walk on a missing root returns an error; collectPageInfo wraps it.
	if err := helper.collectPageInfo("example.com/mymod"); err == nil {
		t.Error("expected error walking a missing pages folder")
	}
}

func TestPruneMissingFiles(t *testing.T) {
	helper, root := newHelperWithFixtures(t)
	helper.Initialize("example.com/mymod")

	// Add one valid route (the fixture file exists) and one stale route.
	existing := filepath.Join(root, "src", "pages", "index_templ.go")
	helper.TemplateInfo.Routes = []RouteTemplate{
		{FunctionName: "Index", OriginFile: existing, PackageName: "pages"},
		{FunctionName: "Ghost", OriginFile: filepath.Join(root, "src", "pages", "ghost_templ.go"), PackageName: "ghost"},
	}
	helper.TemplateInfo.Imports = []Imports{
		{Package: "pages", PackagePath: "example.com/mymod/src/pages"},
		{Package: "ghost", PackagePath: "example.com/mymod/src/ghost"},
	}

	helper.pruneMissingFiles()

	if len(helper.TemplateInfo.Routes) != 1 {
		t.Errorf("expected 1 surviving route after prune, got %d", len(helper.TemplateInfo.Routes))
	}
	if helper.TemplateInfo.Routes[0].FunctionName != "Index" {
		t.Errorf("expected surviving route to be Index, got %q", helper.TemplateInfo.Routes[0].FunctionName)
	}
	// The ghost import is unused after pruning and must be dropped.
	for _, imp := range helper.TemplateInfo.Imports {
		if imp.Package == "ghost" {
			t.Error("expected unused ghost import to be pruned")
		}
	}
}

func TestFileBasedRouteHelperRender(t *testing.T) {
	helper, root := newHelperWithFixtures(t)

	if err := helper.Render("example.com/mymod"); err != nil {
		t.Fatalf("Render: %v", err)
	}

	out := filepath.Join(root, "routes_gen.go")
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("expected generated routes file: %v", err)
	}
	gen := string(data)
	if !strings.Contains(gen, "Index") {
		t.Errorf("expected generated file to reference Index route, got:\n%s", gen)
	}
	if !strings.Contains(gen, "Health") {
		t.Errorf("expected generated file to reference Health api route, got:\n%s", gen)
	}
}
