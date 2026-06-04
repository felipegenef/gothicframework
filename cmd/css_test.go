package cmd

import (
	"testing"

	gothic_cli "github.com/felipegenef/gothicframework/v2/pkg/cli"
	"github.com/spf13/cobra"
)

func TestNewCssCommandCli(t *testing.T) {
	cli := gothic_cli.NewCli()
	cmd := newCssCommandCli(&cli)
	if cmd.cli == nil {
		t.Fatal("expected cli to be set on CssCommand")
	}
}

func TestCssCommandSucceedsWithFakeBinary(t *testing.T) {
	bin := writeFakeTailwind(t, true)
	chdirTemp(t)
	writeConfig(t, `{"projectName":"demo","goModuleName":"demo","tailwindBinary":"`+bin+`"}`)

	runE := newCssCommand(gothic_cli.NewCli())
	if err := runE(&cobra.Command{}, nil); err != nil {
		t.Fatalf("css RunE unexpected error: %v", err)
	}
}

func TestCssCommandPropagatesBinaryError(t *testing.T) {
	bin := writeFakeTailwind(t, false) // exits 1
	chdirTemp(t)
	writeConfig(t, `{"projectName":"demo","goModuleName":"demo","tailwindBinary":"`+bin+`"}`)

	runE := newCssCommand(gothic_cli.NewCli())
	if err := runE(&cobra.Command{}, nil); err == nil {
		t.Fatal("expected css RunE to fail when tailwind binary exits non-zero")
	}
}

func TestCssCommandFailsWithMissingBinaryOverride(t *testing.T) {
	chdirTemp(t)
	writeConfig(t, `{"projectName":"demo","goModuleName":"demo","tailwindBinary":"/nonexistent/tailwind-bin"}`)

	runE := newCssCommand(gothic_cli.NewCli())
	if err := runE(&cobra.Command{}, nil); err == nil {
		t.Fatal("expected css RunE to fail when tailwind binary override does not exist")
	}
}
