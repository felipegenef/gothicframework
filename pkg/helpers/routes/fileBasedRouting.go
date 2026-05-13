package helpers

import (
	"bytes"
	"context"
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
	helpers "github.com/felipegenef/gothicframework/pkg/helpers"
	"github.com/go-chi/chi/v5"
)

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

type RouteConfig[T any] struct {
	Type            ConfigType
	HttpMethod      HttpMethod
	RevalidateInSec int
	Middleware      func(w http.ResponseWriter, r *http.Request) T
	// PageState, if non-nil, signals that this route has a WASM reactive state
	// function.  The CLI extracts the function body and compiles it with TinyGo.
	// The function is never called server-side; it only needs to compile.
	PageState func()
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
	if config.PageState != nil {
		wasmName := WasmOutputName(httpPath)
		wrapped = func(props T) templ.Component {
			return &wasmInjectedComponent{inner: component(props), wasmName: wasmName}
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
	inner    templ.Component
	wasmName string
}

func (c *wasmInjectedComponent) Render(ctx context.Context, w io.Writer) error {
	var buf bytes.Buffer
	if err := c.inner.Render(ctx, &buf); err != nil {
		return err
	}
	html := injectGothicScope(buf.Bytes(), c.wasmName)
	_, err := w.Write(injectWasmBootstrap(html, c.wasmName))
	return err
}

// injectGothicScope marks the scope boundary for a WASM instance.
// Uses data-gothic-wasm (static, no random value) so the HTML is CDN-cacheable.
// The browser-side bootstrap script generates the unique data-gothic-scope ID at runtime.
func injectGothicScope(html []byte, wasmName string) []byte {
	attr := `data-gothic-wasm="` + wasmName + `"`
	if bytes.Contains(html, []byte("<body")) {
		return bytes.Replace(html, []byte("<body"), []byte("<body "+attr), 1)
	}
	var buf bytes.Buffer
	buf.WriteString(`<div ` + attr + ` style="display:contents">`)
	buf.Write(html)
	buf.WriteString(`</div>`)
	return buf.Bytes()
}

// injectWasmBootstrap injects the WASM loader script.
// The scope ID is generated client-side via Math.random() so the HTML remains
// fully static and CDN-cacheable regardless of route type.
func injectWasmBootstrap(html []byte, wasmName string) []byte {
	isFullPage := bytes.Contains(html, []byte("</body>"))

	// For full pages the scope is on <body>; for fragments it's on the wrapper div
	// immediately before the script tag (previousElementSibling).
	var findEl string
	if isFullPage {
		findEl = `document.querySelector('body[data-gothic-wasm="` + wasmName + `"]')`
	} else {
		findEl = `(document.currentScript&&document.currentScript.previousElementSibling)` +
			`||document.querySelector('[data-gothic-wasm="` + wasmName + `"]:not([data-gothic-scope])')`
	}

	script := fmt.Sprintf(`<script>
(function(){
    var wn='%s';
    var el=(%s);
    if(!el)return;
    var id=wn+'-'+(Math.random()*0xFFFFFFFF>>>0).toString(16).padStart(8,'0');
    el.setAttribute('data-gothic-scope',id);
    (async function(){
        if(typeof Go==='undefined'){
            await new Promise(function(res,rej){
                var s=document.createElement('script');
                s.src='/public/wasm_exec.js';
                s.onload=res;s.onerror=rej;
                document.head.appendChild(s);
            });
        }
        var go=new Go();
        var r=await WebAssembly.instantiateStreaming(
            fetch('/public/wasm/'+wn+'.wasm.gz'),go.importObject
        );
        window.__gothicCurrentModule=id;
        go.run(r.instance);
    })();
})();
</script>`, wasmName, findEl)

	if isFullPage {
		return bytes.Replace(html, []byte("</body>"), []byte(script+"</body>"), 1)
	}
	return append(html, []byte(script)...)
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
	TemplateInfo            TemplateInfo
	PackageRegex            *regexp.Regexp
	RouteConfigNameRegex    *regexp.Regexp
	ApiRouteConfigNameRegex *regexp.Regexp
	RouteFuncNameRegex      *regexp.Regexp
	ApiRouteFuncNameRegex   *regexp.Regexp
	OutputFile              string
	TemplateFile            string
	ApiRoutesFolder         string
	ComponentRoutesFolder   string
	PageRoutesFolder        string
	Template                helpers.TemplateHelper
}

func NewFileBasedRouteHelper() FileBasedRouteHelper {
	return FileBasedRouteHelper{
		OutputFile:              "./src/routes/routes_gen.go",
		TemplateFile:            "./.gothicCli/templates/routes_gen.go",
		ApiRoutesFolder:         "./src/api",
		ComponentRoutesFolder:   "./src/components",
		PageRoutesFolder:        "./src/pages",
		PackageRegex:            regexp.MustCompile(`(?m)^package\s+(\w+)`),
		RouteConfigNameRegex:    regexp.MustCompile(`(?m)^var\s+(\w+)\s*=\s*routes\.RouteConfig\[[^\]]+\]\s*{([^}]*)}`),
		ApiRouteConfigNameRegex: regexp.MustCompile(`(?m)^var\s+(\w+)\s*=\s*routes\.ApiRouteConfig\s*{([^}]+)}`),
		RouteFuncNameRegex:      regexp.MustCompile(`(?m)^func\s+(\w+)\s*\(.*\)\s+templ\.Component\s*{`),
		ApiRouteFuncNameRegex:   regexp.MustCompile(`(?m)^func\s+(\w+)\s*\(.*\)\s*{`),
		Template:                helpers.NewTemplateHelper(),
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
	return helper.Template.UpdateFromTemplate(helper.TemplateFile, helper.OutputFile, helper.TemplateInfo)
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
			content, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("failed to read file %s: %w", path, err)
			}

			packageMatch := helper.PackageRegex.FindStringSubmatch(string(content))
			if len(packageMatch) > 1 {
				route.PackageName = packageMatch[1]
				route.ConfigPackageName = packageMatch[1]
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

			configMatch := helper.ApiRouteConfigNameRegex.FindStringSubmatch(string(content))
			if len(configMatch) > 1 {
				route.ConfigName = configMatch[1]
			} else {
				route.ConfigName = "DefaultApiConfig"
				route.ConfigPackageName = "routes"
			}

			funcMatch := helper.ApiRouteFuncNameRegex.FindStringSubmatch(string(content))
			if len(funcMatch) > 1 {
				route.FunctionName = funcMatch[1]
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
			content, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("failed to read file %s: %w", path, err)
			}

			packageMatch := helper.PackageRegex.FindStringSubmatch(string(content))
			if len(packageMatch) > 1 {
				route.PackageName = packageMatch[1]
				route.ConfigPackageName = packageMatch[1]
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

			configMatch := helper.RouteConfigNameRegex.FindStringSubmatch(string(content))
			if len(configMatch) > 1 {
				route.ConfigName = configMatch[1]
			} else {
				route.ConfigName = "DefaultConfig"
				route.ConfigPackageName = "routes"
			}

			funcMatch := helper.RouteFuncNameRegex.FindStringSubmatch(string(content))
			if len(funcMatch) > 1 {
				route.FunctionName = funcMatch[1]
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
			content, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("failed to read file %s: %w", path, err)
			}

			packageMatch := helper.PackageRegex.FindStringSubmatch(string(content))
			if len(packageMatch) > 1 {
				route.PackageName = packageMatch[1]
				route.ConfigPackageName = packageMatch[1]
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

			configMatch := helper.RouteConfigNameRegex.FindStringSubmatch(string(content))
			if len(configMatch) > 1 {
				route.ConfigName = configMatch[1]
			} else {
				route.ConfigName = "DefaultConfig"
				route.ConfigPackageName = "routes"
			}

			funcMatch := helper.RouteFuncNameRegex.FindStringSubmatch(string(content))
			if len(funcMatch) > 1 {
				route.FunctionName = funcMatch[1]
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
		re := regexp.MustCompile(`var_([a-zA-Z0-9_]+)`)
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
