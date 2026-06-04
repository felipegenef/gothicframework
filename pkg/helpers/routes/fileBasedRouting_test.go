package helpers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"
)

func TestNormalizeHttpPath(t *testing.T) {
	helper := NewFileBasedRouteHelper()

	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "pages index route",
			path:     "src/pages/index_templ.go",
			expected: "/",
		},
		{
			name:     "pages about route",
			path:     "src/pages/about_templ.go",
			expected: "/about",
		},
		{
			name:     "pages nested route",
			path:     "src/pages/blog/post_templ.go",
			expected: "/blog/post",
		},
		{
			name:     "pages nested index route",
			path:     "src/pages/blog/index_templ.go",
			expected: "/blog",
		},
		{
			name:     "pages dynamic route param",
			path:     "src/pages/blog/var_id_templ.go",
			expected: "/blog/{id}",
		},
		{
			name:     "components route",
			path:     "src/components/navbar_templ.go",
			expected: "/components/navbar",
		},
		{
			name:     "api route with .go extension",
			path:     "src/api/users.go",
			expected: "/api/users",
		},
		{
			name:     "api nested route",
			path:     "src/api/v1/health.go",
			expected: "/api/v1/health",
		},
		{
			name:     "api dynamic route param",
			path:     "src/api/users/var_id.go",
			expected: "/api/users/{id}",
		},
		{
			name:     "api nested dynamic route param",
			path:     "src/api/v1/posts/var_postId.go",
			expected: "/api/v1/posts/{postId}",
		},
		{
			name:     "regex anchor: simple var_ prefix",
			path:     "src/pages/var_foo",
			expected: "/{foo}",
		},
		{
			name:     "regex anchor: adjacent var_ tokens treated as one identifier",
			path:     "src/pages/var_foovar_bar",
			expected: "/{foovar_bar}",
		},
		{
			name:     "regex anchor: identifier starting with digit is not a param",
			path:     "src/pages/var_0bad",
			expected: "/var_0bad",
		},
		{
			name:     "regex anchor: no word boundary before var_ means no match",
			path:     "src/pages/_var_hidden",
			expected: "/_var_hidden",
		},
		{
			name:     "deeply nested pages",
			path:     "src/pages/admin/settings/profile_templ.go",
			expected: "/admin/settings/profile",
		},
		{
			name:     "root index only",
			path:     "src/pages/index_templ.go",
			expected: "/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := helper.normalizeHttpPath(tt.path)
			if got != tt.expected {
				t.Errorf("normalizeHttpPath(%q) = %q, want %q", tt.path, got, tt.expected)
			}
		})
	}
}

func TestRemoveDuplicates(t *testing.T) {
	helper := NewFileBasedRouteHelper()
	helper.TemplateInfo.Imports = []Imports{
		{Package: "pages", PackagePath: "example.com/src/pages"},
		{Package: "pages", PackagePath: "example.com/src/pages"},
		{Package: "components", PackagePath: "example.com/src/components"},
	}
	helper.TemplateInfo.Routes = []RouteTemplate{
		{ConfigName: "DefaultConfig", PackageName: "pages"},
	}

	helper.RemoveDuplicates()

	if !helper.TemplateInfo.ImportDefault {
		t.Error("expected ImportDefault to be true when DefaultConfig is used")
	}

	if len(helper.TemplateInfo.Imports) != 2 {
		t.Errorf("expected 2 unique imports, got %d", len(helper.TemplateInfo.Imports))
	}
}

func TestInitialize(t *testing.T) {
	helper := NewFileBasedRouteHelper()
	helper.TemplateInfo.Routes = []RouteTemplate{{FunctionName: "old"}}
	helper.TemplateInfo.ApiRoutes = []RouteTemplate{{FunctionName: "old"}}
	helper.TemplateInfo.ImportDefault = true

	helper.Initialize("example.com/mymod")

	if len(helper.TemplateInfo.Routes) != 0 {
		t.Error("expected Routes to be empty after Initialize")
	}
	if len(helper.TemplateInfo.ApiRoutes) != 0 {
		t.Error("expected ApiRoutes to be empty after Initialize")
	}
	if helper.TemplateInfo.GoModName != "example.com/mymod" {
		t.Errorf("expected GoModName to be 'example.com/mymod', got %q", helper.TemplateInfo.GoModName)
	}
	if helper.TemplateInfo.ImportDefault {
		t.Error("expected ImportDefault to be false after Initialize")
	}
}

// mockComponent returns a templ.Component that writes the given HTML string.
func mockComponent(html string) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, err := w.Write([]byte(html))
		return err
	})
}

func TestRegisterRouteStaticCACHE_CONTROL_HEADERS(t *testing.T) {
	// CACHE_CONTROL_HEADERS (default): should set Cache-Control header
	resetGlobalCache()
	InitCache(CACHE_CONTROL_HEADERS, nil)

	config := RouteConfig[string]{
		Type:       STATIC,
		HttpMethod: GET,
		Middleware: func(w http.ResponseWriter, r *http.Request) string {
			return "hello"
		},
	}

	r := chi.NewRouter()
	config.RegisterRoute(r, "/test", func(props string) templ.Component {
		return mockComponent("<p>" + props + "</p>")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "max-age=31536000" {
		t.Errorf("expected Cache-Control max-age=31536000, got %q", cc)
	}
	if body := rec.Body.String(); body != "<p>hello</p>" {
		t.Errorf("expected '<p>hello</p>', got %q", body)
	}

	resetGlobalCache()
}

func TestRegisterRouteStaticInMemory(t *testing.T) {
	resetGlobalCache()
	InitCache(IN_MEMORY, nil)

	config := RouteConfig[string]{
		Type:       STATIC,
		HttpMethod: GET,
		Middleware: func(w http.ResponseWriter, r *http.Request) string {
			return "cached"
		},
	}

	r := chi.NewRouter()
	config.RegisterRoute(r, "/mem", func(props string) templ.Component {
		return mockComponent("<div>" + props + "</div>")
	})

	// First request: cache miss
	req := httptest.NewRequest("GET", "/mem", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if body := rec.Body.String(); body != "<div>cached</div>" {
		t.Errorf("first request: expected '<div>cached</div>', got %q", body)
	}

	// Second request: cache hit (same content)
	req2 := httptest.NewRequest("GET", "/mem", nil)
	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, req2)

	if body := rec2.Body.String(); body != "<div>cached</div>" {
		t.Errorf("second request: expected '<div>cached</div>', got %q", body)
	}

	resetGlobalCache()
}

func TestRegisterRouteDynamic(t *testing.T) {
	resetGlobalCache()
	InitCache(CACHE_CONTROL_HEADERS, nil)

	callCount := 0
	config := RouteConfig[int]{
		Type:       DYNAMIC,
		HttpMethod: GET,
		Middleware: func(w http.ResponseWriter, r *http.Request) int {
			callCount++
			return callCount
		},
	}

	r := chi.NewRouter()
	config.RegisterRoute(r, "/dyn", func(props int) templ.Component {
		return mockComponent("<span>dynamic</span>")
	})

	// Each request should call middleware
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("GET", "/dyn", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
	}

	if callCount != 3 {
		t.Errorf("expected middleware called 3 times, got %d", callCount)
	}

	resetGlobalCache()
}

func TestRegisterRouteISRInMemory(t *testing.T) {
	resetGlobalCache()
	InitCache(IN_MEMORY, nil)

	callCount := 0
	config := RouteConfig[string]{
		Type:            ISR,
		HttpMethod:      GET,
		RevalidateInSec: 60,
		Middleware: func(w http.ResponseWriter, r *http.Request) string {
			callCount++
			return "isr-data"
		},
	}

	r := chi.NewRouter()
	config.RegisterRoute(r, "/isr", func(props string) templ.Component {
		return mockComponent("<h1>" + props + "</h1>")
	})

	// First request: cache miss
	req := httptest.NewRequest("GET", "/isr", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if body := rec.Body.String(); body != "<h1>isr-data</h1>" {
		t.Errorf("expected '<h1>isr-data</h1>', got %q", body)
	}

	// Second request: cache hit (middleware not called again)
	req2 := httptest.NewRequest("GET", "/isr", nil)
	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, req2)

	if callCount != 1 {
		t.Errorf("expected middleware called once (cached), got %d", callCount)
	}
	if body := rec2.Body.String(); body != "<h1>isr-data</h1>" {
		t.Errorf("cache hit: expected '<h1>isr-data</h1>', got %q", body)
	}

	resetGlobalCache()
}

func TestRegisterRouteStaticLocalFiles(t *testing.T) {
	tmpDir := t.TempDir()
	resetGlobalCache()
	InitCache(LOCAL_FILES, &CacheConfig{CacheFilesPath: tmpDir})

	config := RouteConfig[string]{
		Type:       STATIC,
		HttpMethod: GET,
		Middleware: func(w http.ResponseWriter, r *http.Request) string {
			return "file-cached"
		},
	}

	r := chi.NewRouter()
	config.RegisterRoute(r, "/files", func(props string) templ.Component {
		return mockComponent("<p>" + props + "</p>")
	})

	req := httptest.NewRequest("GET", "/files", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if body := rec.Body.String(); body != "<p>file-cached</p>" {
		t.Errorf("expected '<p>file-cached</p>', got %q", body)
	}

	// Second request hits file cache
	req2 := httptest.NewRequest("GET", "/files", nil)
	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, req2)

	if body := rec2.Body.String(); body != "<p>file-cached</p>" {
		t.Errorf("cache hit: expected '<p>file-cached</p>', got %q", body)
	}

	resetGlobalCache()
}

func TestApiRegisterRouteDynamic(t *testing.T) {
	resetGlobalCache()
	InitCache(IN_MEMORY, nil)

	callCount := 0
	config := ApiRouteConfig{
		Type:       DYNAMIC,
		HttpMethod: GET,
	}

	r := chi.NewRouter()
	config.RegisterRoute(r, "/api/dyn", func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"count":` + fmt.Sprintf("%d", callCount) + `}`))
	})

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("GET", "/api/dyn", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
	}

	if callCount != 3 {
		t.Errorf("expected handler called 3 times for DYNAMIC, got %d", callCount)
	}

	resetGlobalCache()
}

func TestApiRegisterRouteStaticCACHE_CONTROL_HEADERS(t *testing.T) {
	resetGlobalCache()
	InitCache(CACHE_CONTROL_HEADERS, nil)

	config := ApiRouteConfig{
		Type:       STATIC,
		HttpMethod: GET,
	}

	r := chi.NewRouter()
	config.RegisterRoute(r, "/api/static-cc", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	})

	req := httptest.NewRequest("GET", "/api/static-cc", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "max-age=31536000" {
		t.Errorf("expected Cache-Control max-age=31536000, got %q", cc)
	}
	if body := rec.Body.String(); body != `{"ok":true}` {
		t.Errorf("expected '{\"ok\":true}', got %q", body)
	}

	resetGlobalCache()
}

func TestApiRegisterRouteStaticInMemory(t *testing.T) {
	resetGlobalCache()
	InitCache(IN_MEMORY, nil)

	callCount := 0
	config := ApiRouteConfig{
		Type:       STATIC,
		HttpMethod: GET,
	}

	r := chi.NewRouter()
	config.RegisterRoute(r, "/api/static-mem", func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"cached":"yes"}`))
	})

	// First request: cache miss
	req := httptest.NewRequest("GET", "/api/static-mem", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if body := rec.Body.String(); body != `{"cached":"yes"}` {
		t.Errorf("first request: expected '{\"cached\":\"yes\"}', got %q", body)
	}

	// Second request: cache hit (handler not called again)
	req2 := httptest.NewRequest("GET", "/api/static-mem", nil)
	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, req2)

	if callCount != 1 {
		t.Errorf("expected handler called once (cached), got %d", callCount)
	}
	if body := rec2.Body.String(); body != `{"cached":"yes"}` {
		t.Errorf("cache hit: expected '{\"cached\":\"yes\"}', got %q", body)
	}

	resetGlobalCache()
}

func TestApiRegisterRouteISRInMemory(t *testing.T) {
	resetGlobalCache()
	InitCache(IN_MEMORY, nil)

	callCount := 0
	config := ApiRouteConfig{
		Type:            ISR,
		HttpMethod:      GET,
		RevalidateInSec: 60,
	}

	r := chi.NewRouter()
	config.RegisterRoute(r, "/api/isr-mem", func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"isr":"data"}`))
	})

	// First request: cache miss
	req := httptest.NewRequest("GET", "/api/isr-mem", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if body := rec.Body.String(); body != `{"isr":"data"}` {
		t.Errorf("expected '{\"isr\":\"data\"}', got %q", body)
	}

	// Second request: cache hit
	req2 := httptest.NewRequest("GET", "/api/isr-mem", nil)
	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, req2)

	if callCount != 1 {
		t.Errorf("expected handler called once (cached), got %d", callCount)
	}
	if body := rec2.Body.String(); body != `{"isr":"data"}` {
		t.Errorf("cache hit: expected '{\"isr\":\"data\"}', got %q", body)
	}

	resetGlobalCache()
}

func TestApiRegisterRouteISRCACHE_CONTROL_HEADERS(t *testing.T) {
	resetGlobalCache()
	InitCache(CACHE_CONTROL_HEADERS, nil)

	config := ApiRouteConfig{
		Type:            ISR,
		HttpMethod:      GET,
		RevalidateInSec: 30,
	}

	r := chi.NewRouter()
	config.RegisterRoute(r, "/api/isr-cc", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	})

	req := httptest.NewRequest("GET", "/api/isr-cc", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	expected := "max-age=30, stale-while-revalidate=30, stale-if-error=30"
	if cc := rec.Header().Get("Cache-Control"); cc != expected {
		t.Errorf("expected Cache-Control %q, got %q", expected, cc)
	}

	resetGlobalCache()
}

func TestApiRegisterRouteStaticLocalFiles(t *testing.T) {
	tmpDir := t.TempDir()
	resetGlobalCache()
	InitCache(LOCAL_FILES, &CacheConfig{CacheFilesPath: tmpDir})

	callCount := 0
	config := ApiRouteConfig{
		Type:       STATIC,
		HttpMethod: GET,
	}

	r := chi.NewRouter()
	config.RegisterRoute(r, "/api/files", func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"file":"cached"}`))
	})

	req := httptest.NewRequest("GET", "/api/files", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if body := rec.Body.String(); body != `{"file":"cached"}` {
		t.Errorf("expected '{\"file\":\"cached\"}', got %q", body)
	}

	// Second request: cache hit
	req2 := httptest.NewRequest("GET", "/api/files", nil)
	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, req2)

	if callCount != 1 {
		t.Errorf("expected handler called once (file cached), got %d", callCount)
	}
	if body := rec2.Body.String(); body != `{"file":"cached"}` {
		t.Errorf("cache hit: expected '{\"file\":\"cached\"}', got %q", body)
	}

	resetGlobalCache()
}

func TestApiCachedResponsePreservesStatusCodeAndContentType(t *testing.T) {
	resetGlobalCache()
	InitCache(IN_MEMORY, nil)

	config := ApiRouteConfig{
		Type:       STATIC,
		HttpMethod: POST,
	}

	r := chi.NewRouter()
	config.RegisterRoute(r, "/api/create", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id":"123"}`))
	})

	// First request: cache miss
	req := httptest.NewRequest("POST", "/api/create", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("first request: expected 201, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("first request: expected Content-Type application/json, got %q", ct)
	}

	// Second request: cache hit — should preserve status code and content type
	req2 := httptest.NewRequest("POST", "/api/create", nil)
	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusCreated {
		t.Errorf("cache hit: expected 201, got %d", rec2.Code)
	}
	if ct := rec2.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("cache hit: expected Content-Type application/json, got %q", ct)
	}
	if body := rec2.Body.String(); body != `{"id":"123"}` {
		t.Errorf("cache hit: expected '{\"id\":\"123\"}', got %q", body)
	}

	resetGlobalCache()
}

func TestApiRegisterRouteHTTPMethods(t *testing.T) {
	resetGlobalCache()
	InitCache(CACHE_CONTROL_HEADERS, nil)

	methods := []struct {
		method     HttpMethod
		httpMethod string
	}{
		{GET, "GET"},
		{POST, "POST"},
		{PUT, "PUT"},
		{PATCH, "PATCH"},
		{DELETE, "DELETE"},
	}

	for _, m := range methods {
		t.Run(m.httpMethod, func(t *testing.T) {
			config := ApiRouteConfig{
				Type:       DYNAMIC,
				HttpMethod: m.method,
			}

			r := chi.NewRouter()
			config.RegisterRoute(r, "/api/method-test", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("ok"))
			})

			req := httptest.NewRequest(m.httpMethod, "/api/method-test", nil)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Errorf("%s: expected 200, got %d", m.httpMethod, rec.Code)
			}
		})
	}

	resetGlobalCache()
}

func TestRegisterRouteHTTPMethods(t *testing.T) {
	resetGlobalCache()
	InitCache(CACHE_CONTROL_HEADERS, nil)

	methods := []struct {
		method     HttpMethod
		httpMethod string
	}{
		{GET, "GET"},
		{POST, "POST"},
		{PUT, "PUT"},
		{PATCH, "PATCH"},
		{DELETE, "DELETE"},
	}

	for _, m := range methods {
		t.Run(m.httpMethod, func(t *testing.T) {
			config := RouteConfig[any]{
				Type:       DYNAMIC,
				HttpMethod: m.method,
				Middleware: func(w http.ResponseWriter, r *http.Request) any {
					return nil
				},
			}

			r := chi.NewRouter()
			config.RegisterRoute(r, "/method-test", func(props any) templ.Component {
				return mockComponent("ok")
			})

			req := httptest.NewRequest(m.httpMethod, "/method-test", nil)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Errorf("%s: expected 200, got %d", m.httpMethod, rec.Code)
			}
		})
	}

	resetGlobalCache()
}
