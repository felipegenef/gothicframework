package helpers

import (
	"runtime"

	helpers "github.com/felipegenef/gothicframework/pkg/helpers"
	"github.com/felipegenef/gothicframework/pkg/helpers/wasm/astx"
)

const tinyGoVersion = "0.41.1"

// WasmHelper manages the TinyGo toolchain and compiles WASM pages.
// It follows the same struct + method pattern as TailwindHelper and FileBasedRouteHelper.
type WasmHelper struct {
	Template       helpers.TemplateHelper
	Runtime        string
	Arch           string
	Version        string
	ConfigOverride string
	cache          *wasmCache
	astLoader      *astx.Loader
}

// WasmCompression is the compression algorithm for compiled WASM output.
// Mirrors routes.CompressionMethod to avoid a circular import with the helpers/routes package.
type WasmCompression int

const (
	WasmCompressionGzip   WasmCompression = iota // default (routes.GZIP == 0)
	WasmCompressionBrotli WasmCompression = iota // routes.BROTLI == 1
)

// WasmCompilerChoice mirrors routes.WasmCompiler to avoid circular imports.
type WasmCompilerChoice int

const (
	WasmCompilerGothicTinyGo WasmCompilerChoice = iota // default
	WasmCompilerLocalTinyGo
	WasmCompilerGolang
)

// WasmPage describes a single page that has a WASM state function.
type WasmPage struct {
	SourceFile  string
	FuncName    string
	FuncBody    string
	Imports     []string
	Helpers     []string
	HttpPath    string
	OutputName  string
	Compression WasmCompression
	Compiler    WasmCompilerChoice
}

func NewWasmHelper(goos, goarch string) WasmHelper {
	return WasmHelper{
		Template: helpers.NewTemplateHelper(),
		Runtime:  goos,
		Arch:     goarch,
		Version:  tinyGoVersion,
	}
}

// DefaultWasmHelper creates a WasmHelper using the current runtime's OS and architecture.
func DefaultWasmHelper() WasmHelper {
	return NewWasmHelper(runtime.GOOS, runtime.GOARCH)
}
