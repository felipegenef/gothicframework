package routes

type RouteConfig struct {
	ClientSideState func()
	WasmCompression string
	WasmCompiler    string
}
