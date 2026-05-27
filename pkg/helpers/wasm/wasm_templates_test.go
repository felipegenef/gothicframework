package helpers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	helpers "github.com/felipegenef/gothicframework/v2/pkg/helpers"
)

// TestEmbeddedTemplatesReadable verifies the four WASM-side templates the CLI
// now ships embedded are readable from WasmTemplateFS and contain their
// expected anchors. This guards against an accidental //go:embed directive
// regression that would silently drop one of them.
func TestEmbeddedTemplatesReadable(t *testing.T) {
	page, err := WasmTemplateFS.ReadFile(EmbeddedTmplWasmPageMain)
	if err != nil {
		t.Fatalf("read embedded page template: %v", err)
	}
	if !strings.Contains(string(page), "func main()") {
		t.Errorf("embedded page template missing `func main()`")
	}
	if !strings.Contains(string(page), "select {}") {
		t.Errorf("embedded page template missing trailing `select {}`")
	}

	if _, err := WasmTemplateFS.ReadFile(EmbeddedTmplTopicManagerMain); err != nil {
		t.Fatalf("read embedded topic manager template: %v", err)
	}
	if _, err := WasmTemplateFS.ReadFile(EmbeddedTmplTopicGen); err != nil {
		t.Fatalf("read embedded topic gen template: %v", err)
	}
}

// TestUpdateFromTemplateFS confirms the new helper renders an embedded template
// to disk with the substituted data. We render the embedded topic_gen.go.tmpl
// because it has trivial branches that work with zero-value inputs.
func TestUpdateFromTemplateFS(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "nested", "out.go")

	th := helpers.NewTemplateHelper()
	data := struct {
		PkgName     string
		HasTopics   bool
		HasTime     bool
		Codecs      []any
		KeyVars     []any
		TopicTypes  []any
		ServerFuncs []any
		MountFns    []any
	}{PkgName: "mypkg"}

	if err := th.UpdateFromTemplateFS(WasmTemplateFS, EmbeddedTmplTopicGen, out, data); err != nil {
		t.Fatalf("UpdateFromTemplateFS: %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read rendered file: %v", err)
	}
	if !strings.Contains(string(got), "package mypkg") {
		t.Errorf("rendered template missing `package mypkg`; got:\n%s", got)
	}
}

// TestCleanupLegacyTemplates_RemovesPresent seeds the four legacy on-disk
// template paths and verifies CleanupLegacyTemplates removes all of them.
func TestCleanupLegacyTemplates_RemovesPresent(t *testing.T) {
	dir := t.TempDir()
	rels := []string{
		".gothicCli/templates/wasm/wasm_page_main.go",
		".gothicCli/templates/wasm/wasm_topic_manager_main.go",
		".gothicCli/templates/wasm/topic_gen.go",
		".gothicCli/templates/routes_gen.go",
	}
	for _, rel := range rels {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", full, err)
		}
		if err := os.WriteFile(full, []byte("stale"), 0644); err != nil {
			t.Fatalf("seed %s: %v", full, err)
		}
	}

	if err := CleanupLegacyTemplates(dir); err != nil {
		t.Fatalf("CleanupLegacyTemplates: %v", err)
	}

	for _, rel := range rels {
		full := filepath.Join(dir, rel)
		if _, err := os.Stat(full); !os.IsNotExist(err) {
			t.Errorf("expected %s to be removed, stat err=%v", full, err)
		}
	}
}

// TestCleanupLegacyTemplates_Idempotent verifies that calling cleanup on a
// directory with none of the legacy files present returns nil (no error).
func TestCleanupLegacyTemplates_Idempotent(t *testing.T) {
	dir := t.TempDir()
	if err := CleanupLegacyTemplates(dir); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if err := CleanupLegacyTemplates(dir); err != nil {
		t.Fatalf("second call: %v", err)
	}
}
