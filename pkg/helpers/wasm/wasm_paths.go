package helpers

import (
	"regexp"
	"runtime"
	"strings"
)

// Path normalization: convert source-file paths under src/pages and src/components
// to the URL path the framework will serve them under, and to the WASM output
// file name (no slashes, no leading dash).

// wasmOutputName converts an HTTP path to a WASM output file basename
// (no extension). "/" → "index", "/foo/bar" → "foo-bar". Parameter segments
// like "/blog/{slug}" become "blog-slug".
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

// normalizeWasmHttpPath converts a source-file path (e.g.
// "src/pages/blog/var_slug_templ.go") to the HTTP path it serves
// ("/blog/{slug}").
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
