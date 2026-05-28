package helpers

import (
	"runtime"

	helpers "github.com/felipegenef/gothicframework/v2/pkg/helpers"
	"github.com/felipegenef/gothicframework/v2/pkg/helpers/wasm/astx"
)

const tinyGoVersion = "0.41.1"
const binaryenVersion = "117"

// WasmHelper manages the TinyGo toolchain and compiles WASM pages.
// It follows the same struct + method pattern as TailwindHelper and FileBasedRouteHelper.
type WasmHelper struct {
	Template        helpers.TemplateHelper
	Runtime         string
	Arch            string
	Version         string
	BinaryenVersion string
	ConfigOverride  string
	cache           *wasmCache
	astLoader       *astx.Loader
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
	IsComponent bool // true when scanned from componentsDir, false for pagesDir
	// LocalPackageDirs lists absolute directories of local (user-module)
	// packages whose helpers/types are referenced by this page. Used by the
	// WASM cache to invalidate when a transitively imported local package
	// changes on disk. Sorted alphabetically and de-duplicated by the scanner.
	LocalPackageDirs []string
	// UsedDeclSources contains the formatted Go source of each AST declaration
	// (func/const/type) that the page's ClientSideState body transitively
	// references in its own package. Sorted alphabetically for hash stability.
	// Used by the WASM cache to invalidate only when a referenced symbol's
	// source actually changes, rather than any file in the package.
	UsedDeclSources []string
}

func NewWasmHelper(goos, goarch string) WasmHelper {
	return WasmHelper{
		Template:        helpers.NewTemplateHelper(),
		Runtime:         goos,
		Arch:            goarch,
		Version:         tinyGoVersion,
		BinaryenVersion: binaryenVersion,
	}
}

// DefaultWasmHelper creates a WasmHelper using the current runtime's OS and architecture.
func DefaultWasmHelper() WasmHelper {
	return NewWasmHelper(runtime.GOOS, runtime.GOARCH)
}
