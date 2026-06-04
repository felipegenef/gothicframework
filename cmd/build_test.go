package cmd

import (
	"os"
	"testing"

	gothic_cli "github.com/felipegenef/gothicframework/v2/pkg/cli"
	"github.com/spf13/cobra"
)

// scaffoldSrc creates the minimal src/ tree the file-based router walks so that
// FileBasedRouter.Render succeeds without any real page files.
func scaffoldSrc(t *testing.T) {
	t.Helper()
	for _, d := range []string{"src/pages", "src/components", "src/api", "src/routes"} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
}

func TestNewBuildCommandCli(t *testing.T) {
	cli := gothic_cli.NewCli()
	cmd := newBuildCommandCli(&cli)
	if cmd.cli == nil {
		t.Fatal("expected cli to be set on BuildCommand")
	}
}

func TestBuildSucceedsWithScaffold(t *testing.T) {
	chdirTemp(t)
	scaffoldSrc(t)
	writeConfig(t, `{"projectName":"demo","goModuleName":"demo"}`)

	cli := gothic_cli.NewCli()
	cmd := newBuildCommandCli(&cli)
	if err := cmd.Build(); err != nil {
		t.Fatalf("Build() unexpected error: %v", err)
	}
	if _, err := os.Stat("src/routes/routes_gen.go"); err != nil {
		t.Errorf("expected routes_gen.go to be generated: %v", err)
	}
}

func TestBuildFailsWithoutConfig(t *testing.T) {
	chdirTemp(t)
	// No gothic-config.json present: Templ.Render succeeds (no templ files),
	// then GetConfig must fail.
	cli := gothic_cli.NewCli()
	cmd := newBuildCommandCli(&cli)
	if err := cmd.Build(); err == nil {
		t.Fatal("expected Build() to fail without gothic-config.json")
	}
}

func TestBuildFailsWithoutSrcPages(t *testing.T) {
	chdirTemp(t)
	writeConfig(t, `{"projectName":"demo","goModuleName":"demo"}`)
	// No src/ tree: FileBasedRouter.Render must fail walking ./src/pages.
	cli := gothic_cli.NewCli()
	cmd := newBuildCommandCli(&cli)
	if err := cmd.Build(); err == nil {
		t.Fatal("expected Build() to fail without src/pages directory")
	}
}

func TestNewBuildCommandRunE(t *testing.T) {
	chdirTemp(t)
	scaffoldSrc(t)
	writeConfig(t, `{"projectName":"demo","goModuleName":"demo"}`)

	runE := newBuildCommand(gothic_cli.NewCli())
	if err := runE(&cobra.Command{}, nil); err != nil {
		t.Fatalf("build RunE unexpected error: %v", err)
	}
}
