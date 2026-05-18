package helpers

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Page scanning: walk pagesDir and componentsDir looking for *_templ.go files
// that declare a ClientSideState body (either inline or via a named function).
// Extract that body and the page's std imports for the WASM build pipeline.

var pageStateInlineRe = regexp.MustCompile(`(?m)ClientSideState:\s*func\s*\(\s*\)\s*\{`)
var pageStateNamedRe = regexp.MustCompile(`(?m)ClientSideState:\s*(\w+)`)
var wasmCompressionRe = regexp.MustCompile(`(?m)WasmCompression:\s*routes\.(\w+)`)

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

	if loc := pageStateInlineRe.FindStringIndex(content); loc != nil {
		openBrace := loc[1] - 1
		body = h.extractFuncBody(content, openBrace)
		if body == "" {
			return WasmPage{}, false, fmt.Errorf("wasm: could not extract inline ClientSideState body in %s", path)
		}
	} else if m := pageStateNamedRe.FindStringSubmatch(content); m != nil {
		funcName := m[1]
		funcRe := regexp.MustCompile(`(?m)^func\s+` + regexp.QuoteMeta(funcName) + `\s*\(\s*\)\s*\{`)
		floc := funcRe.FindStringIndex(content)
		if floc == nil {
			return WasmPage{}, false, fmt.Errorf("wasm: ClientSideState func %s not found in %s", funcName, path)
		}
		openBrace := floc[1] - 1
		body = h.extractFuncBody(content, openBrace)
		if body == "" {
			return WasmPage{}, false, fmt.Errorf("wasm: could not extract body of %s in %s", funcName, path)
		}
	} else {
		return WasmPage{}, false, nil
	}

	stdImports := h.filterStdImports(content, body)
	httpPath := h.normalizeWasmHttpPath(path)
	outputName := h.wasmOutputName(httpPath)

	compression := WasmCompressionGzip
	if m := wasmCompressionRe.FindStringSubmatch(content); m != nil && m[1] == "BROTLI" {
		compression = WasmCompressionBrotli
	}

	return WasmPage{
		SourceFile:  path,
		FuncBody:    body,
		Imports:     stdImports,
		HttpPath:    httpPath,
		OutputName:  outputName,
		Compression: compression,
	}, true, nil
}

// extractFuncBody returns the substring between the matching braces starting
// at openBrace (which must point to the '{' immediately before the body).
// It is brace-depth aware and skips strings, raw strings, and comments so
// braces inside those are not counted.
func (h *WasmHelper) extractFuncBody(content string, openBrace int) string {
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

// filterStdImports returns only standard-library imports that are referenced
// in body. This is what the generated WASM main.go needs.
func (h *WasmHelper) filterStdImports(content, body string) []string {
	raw := h.extractImportLines(content)
	var kept []string
	for _, line := range raw {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		path, alias := h.parseImportLine(line)
		if path == "" {
			continue
		}
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

func (h *WasmHelper) extractImportLines(content string) []string {
	blockRe := regexp.MustCompile(`(?s)import\s*\((.+?)\)`)
	m := blockRe.FindStringSubmatch(content)
	if m == nil {
		return nil
	}
	return strings.Split(m[1], "\n")
}

func (h *WasmHelper) parseImportLine(line string) (path, alias string) {
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
