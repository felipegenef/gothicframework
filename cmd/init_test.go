package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	gothic_cli "github.com/felipegenef/gothicframework/v2/pkg/cli"
	"github.com/felipegenef/gothicframework/v2/pkg/data"
)

func runInitInDir(t *testing.T, dir string) error {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(orig)

	cli := gothic_cli.NewCli()
	cliData := data.DefaultCLIData
	cliData.ProjectName = "test-project"
	cliData.GoModName = "testmod"

	cmd := &InitCommand{cli: &cli, gothicCliData: cliData}
	return cmd.initializeProject()
}

func TestInitCreatesAllTemplateFiles(t *testing.T) {
	dir := t.TempDir()
	if err := runInitInDir(t, dir); err != nil {
		t.Fatalf("initializeProject: %v", err)
	}

	// User-facing deployment templates that must still be seeded onto disk.
	expected := []string{
		".gothicCli/templates/Dockerfile-template",
		".gothicCli/templates/samconfig-template.toml",
		".gothicCli/templates/sam-template.yaml",
	}
	for _, f := range expected {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("missing template file: %s", f)
		}
	}

	// As of v2.17 these four templates ship inside the CLI binary's embed.FS
	// and must NOT be written to the user's project tree (they are an
	// implementation detail; historic on-disk drift caused silent breakage).
	mustNotExist := []string{
		".gothicCli/templates/wasm/topic_gen.go",
		".gothicCli/templates/wasm/wasm_page_main.go",
		".gothicCli/templates/wasm/wasm_topic_manager_main.go",
		".gothicCli/templates/routes_gen.go",
	}
	for _, f := range mustNotExist {
		if _, err := os.Stat(filepath.Join(dir, f)); err == nil {
			t.Errorf("template %s should be embedded, not written to disk", f)
		}
	}
}

func TestInitCreatesSrcStructure(t *testing.T) {
	dir := t.TempDir()
	if err := runInitInDir(t, dir); err != nil {
		t.Fatalf("initializeProject: %v", err)
	}

	expected := []string{
		"src/routes/routes_gen.go",
		"src/pages/index.templ",
		"src/pages/revalidate.templ",
		"src/layouts/layout.templ",
		"src/components/helloWorld.templ",
		"src/components/lazyLoad.templ",
		"src/css/app.css",
		"src/api/helloWorld.go",
	}
	for _, f := range expected {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("missing src file: %s", f)
		}
	}
}

func TestInitCreatesPublicAssets(t *testing.T) {
	dir := t.TempDir()
	if err := runInitInDir(t, dir); err != nil {
		t.Fatalf("initializeProject: %v", err)
	}

	expected := []string{
		"public/imageExample/blurred.png",
		"public/imageExample/original.png",
		"public/favicon.ico",
		"public/styles.css",
	}
	for _, f := range expected {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("missing public asset: %s", f)
		}
	}
}

func TestInitCreatesRootFiles(t *testing.T) {
	dir := t.TempDir()
	if err := runInitInDir(t, dir); err != nil {
		t.Fatalf("initializeProject: %v", err)
	}

	expected := []string{
		"main.go",
		".env",
		".gitignore",
		"gothic-config.json",
		"makefile",
		"tailwind.config.js",
	}
	for _, f := range expected {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("missing root file: %s", f)
		}
	}
}

func TestInitSubstitutesGoModName(t *testing.T) {
	dir := t.TempDir()
	if err := runInitInDir(t, dir); err != nil {
		t.Fatalf("initializeProject: %v", err)
	}

	mainGo, err := os.ReadFile(filepath.Join(dir, "main.go"))
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	if !strings.Contains(string(mainGo), "testmod") {
		t.Errorf("main.go does not contain go module name 'testmod'")
	}
}

func TestInitIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	if err := runInitInDir(t, dir); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if err := runInitInDir(t, dir); err != nil {
		t.Fatalf("second run (idempotent): %v", err)
	}
}
