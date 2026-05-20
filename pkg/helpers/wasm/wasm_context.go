package helpers

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"github.com/felipegenef/gothicframework/pkg/helpers/wasm/astx"
)

// Context scanning and parsing.
//
// Reads src/context/*.go, parses struct definitions and type aliases,
// generates context_gen.go (server-side helpers), and produces inlinable
// user code snippets for the WASM build pipeline.

const tmplContextGen = ".gothicCli/templates/wasm/context_gen.go"

// collectContextSnippets reads src/context/*.go, parses struct definitions,
// generates context_gen.go (server side), and returns inlinable user code
// snippets and the parsed structs for template rendering.
func (h *WasmHelper) collectContextSnippets() (snippets []string, structs []structInfo, aliases map[string]string) {
	entries, err := os.ReadDir("src/context")
	if err != nil {
		return nil, nil, nil
	}

	type rawFile struct{ name, src string }
	var files []rawFile
	var allStructs []structInfo
	allAliases := make(map[string]string)
	pkgName := "gothicwasm"

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || e.Name() == "context_gen.go" {
			continue
		}
		data, err := os.ReadFile(filepath.Join("src/context", e.Name()))
		if err != nil {
			continue
		}
		src := string(data)
		if fset := token.NewFileSet(); pkgName == "gothicwasm" {
			if pf, err := parser.ParseFile(fset, "", src, 0); err == nil && pf.Name != nil {
				pkgName = pf.Name.Name
			}
		}
		structs, aliases := h.parseStructsFromSource(src)
		allStructs = append(allStructs, structs...)
		for k, v := range aliases {
			allAliases[k] = v
		}
		files = append(files, rawFile{e.Name(), src})
	}

	seenKeys := map[string]string{}
	for _, s := range allStructs {
		if s.KeyName == "" {
			continue
		}
		if prev, exists := seenKeys[s.KeyName]; exists {
			fmt.Fprintf(os.Stderr,
				"error: duplicate context key name %q — used by both %s and %s in src/context/.\n"+
					"  Each context struct must have a unique key name.\n",
				s.KeyName, prev, s.Name)
			os.Exit(1)
		}
		seenKeys[s.KeyName] = s.Name
	}

	h.writeContextKeyStubs(allStructs, allAliases, pkgName)

	for _, f := range files {
		src, err := astx.StripPackageAndImports(f.src)
		if err != nil {
			fmt.Fprintf(os.Stderr, "context strip %s: %v\n", f.name, err)
			os.Exit(1)
		}
		src = h.rewriteAutoKeys(src)
		src = strings.TrimSpace(src)
		if src != "" {
			snippets = append(snippets, "// --- from src/context/"+f.name+" ---\n"+src)
		}
	}
	return snippets, allStructs, allAliases
}

func (h *WasmHelper) writeContextKeyStubs(structs []structInfo, aliases map[string]string, pkgName string) {
	if len(structs) == 0 {
		_ = os.Remove("src/context/context_gen.go")
		return
	}

	codecs, err := h.buildCodecData(structs, aliases)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: context codec: %v\n", err)
		os.Exit(1)
	}

	data := ContextGenData{
		PkgName:     pkgName,
		HasCtx:      h.hasCtxStructs(structs),
		HasTime:     h.hasTimeFields(structs),
		Codecs:      codecs,
		KeyVars:     h.buildKeyVarData(structs),
		CtxTypes:    h.buildCtxTypeData(structs),
		ServerFuncs: h.buildServerCtxFuncData(structs),
		MountFns:    h.buildMountFnData(structs),
	}

	_ = h.Template.UpdateFromTemplate(tmplContextGen, "src/context/context_gen.go", data)
}

// parseStructsFromSource parses struct definitions and type aliases from a Go source string.
// typeAliases maps alias name → underlying type string (e.g. "MyInt" → "int").
func (h *WasmHelper) parseStructsFromSource(src string) (structs []structInfo, typeAliases map[string]string) {
	typeAliases = make(map[string]string)
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", src, parser.ParseComments)
	if err != nil {
		return nil, typeAliases
	}
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gd.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			switch t := ts.Type.(type) {
			case *ast.Ident:
				// type MyInt int  — record the alias
				typeAliases[ts.Name.Name] = t.Name
			case *ast.ArrayType, *ast.MapType, *ast.StarExpr:
				// type Labels []string, type MyMap map[K]V, type MyPtr *T
				if s := h.astTypeString(ts.Type); s != "" {
					typeAliases[ts.Name.Name] = s
				}
				_ = t
			case *ast.StructType:
				si := structInfo{Name: ts.Name.Name}
				for _, field := range t.Fields.List {
					typ := h.astTypeString(field.Type)
					tag := ""
					if field.Tag != nil {
						tag = h.parseGothicTag(strings.Trim(field.Tag.Value, "`"))
					}
					if len(field.Names) == 0 && typ == "GothicSharedContext" {
						if field.Tag != nil {
							raw := strings.Trim(field.Tag.Value, "`")
							si.KeyName = h.parseNameTag(raw)
							si.Compression = h.parseCompressionTag(raw)
						}
						continue
					}
					for _, name := range field.Names {
						si.Fields = append(si.Fields, fieldInfo{Name: name.Name, Type: typ, GothicTag: tag})
					}
				}
				structs = append(structs, si)
			}
		}
	}
	return structs, typeAliases
}

func (h *WasmHelper) astTypeString(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.ArrayType:
		if e.Len == nil {
			return "[]" + h.astTypeString(e.Elt)
		}
		return h.astTypeString(e.Elt)
	case *ast.StarExpr:
		return "*" + h.astTypeString(e.X)
	case *ast.SelectorExpr:
		return h.astTypeString(e.X) + "." + e.Sel.Name
	case *ast.MapType:
		return "map[" + h.astTypeString(e.Key) + "]" + h.astTypeString(e.Value)
	}
	return ""
}

func (h *WasmHelper) parseGothicTag(tag string) string {
	for _, part := range strings.Fields(tag) {
		if strings.HasPrefix(part, `gothic:"`) {
			return strings.Trim(strings.TrimPrefix(part, "gothic:"), `"`)
		}
	}
	return ""
}

func (h *WasmHelper) parseNameTag(tag string) string {
	for _, part := range strings.Fields(tag) {
		if strings.HasPrefix(part, `name:"`) {
			return strings.Trim(strings.TrimPrefix(part, "name:"), `"`)
		}
	}
	return ""
}

func (h *WasmHelper) parseCompressionTag(tag string) WasmCompression {
	for _, part := range strings.Fields(tag) {
		if strings.HasPrefix(part, `compression:"`) {
			val := strings.Trim(strings.TrimPrefix(part, "compression:"), `"`)
			if strings.EqualFold(val, "brotli") {
				return WasmCompressionBrotli
			}
		}
	}
	return WasmCompressionGzip
}

func (h *WasmHelper) hasCtxStructs(structs []structInfo) bool {
	for _, s := range structs {
		if s.KeyName != "" {
			return true
		}
	}
	return false
}

func (h *WasmHelper) hasTimeFields(structs []structInfo) bool {
	for _, s := range structs {
		for _, f := range s.Fields {
			if f.Type == "time.Time" {
				return true
			}
		}
	}
	return false
}

func (h *WasmHelper) ctxTypeName(structName string) string {
	return strings.ToLower(structName[:1]) + structName[1:] + "Context"
}

func (h *WasmHelper) ctxFuncName(structName string) string { return structName + "Context" }
