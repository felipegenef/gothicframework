package helpers

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
)

// wasm_templates.go owns the WASM build templates. As of v2.17 these live
// exclusively inside the CLI binary's embed.FS — they are an implementation
// detail of the build pipeline, not user-editable assets, so we no longer
// seed them onto disk under `.gothicCli/templates/wasm/`. Older projects that
// were initialised by earlier CLI versions may still have stale copies on
// disk; CleanupLegacyTemplates removes them at the start of a WASM build.

//go:embed embedded_templates/wasm_page_main.go.tmpl
//go:embed embedded_templates/wasm_topic_manager_main.go.tmpl
//go:embed embedded_templates/topic_gen.go.tmpl
var WasmTemplateFS embed.FS

// Embedded source paths used with UpdateFromTemplateFS.
const (
	EmbeddedTmplWasmPageMain      = "embedded_templates/wasm_page_main.go.tmpl"
	EmbeddedTmplTopicManagerMain  = "embedded_templates/wasm_topic_manager_main.go.tmpl"
	EmbeddedTmplTopicGen          = "embedded_templates/topic_gen.go.tmpl"
)

// legacyTemplatePaths is the list of on-disk template files that earlier CLI
// versions seeded into the user's project tree. These are now embedded in the
// CLI binary and the on-disk copies are removed on first build by a v2.17+
// CLI so they cannot drift out of sync with the binary.
var legacyTemplatePaths = []string{
	".gothicCli/templates/wasm/wasm_page_main.go",
	".gothicCli/templates/wasm/wasm_topic_manager_main.go",
	".gothicCli/templates/wasm/topic_gen.go",
	".gothicCli/templates/routes_gen.go",
}

// CleanupLegacyTemplates removes any on-disk copies of the four templates that
// pre-v2.17 CLIs used to seed under `.gothicCli/templates/`. Idempotent: files
// that are already absent are silently skipped. Each deletion is logged to
// stderr so users notice the one-time migration.
func CleanupLegacyTemplates(projectRoot string) error {
	for _, rel := range legacyTemplatePaths {
		path := filepath.Join(projectRoot, rel)
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("wasm: stat legacy template %s: %w", path, err)
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("wasm: remove legacy template %s: %w", path, err)
		}
		fmt.Fprintf(os.Stderr, "wasm: removed legacy on-disk template %s (now embedded in CLI binary)\n", path)
	}
	return nil
}
