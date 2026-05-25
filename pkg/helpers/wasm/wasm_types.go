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

// TopicFieldData holds data for one field in a topic struct.
type TopicFieldData struct {
	Name string
	Type string
}

// TopicTypeData holds data for a topic type struct declaration.
type TopicTypeData struct {
	TypeName string
	Fields   []TopicFieldData
}

// PerFieldCodec carries per-field codec lines for the consumer (page) template.
// Distinct from FieldCodec (which models a single line in a whole-struct codec).
type PerFieldCodec struct {
	FieldName string // Go field name
	FieldType string // Go field type as written in source (e.g. "int", "[]Item")
	EncLines  string // encoder lines (references v.<FieldName>)
	DecLines  string // decoder lines (references v.<FieldName>)
}

// WasmTopicFuncData holds data for one WASM-side topic constructor + Set method.
type WasmTopicFuncData struct {
	CtorName    string
	TypeName    string
	StructName  string
	KeyName     string
	Fields      []TopicFieldData
	FieldCodecs []PerFieldCodec // one entry per source struct field, in declaration order
}

// ServerTopicFuncData holds data for one server-side topic stub.
type ServerTopicFuncData struct {
	CtorName   string
	TypeName   string
	StructName string
	Fields     []TopicFieldData
}

// MountFnData holds data for an AddXxxContext() mount function.
type MountFnData struct {
	FuncName         string
	WasmName         string
	Compression      WasmCompression
	CompressionConst string // "routes.GZIP" or "routes.BROTLI" — used in generated Go code
}

// TopicGenData drives topic_gen.go.tmpl.
type TopicGenData struct {
	PkgName     string
	HasCtx      bool
	HasTime     bool // true when any struct field has type time.Time
	Codecs      []StructCodecData
	KeyVars     []KeyVarData
	TopicTypes  []TopicTypeData
	ServerFuncs []ServerTopicFuncData
	MountFns    []MountFnData
}

// WasmPageMainData drives wasm_page_main.go.tmpl.
type WasmPageMainData struct {
	SourceFile     string
	StdImports     []string
	Codecs         []StructCodecData
	KeyVars        []KeyVarData
	TopicTypes     []TopicTypeData
	WasmFuncs      []WasmTopicFuncData
	TopicSnippets  []string
	Body           string
	Helpers        []string
}

// ManagerFieldData carries per-field information for the manager template.
// One entry per source-struct field, in declaration order.
type ManagerFieldData struct {
	FieldName   string // Go field name, e.g. "Pings"
	EncodeLines string // body of inline encode snippet referencing v.<FieldName>
	DecodeLines string // body of inline decode snippet referencing v.<FieldName>
	CaptureBody string // body of _capture<FieldName>(d *Decoder) []byte (from Phase 1)
}

// WasmTopicManagerMainData drives wasm_topic_manager_main.go.tmpl.
type WasmTopicManagerMainData struct {
	StructName     string
	KeyName        string
	HasTime        bool // true when any struct field has type time.Time
	Codecs         []StructCodecData
	TopicSnippets  []string
	Fields         []ManagerFieldData // one entry per source struct field, in declaration order
}

// structInfo / fieldInfo are the parsed representation of src/topics/*.go.

type structInfo struct {
	Name         string
	KeyName      string
	Compression  WasmCompression
	Fields       []fieldInfo
	AccessorName string // var name from "var X = CreateTopic(...)", falls back to struct-derived name
}

type fieldInfo struct {
	Name      string
	Type      string
	TypeRef   typeRef // populated by parseStructsFromSource via typeRefFromExpr
	GothicTag string
}
