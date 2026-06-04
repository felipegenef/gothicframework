package routes

// RouteConfig mirrors the production routes.RouteConfig shape closely enough
// that its type string ends in "helpers/routes.RouteConfig".
type RouteConfig struct {
	ClientSideState func()
	WasmCompression string
	WasmCompiler    string
}
