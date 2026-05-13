package helpers

import "strings"

// WasmOutputName converts an HTTP path to a safe filename for public/wasm/.
//
//	/          → index
//	/counter   → counter
//	/user/{id} → user-id
//	/blog/{slug}/comments → blog-slug-comments
func WasmOutputName(httpPath string) string {
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
