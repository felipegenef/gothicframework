package helpers

import (
	"bytes"
	"context"
	"embed"
	"encoding/gob"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/a-h/templ"
	helpers "github.com/felipegenef/gothicframework/v2/pkg/helpers"
	"github.com/go-chi/chi/v5"
)

// routesGenTemplateFS embeds the generator template for src/routes/routes_gen.go.
// Pre-v2.17 CLIs seeded this onto disk under `.gothicCli/templates/routes_gen.go`
// where users could (in theory) edit it. In practice nobody ever did, and drift
// between the CLI's expectations and the on-disk copy caused silent breakage,
// so it now lives inside the CLI binary.
//
//go:embed routes_gen.go.tmpl
var routesGenTemplateFS embed.FS

const routesGenTemplatePath = "routes_gen.go.tmpl"

type ConfigType int

const (
	ISR ConfigType = iota
	STATIC
	DYNAMIC
)

type HttpMethod int

const (
	GET HttpMethod = iota
	POST
	PUT
	PATCH
	DELETE
)

// WasmCompiler selects the WASM build toolchain for a route.
type WasmCompiler int

const (
	GothicTinyGo WasmCompiler = iota // default: embedded TinyGo binary
	LocalTinyGo                      // system tinygo binary in PATH
	Golang                           // GOOS=js GOARCH=wasm standard Go compiler
)

type RouteConfig[T any] struct {
	Type            ConfigType
	HttpMethod      HttpMethod
	RevalidateInSec int
	Middleware      func(w http.ResponseWriter, r *http.Request) T
	// ClientSideState, if non-nil, marks this route as having a WASM reactive state
	// function.  The CLI extracts the function body and compiles it with TinyGo.
	// The function is never called server-side; it only needs to compile.
	ClientSideState func()
	// WasmCompression sets the compression algorithm for the compiled WASM output.
	// Defaults to GZIP (zero value). Options: GZIP, BROTLI.
	WasmCompression CompressionMethod
	// WasmCompiler selects the WASM build toolchain. Defaults to GothicTinyGo.
	WasmCompiler WasmCompiler
	// Path is the HTTP route path, set automatically by RegisterRoute.
	// Use it with StatefulComponentOf to avoid hardcoding path strings.
	Path string
}

var DefaultConfig = RouteConfig[any]{
	Type:       STATIC,
	HttpMethod: GET,
	Middleware: func(w http.ResponseWriter, r *http.Request) any {
		return nil
	},
}

var DefaultApiConfig = ApiRouteConfig{
	HttpMethod: GET,
	Type:       DYNAMIC,
}

func (config *RouteConfig[T]) RegisterRoute(r chi.Router, httpPath string, component func(T) templ.Component) {
	config.Path = httpPath
	wrapped := component
	if config.ClientSideState != nil {
		wasmName := WasmOutputName(httpPath)
		compression := config.WasmCompression
		compiler := config.WasmCompiler
		wrapped = func(props T) templ.Component {
			return &wasmInjectedComponent{inner: component(props), wasmName: wasmName, compression: compression, compiler: compiler}
		}
	}
	handler := config.resolveHandler(wrapped)

	switch config.HttpMethod {
	case GET:
		r.Get(httpPath, handler)
	case POST:
		r.Post(httpPath, handler)
	case PUT:
		r.Put(httpPath, handler)
	case PATCH:
		r.Patch(httpPath, handler)
	case DELETE:
		r.Delete(httpPath, handler)
	}
}

// wasmInjectedComponent wraps a templ.Component and injects the WASM bootstrap
// script before </body> in the rendered HTML.
type wasmInjectedComponent struct {
	inner       templ.Component
	wasmName    string
	compression CompressionMethod
	compiler    WasmCompiler
}

func (c *wasmInjectedComponent) Render(ctx context.Context, w io.Writer) error {
	var buf bytes.Buffer
	if err := c.inner.Render(ctx, &buf); err != nil {
		return err
	}
	_, err := w.Write(injectWasmEnvelope(buf.Bytes(), c.wasmName, c.compression, c.compiler))
	return err
}

// emptyComponent renders nothing — used as the inner component for topic managers.
type emptyComponent struct{}

func (emptyComponent) Render(_ context.Context, _ io.Writer) error { return nil }

// TopicManagerComponent returns a templ.Component that loads the named topic-manager
// WASM inline (no HTMX round-trip). Drop it in any layout or page with @wasm.AddPageTopic().
func TopicManagerComponent(wasmName string, compression CompressionMethod) templ.Component {
	return &wasmInjectedComponent{inner: emptyComponent{}, wasmName: wasmName, compression: compression, compiler: GothicTinyGo}
}


func (config *RouteConfig[T]) resolveHandler(component func(T) templ.Component) http.HandlerFunc {
	switch config.Type {
	case STATIC:
		store := getGlobalCacheStore()
		cacheType := getGlobalCacheType()
		return config.staticHandler(component, store, cacheType)
	case ISR:
		store := getGlobalCacheStore()
		cacheType := getGlobalCacheType()
		return config.isrHandler(component, store, cacheType)
	default:
		return config.dynamicHandler(component)
	}
}

func (config *RouteConfig[T]) dynamicHandler(component func(T) templ.Component) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		config.Render(r, w, component(config.Middleware(w, r)))
	}
}

func (config *RouteConfig[T]) staticHandler(component func(T) templ.Component, store CacheStore, cacheType CacheType) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// CACHE_CONTROL_HEADERS mode: set headers and render directly (no store caching)
		if cacheType == CACHE_CONTROL_HEADERS {
			w.Header().Set("Cache-Control", "max-age=31536000")
			config.Render(r, w, component(config.Middleware(w, r)))
			return
		}

		// Check cache
		key := r.URL.RequestURI()
		if cached, ok := store.Get(key); ok {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(cached)
			return
		}

		// Cache miss: render to buffer, cache, and write response
		middlewareResult := config.Middleware(w, r)
		var buf bytes.Buffer
		component(middlewareResult).Render(r.Context(), &buf)
		store.Set(key, buf.Bytes(), 0)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(buf.Bytes())
	}
}

func (config *RouteConfig[T]) isrHandler(component func(T) templ.Component, store CacheStore, cacheType CacheType) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// CACHE_CONTROL_HEADERS mode: set headers and render directly (no store caching)
		if cacheType == CACHE_CONTROL_HEADERS {
			w.Header().Set("Cache-Control", fmt.Sprintf(
				"max-age=%v, stale-while-revalidate=%v, stale-if-error=%v",
				config.RevalidateInSec, config.RevalidateInSec, config.RevalidateInSec,
			))
			config.Render(r, w, component(config.Middleware(w, r)))
			return
		}

		// Check cache
		key := r.URL.RequestURI()
		if cached, ok := store.Get(key); ok {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(cached)
			return
		}

		// Cache miss: render to buffer, cache with TTL, and write response
		ttl := time.Duration(config.RevalidateInSec) * time.Second
		middlewareResult := config.Middleware(w, r)
		var buf bytes.Buffer
		component(middlewareResult).Render(r.Context(), &buf)
		store.Set(key, buf.Bytes(), ttl)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(buf.Bytes())
	}
}

func (config *RouteConfig[T]) Render(r *http.Request, w http.ResponseWriter, component templ.Component) error {
	return component.Render(r.Context(), w)
}

type ApiRouteConfig struct {
	Type            ConfigType
	HttpMethod      HttpMethod
	RevalidateInSec int
}

func (config *ApiRouteConfig) RegisterRoute(r chi.Router, httpPath string, fn func(w http.ResponseWriter, r *http.Request)) {
	handler := config.resolveApiHandler(fn)

	switch config.HttpMethod {
	case GET:
		r.Get(httpPath, handler)
	case POST:
		r.Post(httpPath, handler)
	case PUT:
		r.Put(httpPath, handler)
	case PATCH:
		r.Patch(httpPath, handler)
	case DELETE:
		r.Delete(httpPath, handler)
	}
}

type cachedAPIResponse struct {
	StatusCode  int
	ContentType string
	Body        []byte
}

func encodeCachedAPIResponse(resp cachedAPIResponse) ([]byte, error) {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(resp); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func decodeCachedAPIResponse(data []byte) (cachedAPIResponse, error) {
	var resp cachedAPIResponse
	if err := gob.NewDecoder(bytes.NewReader(data)).Decode(&resp); err != nil {
		return resp, err
	}
	return resp, nil
}

func replayAPIResponse(w http.ResponseWriter, resp cachedAPIResponse) {
	if resp.ContentType != "" {
		w.Header().Set("Content-Type", resp.ContentType)
	}
	w.WriteHeader(resp.StatusCode)
	w.Write(resp.Body)
}

func (config *ApiRouteConfig) resolveApiHandler(fn func(http.ResponseWriter, *http.Request)) http.HandlerFunc {
	switch config.Type {
	case STATIC:
		store := getGlobalCacheStore()
		cacheType := getGlobalCacheType()
		return config.apiStaticHandler(fn, store, cacheType)
	case ISR:
		store := getGlobalCacheStore()
		cacheType := getGlobalCacheType()
		return config.apiISRHandler(fn, store, cacheType)
	default:
		return fn
	}
}

func (config *ApiRouteConfig) apiStaticHandler(fn func(http.ResponseWriter, *http.Request), store CacheStore, cacheType CacheType) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cacheType == CACHE_CONTROL_HEADERS {
			w.Header().Set("Cache-Control", "max-age=31536000")
			fn(w, r)
			return
		}

		key := "api:" + r.URL.RequestURI()
		if cached, ok := store.Get(key); ok {
			resp, err := decodeCachedAPIResponse(cached)
			if err == nil {
				replayAPIResponse(w, resp)
				return
			}
		}

		rec := httptest.NewRecorder()
		fn(rec, r)

		resp := cachedAPIResponse{
			StatusCode:  rec.Code,
			ContentType: rec.Header().Get("Content-Type"),
			Body:        rec.Body.Bytes(),
		}
		if encoded, err := encodeCachedAPIResponse(resp); err == nil {
			store.Set(key, encoded, 0)
		}
		replayAPIResponse(w, resp)
	}
}

func (config *ApiRouteConfig) apiISRHandler(fn func(http.ResponseWriter, *http.Request), store CacheStore, cacheType CacheType) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cacheType == CACHE_CONTROL_HEADERS {
			w.Header().Set("Cache-Control", fmt.Sprintf(
				"max-age=%v, stale-while-revalidate=%v, stale-if-error=%v",
				config.RevalidateInSec, config.RevalidateInSec, config.RevalidateInSec,
			))
			fn(w, r)
			return
		}

		key := "api:" + r.URL.RequestURI()
		if cached, ok := store.Get(key); ok {
			resp, err := decodeCachedAPIResponse(cached)
			if err == nil {
				replayAPIResponse(w, resp)
				return
			}
		}

		rec := httptest.NewRecorder()
		fn(rec, r)

		ttl := time.Duration(config.RevalidateInSec) * time.Second
		resp := cachedAPIResponse{
			StatusCode:  rec.Code,
			ContentType: rec.Header().Get("Content-Type"),
			Body:        rec.Body.Bytes(),
		}
		if encoded, err := encodeCachedAPIResponse(resp); err == nil {
			store.Set(key, encoded, ttl)
		}
		replayAPIResponse(w, resp)
	}
}

type RouteTemplate struct {
	FunctionName      string
	ConfigName        string
	PackageName       string
	ConfigPackageName string
	HttpPath          string
	OriginFile        string
}

type Imports struct {
	Package     string
	PackagePath string
}

type TemplateInfo struct {
	GoModName     string
	ImportDefault bool
	Imports       []Imports
	Routes        []RouteTemplate
	ApiRoutes     []RouteTemplate
}

type FileBasedRouteHelper struct {
	TemplateInfo          TemplateInfo
	OutputFile            string
	TemplateFile          string
	ApiRoutesFolder       string
	ComponentRoutesFolder string
	PageRoutesFolder      string
	Template              helpers.TemplateHelper
}

func NewFileBasedRouteHelper() FileBasedRouteHelper {
	return FileBasedRouteHelper{
		OutputFile:            "./src/routes/routes_gen.go",
		TemplateFile:          routesGenTemplatePath,
		ApiRoutesFolder:       "./src/api",
		ComponentRoutesFolder: "./src/components",
		PageRoutesFolder:      "./src/pages",
		Template:              helpers.NewTemplateHelper(),
	}
}

func (helper *FileBasedRouteHelper) Render(goModName string) error {
	helper.Initialize(goModName)
	// 1️⃣ Walk through ./src/pages
	if err := helper.collectPageInfo(goModName); err != nil {
		return err
	}
	// 2️⃣ Walk through ./src/components
	if err := helper.collectComponentsInfo(goModName); err != nil {
		return err
	}
	// 3️⃣ Walk through ./src/api
	if err := helper.collectApiRoutesInfo(goModName); err != nil {
		return err
	}
	// 4️⃣ Deduplicate imports
	helper.RemoveDuplicates()
	helper.pruneMissingFiles()

	// 5️⃣ Render template
	return helper.Template.UpdateFromTemplateFS(routesGenTemplateFS, helper.TemplateFile, helper.OutputFile, helper.TemplateInfo)
}

func (helper *FileBasedRouteHelper) collectApiRoutesInfo(goModName string) error {
	err := filepath.Walk(helper.ApiRoutesFolder, func(path string, info os.FileInfo, err error) error {
		var route RouteTemplate
		if err != nil {
			return err
		}
		if strings.HasSuffix(info.Name(), ".go") {
			route.OriginFile = path
			route.ConfigName = "DefaultApiConfig"
			route.ConfigPackageName = "routes"

			scan, scanErr := astScanFile(path)
			if scanErr != nil {
				// Mid-edit / malformed source: skip silently, matching prior regex behavior.
				return nil
			}

			if scan.PackageName != "" {
				route.PackageName = scan.PackageName
				route.ConfigPackageName = scan.PackageName
				relPath, err := filepath.Rel("src", filepath.Dir(path))
				if err != nil {
					return fmt.Errorf("failed to get relative import path for %s: %w", path, err)
				}
				importStruct := Imports{
					Package:     route.PackageName,
					PackagePath: fmt.Sprintf("%s/src/%s", goModName, filepath.ToSlash(relPath)),
				}
				helper.TemplateInfo.Imports = append(helper.TemplateInfo.Imports, importStruct)
			}

			if scan.ApiRouteConfigName != "" {
				route.ConfigName = scan.ApiRouteConfigName
			} else {
				route.ConfigName = "DefaultApiConfig"
				route.ConfigPackageName = "routes"
			}

			if scan.ApiFuncName != "" {
				route.FunctionName = scan.ApiFuncName
			}

			route.HttpPath = helper.normalizeHttpPath(path)
			if route.FunctionName != "" {
				helper.TemplateInfo.ApiRoutes = append(helper.TemplateInfo.ApiRoutes, route)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to walk through api: %w", err)
	}
	return nil
}

func (helper *FileBasedRouteHelper) collectComponentsInfo(goModName string) error {
	err := filepath.Walk(helper.ComponentRoutesFolder, func(path string, info os.FileInfo, err error) error {
		var route RouteTemplate
		if err != nil {
			return err
		}
		if strings.HasSuffix(info.Name(), "templ.go") {
			route.OriginFile = path
			route.ConfigName = "DefaultConfig"
			route.ConfigPackageName = "routes"

			scan, scanErr := astScanFile(path)
			if scanErr != nil {
				return nil
			}

			if scan.PackageName != "" {
				route.PackageName = scan.PackageName
				route.ConfigPackageName = scan.PackageName
				relPath, err := filepath.Rel("src", filepath.Dir(path))
				if err != nil {
					return fmt.Errorf("failed to get relative import path for %s: %w", path, err)
				}
				importStruct := Imports{
					Package:     route.PackageName,
					PackagePath: fmt.Sprintf("%s/src/%s", goModName, filepath.ToSlash(relPath)),
				}
				helper.TemplateInfo.Imports = append(helper.TemplateInfo.Imports, importStruct)
			}

			if scan.RouteConfigName != "" {
				route.ConfigName = scan.RouteConfigName
			} else {
				route.ConfigName = "DefaultConfig"
				route.ConfigPackageName = "routes"
			}

			if scan.RouteFuncName != "" {
				route.FunctionName = scan.RouteFuncName
			}

			route.HttpPath = helper.normalizeHttpPath(path)
			if route.FunctionName != "" {
				helper.TemplateInfo.Routes = append(helper.TemplateInfo.Routes, route)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to walk through components: %w", err)
	}
	return nil
}

func (helper *FileBasedRouteHelper) collectPageInfo(goModName string) error {
	err := filepath.Walk(helper.PageRoutesFolder, func(path string, info os.FileInfo, err error) error {
		var route RouteTemplate
		if err != nil {
			return err
		}
		if strings.HasSuffix(info.Name(), "templ.go") {
			route.OriginFile = path
			route.ConfigName = "DefaultConfig"
			route.ConfigPackageName = "routes"

			scan, scanErr := astScanFile(path)
			if scanErr != nil {
				return nil
			}

			if scan.PackageName != "" {
				route.PackageName = scan.PackageName
				route.ConfigPackageName = scan.PackageName
				relPath, err := filepath.Rel("src", filepath.Dir(path))
				if err != nil {
					return fmt.Errorf("failed to get relative import path for %s: %w", path, err)
				}
				importStruct := Imports{
					Package:     route.PackageName,
					PackagePath: fmt.Sprintf("%s/src/%s", goModName, filepath.ToSlash(relPath)),
				}
				helper.TemplateInfo.Imports = append(helper.TemplateInfo.Imports, importStruct)
			}

			if scan.RouteConfigName != "" {
				route.ConfigName = scan.RouteConfigName
			} else {
				route.ConfigName = "DefaultConfig"
				route.ConfigPackageName = "routes"
			}

			if scan.RouteFuncName != "" {
				route.FunctionName = scan.RouteFuncName
			}

			route.HttpPath = helper.normalizeHttpPath(path)
			if route.FunctionName != "" {
				helper.TemplateInfo.Routes = append(helper.TemplateInfo.Routes, route)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to walk through pages: %w", err)
	}
	return nil
}

func (helper *FileBasedRouteHelper) pruneMissingFiles() {
	validFiles := make(map[string]bool)

	// Check existence based on OriginFile
	for _, route := range append(helper.TemplateInfo.Routes, helper.TemplateInfo.ApiRoutes...) {
		if _, err := os.Stat(route.OriginFile); err == nil {
			validFiles[route.OriginFile] = true
		}
	}

	filteredRoutes := make([]RouteTemplate, 0, len(helper.TemplateInfo.Routes))
	for _, route := range helper.TemplateInfo.Routes {
		if validFiles[route.OriginFile] {
			filteredRoutes = append(filteredRoutes, route)
		}
	}
	helper.TemplateInfo.Routes = filteredRoutes

	filteredApiRoutes := make([]RouteTemplate, 0, len(helper.TemplateInfo.ApiRoutes))
	for _, route := range helper.TemplateInfo.ApiRoutes {
		if validFiles[route.OriginFile] {
			filteredApiRoutes = append(filteredApiRoutes, route)
		}
	}
	helper.TemplateInfo.ApiRoutes = filteredApiRoutes

	// Filter imports based on usage in valid routes
	usedPackages := make(map[string]bool)
	for _, route := range helper.TemplateInfo.Routes {
		usedPackages[route.PackageName] = true
	}
	for _, route := range helper.TemplateInfo.ApiRoutes {
		usedPackages[route.PackageName] = true
	}

	filteredImports := make([]Imports, 0, len(helper.TemplateInfo.Imports))
	for _, imp := range helper.TemplateInfo.Imports {
		if usedPackages[imp.Package] {
			filteredImports = append(filteredImports, imp)
		}
	}
	helper.TemplateInfo.Imports = filteredImports
}

func (helper *FileBasedRouteHelper) normalizeHttpPath(path string) string {
	// Normalize Windows path separators to Unix-style
	if runtime.GOOS == "windows" {
		path = strings.ReplaceAll(path, `\`, `/`)
	}

	// Remove extensions
	path = strings.TrimSuffix(path, "_templ.go")
	path = strings.TrimSuffix(path, ".go")

	// Determine if it's a route that needs var_ to {param} conversion
	isHttpRoute := strings.Contains(path, "src/pages") || strings.Contains(path, "src/components") || strings.Contains(path, "src/api")

	// Remove base prefixes
	path = strings.TrimPrefix(path, "src/pages")
	path = strings.TrimPrefix(path, "src")

	// Normalize /index
	if strings.HasSuffix(path, "/index") {
		path = strings.TrimSuffix(path, "/index")
		if path == "" {
			path = "/"
		}
	}

	// Convert var_param__ to {param} ONLY for HTTP routes
	if isHttpRoute {
		re := regexp.MustCompile(`\bvar_([a-zA-Z_][a-zA-Z0-9_]*)\b`)
		path = re.ReplaceAllString(path, `{$1}`)
	}

	return path
}

func (helper *FileBasedRouteHelper) RemoveDuplicates() {
	for _, route := range helper.TemplateInfo.Routes {
		if route.ConfigName == "DefaultConfig" {
			helper.TemplateInfo.ImportDefault = true
		}
	}
	for _, route := range helper.TemplateInfo.ApiRoutes {
		if route.ConfigName == "DefaultApiConfig" {
			helper.TemplateInfo.ImportDefault = true
		}
	}
	uniqueImports := make(map[string]Imports)
	for _, imp := range helper.TemplateInfo.Imports {
		uniqueImports[imp.PackagePath] = imp
	}

	helper.TemplateInfo.Imports = make([]Imports, 0, len(uniqueImports))
	for _, imp := range uniqueImports {
		helper.TemplateInfo.Imports = append(helper.TemplateInfo.Imports, imp)
	}
}

func (helper *FileBasedRouteHelper) Initialize(goModName string) {
	helper.TemplateInfo.ApiRoutes = []RouteTemplate{}
	helper.TemplateInfo.Routes = []RouteTemplate{}
	helper.TemplateInfo.GoModName = goModName
	helper.TemplateInfo.ImportDefault = false
	helper.Template.DeleteFile(helper.OutputFile)
}
