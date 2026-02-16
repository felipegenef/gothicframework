package helpers

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/a-h/templ"
	helpers "github.com/felipegenef/gothicframework/pkg/helpers"
	"github.com/go-chi/chi/v5"
	"github.com/joho/godotenv"
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
}

type isrLocalCache struct {
	data       any
	revalidate time.Time
}

var localCacheMutex sync.RWMutex
var localCacheValue map[string]any = make(map[string]any)

func (config *RouteConfig[T]) RegisterRoute(r chi.Router, httpPath string, component func(T) templ.Component) {
	godotenv.Load()
	var localServe = os.Getenv("LOCAL_SERVE")
	var isLocal = len(localServe) > 0 && localServe == "true"
	if config.Type == STATIC {
		switch config.HttpMethod {
		case GET:
			r.Get(httpPath, func(w http.ResponseWriter, r *http.Request) {
				if !isLocal {
					w.Header().Set("Cache-Control", "max-age=31536000")
					config.Render(r, w, component(config.Middleware(w, r)))
					return
				}
				fullURL := r.URL.RequestURI() // this includes query parameters
				config.Render(r, w, component(config.getCachedOrUpdate(fullURL, w, r)))
			})
		case POST:
			r.Post(httpPath, func(w http.ResponseWriter, r *http.Request) {
				if !isLocal {
					w.Header().Set("Cache-Control", "max-age=31536000")
					config.Render(r, w, component(config.Middleware(w, r)))
					return
				}
				fullURL := r.URL.RequestURI() // this includes query parameters
				config.Render(r, w, component(config.getCachedOrUpdate(fullURL, w, r)))
			})
		case PUT:
			r.Put(httpPath, func(w http.ResponseWriter, r *http.Request) {
				if !isLocal {
					w.Header().Set("Cache-Control", "max-age=31536000")
					config.Render(r, w, component(config.Middleware(w, r)))
					return
				}
				fullURL := r.URL.RequestURI() // this includes query parameters
				config.Render(r, w, component(config.getCachedOrUpdate(fullURL, w, r)))
			})
		case PATCH:
			r.Patch(httpPath, func(w http.ResponseWriter, r *http.Request) {
				if !isLocal {
					w.Header().Set("Cache-Control", "max-age=31536000")
					config.Render(r, w, component(config.Middleware(w, r)))
					return
				}
				fullURL := r.URL.RequestURI() // this includes query parameters
				config.Render(r, w, component(config.getCachedOrUpdate(fullURL, w, r)))
			})
		case DELETE:
			r.Delete(httpPath, func(w http.ResponseWriter, r *http.Request) {
				if !isLocal {
					w.Header().Set("Cache-Control", "max-age=31536000")
					config.Render(r, w, component(config.Middleware(w, r)))
					return
				}
				fullURL := r.URL.RequestURI() // this includes query parameters
				config.Render(r, w, component(config.getCachedOrUpdate(fullURL, w, r)))
			})
		}

	}

	if config.Type == DYNAMIC {
		switch config.HttpMethod {
		case GET:
			r.Get(httpPath, func(w http.ResponseWriter, r *http.Request) {
				config.Render(r, w, component(config.Middleware(w, r)))
			})
		case POST:
			r.Post(httpPath, func(w http.ResponseWriter, r *http.Request) {
				config.Render(r, w, component(config.Middleware(w, r)))
			})
		case PUT:
			r.Put(httpPath, func(w http.ResponseWriter, r *http.Request) {
				config.Render(r, w, component(config.Middleware(w, r)))
			})
		case PATCH:
			r.Patch(httpPath, func(w http.ResponseWriter, r *http.Request) {
				config.Render(r, w, component(config.Middleware(w, r)))
			})
		case DELETE:
			r.Delete(httpPath, func(w http.ResponseWriter, r *http.Request) {
				config.Render(r, w, component(config.Middleware(w, r)))
			})
		}

	}

	if config.Type == ISR {
		switch config.HttpMethod {
		case GET:
			r.Get(httpPath, func(w http.ResponseWriter, r *http.Request) {
				if !isLocal {
					w.Header().Set("Cache-Control", fmt.Sprintf(
						"max-age=%v, stale-while-revalidate=%v, stale-if-error=%v",
						config.RevalidateInSec, config.RevalidateInSec, config.RevalidateInSec,
					))
					config.Render(r, w, component(config.Middleware(w, r)))
					return
				}
				fullURL := r.URL.RequestURI() // this includes query parameters
				config.Render(r, w, component(config.getISRCachedOrUpdate(fullURL, w, r)))

			})
		case POST:
			r.Post(httpPath, func(w http.ResponseWriter, r *http.Request) {
				if !isLocal {
					w.Header().Set("Cache-Control", fmt.Sprintf(
						"max-age=%v, stale-while-revalidate=%v, stale-if-error=%v",
						config.RevalidateInSec, config.RevalidateInSec, config.RevalidateInSec,
					))
					config.Render(r, w, component(config.Middleware(w, r)))
					return
				}
				fullURL := r.URL.RequestURI() // this includes query parameters
				config.Render(r, w, component(config.getISRCachedOrUpdate(fullURL, w, r)))
			})
		case PUT:
			r.Put(httpPath, func(w http.ResponseWriter, r *http.Request) {
				if !isLocal {
					w.Header().Set("Cache-Control", fmt.Sprintf(
						"max-age=%v, stale-while-revalidate=%v, stale-if-error=%v",
						config.RevalidateInSec, config.RevalidateInSec, config.RevalidateInSec,
					))
					config.Render(r, w, component(config.Middleware(w, r)))
					return
				}
				fullURL := r.URL.RequestURI() // this includes query parameters
				config.Render(r, w, component(config.getISRCachedOrUpdate(fullURL, w, r)))
			})
		case PATCH:
			r.Patch(httpPath, func(w http.ResponseWriter, r *http.Request) {
				if !isLocal {
					w.Header().Set("Cache-Control", fmt.Sprintf(
						"max-age=%v, stale-while-revalidate=%v, stale-if-error=%v",
						config.RevalidateInSec, config.RevalidateInSec, config.RevalidateInSec,
					))
					config.Render(r, w, component(config.Middleware(w, r)))
					return
				}
				fullURL := r.URL.RequestURI() // this includes query parameters
				config.Render(r, w, component(config.getISRCachedOrUpdate(fullURL, w, r)))
			})
		case DELETE:
			r.Delete(httpPath, func(w http.ResponseWriter, r *http.Request) {
				if !isLocal {
					w.Header().Set("Cache-Control", fmt.Sprintf(
						"max-age=%v, stale-while-revalidate=%v, stale-if-error=%v",
						config.RevalidateInSec, config.RevalidateInSec, config.RevalidateInSec,
					))
					config.Render(r, w, component(config.Middleware(w, r)))
					return
				}
				fullURL := r.URL.RequestURI() // this includes query parameters
				config.Render(r, w, component(config.getISRCachedOrUpdate(fullURL, w, r)))
			})
		}
	}

}

func (config *RouteConfig[T]) getISRCachedOrUpdate(fullURL string, w http.ResponseWriter, r *http.Request) T {
	localCacheMutex.Lock()
	defer localCacheMutex.Unlock()

	cacheItem, exists := localCacheValue[fullURL]
	now := time.Now()

	if !exists {
		// Cache miss: generate and store new data with revalidation time
		newData := config.Middleware(w, r)
		localCacheValue[fullURL] = isrLocalCache{
			data:       newData,
			revalidate: now.Add(time.Duration(config.RevalidateInSec) * time.Second),
		}
		return newData
	}

	// Try to assert the cache item to expected struct type
	cached, ok := cacheItem.(isrLocalCache)
	if !ok {
		// Cache is corrupted or wrong type; regenerate
		newData := config.Middleware(w, r)
		localCacheValue[fullURL] = isrLocalCache{
			data:       newData,
			revalidate: now.Add(time.Duration(config.RevalidateInSec) * time.Second),
		}
		return newData
	}

	// If revalidation time has passed, refresh the data
	if cached.revalidate.Before(now) {
		newData := config.Middleware(w, r)
		localCacheValue[fullURL] = isrLocalCache{
			data:       newData,
			revalidate: now.Add(time.Duration(config.RevalidateInSec) * time.Second),
		}
		return newData
	}

	// Safe type assertion with check to avoid panic if cached.data is nil
	if val, ok := cached.data.(T); ok {
		return val
	}

	// Fallback: data is nil or not of type T
	newData := config.Middleware(w, r)
	localCacheValue[fullURL] = isrLocalCache{
		data:       newData,
		revalidate: now.Add(time.Duration(config.RevalidateInSec) * time.Second),
	}
	return newData
}

func (config *RouteConfig[T]) getCachedOrUpdate(fullURL string, w http.ResponseWriter, r *http.Request) T {
	localCacheMutex.Lock()
	defer localCacheMutex.Unlock()

	if cached, exists := localCacheValue[fullURL]; exists {
		// Safe type assertion with check to avoid panic
		if val, ok := cached.(T); ok {
			return val
		}
		// Fallback if data in cache is nil or wrong type
	}

	// Cache miss or failed assertion: generate and store
	result := config.Middleware(w, r)
	localCacheValue[fullURL] = result
	return result
}

func (config *RouteConfig[T]) Render(r *http.Request, w http.ResponseWriter, component templ.Component) error {
	return component.Render(r.Context(), w)
}

type ApiRouteConfig struct {
	HttpMethod HttpMethod
}

func (config *ApiRouteConfig) RegisterRoute(r chi.Router, httpPath string, fn func(w http.ResponseWriter, r *http.Request)) {
	switch config.HttpMethod {
	case GET:
		r.Get(httpPath, fn)
	case POST:
		r.Post(httpPath, fn)
	case PUT:
		r.Put(httpPath, fn)
	case PATCH:
		r.Patch(httpPath, fn)
	case DELETE:
		r.Delete(httpPath, fn)
	}

}

func (config *ApiRouteConfig) Render(r *http.Request, w http.ResponseWriter, component templ.Component) error {
	return component.Render(r.Context(), w)
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
		OutputFile:              "./src/routes/autoGenRoutes.go",
		TemplateFile:            "./.gothicCli/templates/autoGenRoutes.go",
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
