package routes

// RouteConfig here uses an `any` ClientSideState so tests can assign a
// non-function identifier and exercise the error branch.
type RouteConfig struct {
	ClientSideState any
	WasmCompression string
	WasmCompiler    string
}
