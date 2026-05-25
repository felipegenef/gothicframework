package helpers

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
)

// wasm_templates.go owns the on-disk WASM build templates. The CLI ships an
// embedded copy of `wasm_page_main.go` and `wasm_topic_manager_main.go`; the on-
// disk versions under `.gothicCli/templates/wasm/` are seeded at `init` time
// and live in the user's project so they CAN be customised — but in practice
// most users never touch them, and template drift between the CLI and an
// older project would silently break compiled WASM in subtle ways (e.g. a
// missing trailing `select {}` letting TinyGo's main return, after which
// every JS callback throws "Go program has already exited").
//
// To prevent that, `EnsureWasmTemplates` rewrites the on-disk copies from the
// embedded source on every WASM build. The write is idempotent — files are
// only touched if their bytes differ from the embedded version — so it does
// not perturb mtimes on otherwise-clean projects.

//go:embed embedded_templates/wasm_page_main.go.tmpl
//go:embed embedded_templates/wasm_topic_manager_main.go.tmpl
//go:embed embedded_templates/topic_gen.go.tmpl
var wasmTemplateFS embed.FS

// wasmTemplateMap pairs each embedded source path with its target on-disk
// location under the user's project root.
var wasmTemplateMap = map[string]string{
	"embedded_templates/wasm_page_main.go.tmpl":          tmplWasmPageMain,
	"embedded_templates/wasm_topic_manager_main.go.tmpl": tmplTopicManagerMain,
	"embedded_templates/topic_gen.go.tmpl":               tmplTopicGen,
}

// EnsureWasmTemplates refreshes the on-disk WASM templates from the embedded
// copies shipped with the CLI binary. If a file already exists with identical
// bytes it is left alone. Missing parent directories are created.
//
// This is called once at the start of GenerateAll so that every WASM build
// uses templates that match the CLI version currently running, regardless of
// when the project was originally initialised.
func (h *WasmHelper) EnsureWasmTemplates() error {
	for src, dst := range wasmTemplateMap {
		want, err := wasmTemplateFS.ReadFile(src)
		if err != nil {
			return fmt.Errorf("wasm: read embedded template %s: %w", src, err)
		}
		if existing, err := os.ReadFile(dst); err == nil && bytesEqual(existing, want) {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			return fmt.Errorf("wasm: mkdir %s: %w", filepath.Dir(dst), err)
		}
		if err := os.WriteFile(dst, want, 0644); err != nil {
			return fmt.Errorf("wasm: write template %s: %w", dst, err)
		}
	}
	return nil
}

// bytesEqual is a tiny stand-in for bytes.Equal so this file doesn't need
// to import "bytes" for a one-call function.
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
