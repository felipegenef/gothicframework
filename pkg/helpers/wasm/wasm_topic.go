package helpers

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"github.com/felipegenef/gothicframework/pkg/helpers/wasm/astx"
)

// Topic scanning and parsing.
//
// Reads src/topics/*.go, parses struct definitions and type aliases,
// generates topic_gen.go (server-side helpers), and produces inlinable
// user code snippets for the WASM build pipeline.

const tmplTopicGen = ".gothicCli/templates/wasm/topic_gen.go"

// resolveTopicSourceDir returns the directory to scan for topic definitions
// and the generated-file name. Returns ("","", false) if src/topics/ does not exist.
func resolveTopicSourceDir() (dir, genFile string, ok bool) {
	if _, err := os.Stat("src/topics"); err == nil {
		return "src/topics", "topic_gen.go", true
	}
	return "", "", false
}

// collectTopicSnippets reads src/topics/*.go, parses struct definitions,
// generates topic_gen.go (server side), and returns inlinable user code
// snippets and the parsed structs for template rendering.
func (h *WasmHelper) collectTopicSnippets() (snippets []string, structs []structInfo, aliases map[string]string, refAliases map[string]typeRef) {
	sourceDir, genFile, ok := resolveTopicSourceDir()
	if !ok {
		return nil, nil, nil, nil
	}
	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		return nil, nil, nil, nil
	}

	type rawFile struct{ name, src string }
	var files []rawFile
	var allStructs []structInfo
	allAliases := make(map[string]string)
	allRefAliases := make(map[string]typeRef)
	pkgName := "gothicwasm"

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || e.Name() == genFile {
			continue
		}
		data, err := os.ReadFile(filepath.Join(sourceDir, e.Name()))
		if err != nil {
			continue
		}
		src := string(data)
		if fset := token.NewFileSet(); pkgName == "gothicwasm" {
			if pf, err := parser.ParseFile(fset, "", src, 0); err == nil && pf.Name != nil {
				pkgName = pf.Name.Name
			}
		}
		structs, aliases, refA := h.parseStructsFromSource(src)
		allStructs = append(allStructs, structs...)
		for k, v := range aliases {
			allAliases[k] = v
		}
		for k, v := range refA {
			allRefAliases[k] = v
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
				"error: duplicate topic key name %q — used by both %s and %s in %s/.\n"+
					"  Each topic struct must have a unique key name.\n",
				s.KeyName, prev, s.Name, sourceDir)
			os.Exit(1)
		}
		seenKeys[s.KeyName] = s.Name
	}

	h.writeTopicKeyStubs(allStructs, allAliases, allRefAliases, pkgName, sourceDir, genFile)

	// Normalize user-authored `var X = CreateTopic(...)` declarations to
	// `var _ = CreateTopic(...)` on disk, so the generated `func X()` in
	// topic_gen.go does not collide with the original var. The CLI has already
	// captured the accessor name into structInfo.AccessorName above.
	if err := normalizeTopicDeclarations(sourceDir, allStructs); err != nil {
		fmt.Fprintf(os.Stderr, "wasm: normalize topic declarations: %v\n", err)
		os.Exit(1)
	}

	for _, f := range files {
		src, err := astx.StripPackageAndImports(f.src)
		if err != nil {
			fmt.Fprintf(os.Stderr, "topic strip %s: %v\n", f.name, err)
			os.Exit(1)
		}
		src, err = h.rewriteAutoKeys(src)
		if err != nil {
			fmt.Fprintf(os.Stderr, "wasm: rewrite auto-keys %s: %v\n", f.name, err)
			os.Exit(1)
		}
		src = strings.TrimSpace(src)
		if src != "" {
			snippets = append(snippets, "// --- from "+sourceDir+"/"+f.name+" ---\n"+src)
		}
	}
	return snippets, allStructs, allAliases, allRefAliases
}

// PregenerateTopicStubs runs the topic_gen.go generation pass without invoking
// the WASM build. This is required before ScanPages so the `func PageTopic()`
// (etc.) accessors exist as real symbols by the time go/packages loads the
// project — otherwise pages that call `PageTopic()` fail to type-check.
//
// Safe to call repeatedly; a no-op when no src/topics/ dir exists.
func (h *WasmHelper) PregenerateTopicStubs() {
	// Ensure on-disk templates (including topic_gen.go.tmpl) match the CLI
	// version. UpdateFromTemplate reads from disk, so the template must exist
	// before collectTopicSnippets calls it.
	if err := h.EnsureWasmTemplates(); err != nil {
		fmt.Fprintf(os.Stderr, "wasm: ensure templates: %v\n", err)
		return
	}
	h.collectTopicSnippets()
}

func (h *WasmHelper) writeTopicKeyStubs(structs []structInfo, aliases map[string]string, refAliases map[string]typeRef, pkgName, sourceDir, genFile string) {
	outPath := filepath.Join(sourceDir, genFile)
	if len(structs) == 0 {
		_ = os.Remove(outPath)
		return
	}

	codecs, err := h.buildCodecData(structs, aliases, refAliases)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: topic codec: %v\n", err)
		os.Exit(1)
	}

	data := TopicGenData{
		PkgName:     pkgName,
		HasTopics:      h.hasTopicStructs(structs),
		HasTime:     h.hasTimeFields(structs),
		Codecs:      codecs,
		KeyVars:     h.buildKeyVarData(structs),
		TopicTypes:  h.buildTopicTypeData(structs),
		ServerFuncs: h.buildServerTopicFuncData(structs),
		MountFns:    h.buildMountFnData(structs),
	}

	_ = h.Template.UpdateFromTemplate(tmplTopicGen, outPath, data)
}

// topicMeta carries the (keyName, compression, accessorName, componentFnName) extracted from a CreateTopic
// call, indexed by the topic's underlying struct name.
type topicMeta struct {
	KeyName         string
	Compression     WasmCompression
	AccessorName    string // the variable name from "var X = CreateTopic(...)"
	ComponentFnName string // overrides Add<StructName>Topic mount helper name
}

// parseStructsFromSource parses struct definitions and type aliases from a Go source string.
// typeAliases maps alias name → underlying type string (e.g. "MyInt" → "int").
func (h *WasmHelper) parseStructsFromSource(src string) (structs []structInfo, typeAliases map[string]string, typeRefAliases map[string]typeRef) {
	typeAliases = make(map[string]string)
	typeRefAliases = make(map[string]typeRef)
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", src, parser.ParseComments)
	if err != nil {
		return nil, typeAliases, typeRefAliases
	}

	// First pass: scan for CreateTopic(T{}, TopicConfig{...}) calls so the
	// metadata is available when struct definitions are processed below.
	topicMetas := collectCreateTopicMetas(f)

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
				typeRefAliases[ts.Name.Name] = Named{Name: t.Name}
			case *ast.ArrayType, *ast.MapType, *ast.StarExpr:
				// type Labels []string, type MyMap map[K]V, type MyPtr *T
				if s := h.astTypeString(ts.Type); s != "" {
					typeAliases[ts.Name.Name] = s
				}
				if tref, err := typeRefFromExpr(ts.Type); err == nil {
					typeRefAliases[ts.Name.Name] = tref
				}
				_ = t
			case *ast.StructType:
				si := structInfo{Name: ts.Name.Name}
				for _, field := range t.Fields.List {
					typ := h.astTypeString(field.Type)
					tref, _ := typeRefFromExpr(field.Type)
					var tag, nameTag string
					var compression WasmCompression
					if field.Tag != nil {
						tag, nameTag, compression = h.parseFieldTag(field.Tag.Value)
					} else {
						compression = WasmCompressionGzip
					}
					_ = nameTag
					_ = compression
					for _, name := range field.Names {
						si.Fields = append(si.Fields, fieldInfo{Name: name.Name, Type: typ, TypeRef: tref, GothicTag: tag})
					}
				}
				// New: CreateTopic(T{}, TopicConfig{...}) — apply metadata if
				// any call references this struct type.
				if meta, ok := topicMetas[si.Name]; ok {
					si.KeyName = meta.KeyName
					si.Compression = meta.Compression
					si.AccessorName = meta.AccessorName
					si.ComponentFnName = meta.ComponentFnName
				}
				structs = append(structs, si)
			}
		}
	}
	return structs, typeAliases, typeRefAliases
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

// parseFieldTag extracts the gothic, name, and compression values from a
// struct field tag using reflect.StructTag, which correctly handles quoted
// characters and other edge cases that ad-hoc string splitting would miss.
// tagValue is the raw tag literal as it appears in the AST (including the
// surrounding backticks).
func (h *WasmHelper) parseFieldTag(tagValue string) (gothic, name string, compression WasmCompression) {
	raw := reflect.StructTag(strings.Trim(tagValue, "`"))
	gothic, _ = raw.Lookup("gothic")
	name, _ = raw.Lookup("name")
	compression = WasmCompressionGzip
	if c, ok := raw.Lookup("compression"); ok && strings.EqualFold(c, "brotli") {
		compression = WasmCompressionBrotli
	}
	return
}

func (h *WasmHelper) hasTopicStructs(structs []structInfo) bool {
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

func (h *WasmHelper) topicTypeName(structName string) string {
	return strings.ToLower(structName[:1]) + structName[1:] + "Topic"
}

func (h *WasmHelper) topicFuncName(structName string) string { return structName + "Topic" }

// topicFuncNameFor returns the accessor function name for a topic struct.
// It prefers the captured variable name (e.g. "PageTopic" from "var PageTopic = CreateTopic(...)"),
// falling back to the struct-derived name when the var is blank or missing.
func (h *WasmHelper) topicFuncNameFor(s structInfo) string {
	if s.AccessorName != "" {
		return s.AccessorName
	}
	return h.topicFuncName(s.Name)
}

// componentFuncNameFor returns the mount/component function name for a topic struct.
// It prefers ComponentFnName when set, falling back to "Add" + topicFuncNameFor.
func (h *WasmHelper) componentFuncNameFor(s structInfo) string {
	if s.ComponentFnName != "" {
		return s.ComponentFnName
	}
	return "Add" + h.topicFuncNameFor(s)
}

// collectCreateTopicMetas walks the AST for `var _ = CreateTopic(T{},
// TopicConfig{Name: "...", Compression: BROTLI})` (or any assignment whose RHS
// is such a call) and returns a map from the underlying struct type name T to
// the extracted metadata.
//
// The first call argument must be a composite literal whose Type is an
// identifier — e.g. `MyStruct{}`. The second argument must be a TopicConfig
// composite literal; its Name/Compression fields are read by key.
func collectCreateTopicMetas(f *ast.File) map[string]topicMeta {
	metas := make(map[string]topicMeta)
	ast.Inspect(f, func(n ast.Node) bool {
		gd, ok := n.(*ast.GenDecl)
		if !ok || gd.Tok != token.VAR {
			return true
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, val := range vs.Values {
				call, ok := val.(*ast.CallExpr)
				if !ok {
					continue
				}
				// Match CreateTopic identifier; accept selector form too
				// (e.g. wasm.CreateTopic) in case of qualified imports.
				if !isCreateTopicCall(call.Fun) {
					continue
				}
				if len(call.Args) < 2 {
					continue
				}
				structName := topicStructNameFromArg(call.Args[0])
				if structName == "" {
					continue
				}
				name, compression, subscriberFnName, componentFnName := parseTopicConfigArg(call.Args[1])
				// Capture the var name (e.g. "PageTopic" from "var PageTopic = CreateTopic(...)").
				// If the var is blank ("_") or missing, fall back to struct-derived name.
				// No warning — blank identifier on disk is the CLI's own normalized form
				// after rewriting "var PageTopic = CreateTopic(...)" → "var _ = CreateTopic(...)".
				var accessorName string
				if i < len(vs.Names) {
					if n := vs.Names[i].Name; n != "_" {
						accessorName = n
					}
				}
				// SubscriberFnName overrides the accessor name when set.
				if subscriberFnName != "" {
					accessorName = subscriberFnName
				}
				metas[structName] = topicMeta{KeyName: name, Compression: compression, AccessorName: accessorName, ComponentFnName: componentFnName}
			}
		}
		return true
	})
	return metas
}

func isCreateTopicCall(fun ast.Expr) bool {
	switch f := fun.(type) {
	case *ast.Ident:
		return f.Name == "CreateTopic"
	case *ast.SelectorExpr:
		return f.Sel != nil && f.Sel.Name == "CreateTopic"
	case *ast.IndexExpr:
		// CreateTopic[T] — generic instantiation form
		return isCreateTopicCall(f.X)
	case *ast.IndexListExpr:
		return isCreateTopicCall(f.X)
	}
	return false
}

// topicStructNameFromArg extracts the struct type name from the first
// argument to CreateTopic. Accepts a composite literal (T{}) or a bare type
// identifier (T) — the latter is unusual but defensive.
func topicStructNameFromArg(arg ast.Expr) string {
	switch a := arg.(type) {
	case *ast.CompositeLit:
		if a.Type == nil {
			return ""
		}
		switch t := a.Type.(type) {
		case *ast.Ident:
			return t.Name
		case *ast.SelectorExpr:
			if t.Sel != nil {
				return t.Sel.Name
			}
		}
	case *ast.Ident:
		return a.Name
	}
	return ""
}

// parseCompressionExpr resolves a Compression field value expression to an
// internal WasmCompression. Accepted forms:
//
//	"BROTLI"          — legacy string literal (backward compatible)
//	BROTLI            — bare identifier (dot-import)
//	wasm.BROTLI       — selector expression (qualified import)
//
// Anything else (including GZIP / wasm.GZIP) maps to the default WasmCompressionGzip.
func parseCompressionExpr(expr ast.Expr) WasmCompression {
	identName := ""
	switch v := expr.(type) {
	case *ast.BasicLit:
		if v.Kind == token.STRING {
			if unq, err := strconv.Unquote(v.Value); err == nil {
				identName = strings.ToUpper(unq)
			}
		}
	case *ast.Ident:
		identName = strings.ToUpper(v.Name)
	case *ast.SelectorExpr:
		identName = strings.ToUpper(v.Sel.Name)
	}
	if identName == "BROTLI" {
		return WasmCompressionBrotli
	}
	return WasmCompressionGzip
}

// parseTopicConfigArg pulls Name, Compression, SubscriberFnName, and ComponentFnName
// from a TopicConfig composite literal. Compression defaults to GZIP; "BROTLI"
// (case-insensitive) → brotli.
func parseTopicConfigArg(arg ast.Expr) (name string, compression WasmCompression, subscriberFnName string, componentFnName string) {
	compression = WasmCompressionGzip
	cl, ok := arg.(*ast.CompositeLit)
	if !ok {
		return
	}
	for _, elt := range cl.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		keyIdent, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}
		switch keyIdent.Name {
		case "Name":
			if bl, ok := kv.Value.(*ast.BasicLit); ok && bl.Kind == token.STRING {
				if unq, err := strconv.Unquote(bl.Value); err == nil {
					name = unq
				}
			}
		case "Compression":
			compression = parseCompressionExpr(kv.Value)
		case "SubscriberFnName":
			if bl, ok := kv.Value.(*ast.BasicLit); ok && bl.Kind == token.STRING {
				if unq, err := strconv.Unquote(bl.Value); err == nil {
					subscriberFnName = unq
				}
			}
		case "ComponentFnName":
			if bl, ok := kv.Value.(*ast.BasicLit); ok && bl.Kind == token.STRING {
				if unq, err := strconv.Unquote(bl.Value); err == nil {
					componentFnName = unq
				}
			}
		}
	}
	return
}

// normalizeTopicDeclarations rewrites every `var <AccessorName> = CreateTopic(`
// line in the topic source files to `var _ = CreateTopic(`. This is done on
// disk AFTER the CLI has captured AccessorName into structInfo, so the
// generated `func <AccessorName>() *...Topic` in topic_gen.go no longer
// collides with the user's original var declaration.
//
// It is intentionally line-based (not AST-rewrite) to preserve formatting and
// comments exactly as the user authored them — only the leading "var Name ="
// fragment changes.
func normalizeTopicDeclarations(sourceDir string, structs []structInfo) error {
	// Build set of accessor names per file. We don't know which file each
	// accessor lives in without re-parsing, so we just attempt the rewrite on
	// every .go file in the directory (cheap, idempotent).
	accessors := make(map[string]struct{}, len(structs))
	for _, s := range structs {
		if s.AccessorName != "" {
			accessors[s.AccessorName] = struct{}{}
		}
	}
	if len(accessors) == 0 {
		return nil
	}
	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		path := filepath.Join(sourceDir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		orig := string(data)
		out := orig
		for name := range accessors {
			// Replace "var <name> = CreateTopic(" → "var _ = CreateTopic("
			needle := "var " + name + " = CreateTopic("
			if strings.Contains(out, needle) {
				out = strings.ReplaceAll(out, needle, "var _ = CreateTopic(")
			}
		}
		if out != orig {
			if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
				return fmt.Errorf("write %s: %w", path, err)
			}
		}
	}
	return nil
}

