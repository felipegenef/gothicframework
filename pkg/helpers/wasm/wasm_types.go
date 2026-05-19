package helpers

// Shared data structs for WASM template rendering.
// These types feed into the .gothicCli/templates/wasm/*.tmpl files via
// TemplateHelper.UpdateFromTemplate.

// FieldCodec holds pre-computed codec lines for a single struct field.
type FieldCodec struct {
	Name    string
	EncLine string
	DecLine string
}

// StructCodecData holds codec render data for one struct type.
type StructCodecData struct {
	Name   string
	Fields []FieldCodec
}

// KeyVarData holds data for a BinaryKey var declaration.
type KeyVarData struct {
	StructName string
	KeyName    string
}

// CtxFieldData holds data for one field in a context struct.
type CtxFieldData struct {
	Name string
	Type string
}

// CtxTypeData holds data for a context type struct declaration.
type CtxTypeData struct {
	TypeName string
	Fields   []CtxFieldData
}

// PerFieldCodec carries per-field codec lines for the consumer (page) template.
// Distinct from FieldCodec (which models a single line in a whole-struct codec).
type PerFieldCodec struct {
	FieldName string // Go field name
	FieldType string // Go field type as written in source (e.g. "int", "[]Item")
	EncLines  string // encoder lines (references v.<FieldName>)
	DecLines  string // decoder lines (references v.<FieldName>)
}

// WasmCtxFuncData holds data for one WASM-side context constructor + Set method.
type WasmCtxFuncData struct {
	CtorName    string
	TypeName    string
	StructName  string
	KeyName     string
	Fields      []CtxFieldData
	FieldCodecs []PerFieldCodec // one entry per source struct field, in declaration order
}

// ServerCtxFuncData holds data for one server-side context stub.
type ServerCtxFuncData struct {
	CtorName   string
	TypeName   string
	StructName string
	Fields     []CtxFieldData
}

// MountFnData holds data for an AddXxxContext() mount function.
type MountFnData struct {
	FuncName         string
	WasmName         string
	Compression      WasmCompression
	CompressionConst string // "routes.GZIP" or "routes.BROTLI" — used in generated Go code
}

// ContextGenData drives context_gen.go.tmpl.
type ContextGenData struct {
	PkgName     string
	HasCtx      bool
	HasTime     bool // true when any struct field has type time.Time
	Codecs      []StructCodecData
	KeyVars     []KeyVarData
	CtxTypes    []CtxTypeData
	ServerFuncs []ServerCtxFuncData
	MountFns    []MountFnData
}

// WasmPageMainData drives wasm_page_main.go.tmpl.
type WasmPageMainData struct {
	SourceFile  string
	StdImports  []string
	Codecs      []StructCodecData
	KeyVars     []KeyVarData
	CtxTypes    []CtxTypeData
	WasmFuncs   []WasmCtxFuncData
	CtxSnippets []string
	Body        string
}

// ManagerFieldData carries per-field information for the manager template.
// One entry per source-struct field, in declaration order.
type ManagerFieldData struct {
	FieldName   string // Go field name, e.g. "Pings"
	EncodeLines string // body of inline encode snippet referencing v.<FieldName>
	DecodeLines string // body of inline decode snippet referencing v.<FieldName>
	CaptureBody string // body of _capture<FieldName>(d *Decoder) []byte (from Phase 1)
}

// WasmCtxManagerMainData drives wasm_ctx_manager_main.go.tmpl.
type WasmCtxManagerMainData struct {
	StructName  string
	KeyName     string
	HasTime     bool // true when any struct field has type time.Time
	Codecs      []StructCodecData
	CtxSnippets []string
	Fields      []ManagerFieldData // one entry per source struct field, in declaration order
}

// structInfo / fieldInfo are the parsed representation of src/context/*.go.

type structInfo struct {
	Name        string
	KeyName     string
	Compression WasmCompression
	Fields      []fieldInfo
}

type fieldInfo struct {
	Name      string
	Type      string
	GothicTag string
}
