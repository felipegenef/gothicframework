package cmd

import (
	"os"
	"testing"

	gothic_cli "github.com/felipegenef/gothicframework/v2/pkg/cli"
	cli_data "github.com/felipegenef/gothicframework/v2/pkg/data"
)

// withStdin replaces os.Stdin with a pipe carrying the given input for the
// duration of fn, so functions using fmt.Scanln can be driven in tests.
func withStdin(t *testing.T, input string, fn func()) {
	t.Helper()
	orig := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdin = r
	defer func() { os.Stdin = orig }()
	if _, err := w.WriteString(input); err != nil {
		t.Fatalf("write stdin: %v", err)
	}
	_ = w.Close()
	fn()
}

func newTestInitCommand() InitCommand {
	cli := gothic_cli.NewCli()
	return NewInitCommandCli(&cli, cli_data.DefaultCLIData)
}

func TestNewInitCommandCli(t *testing.T) {
	cli := gothic_cli.NewCli()
	cmd := NewInitCommandCli(&cli, cli_data.DefaultCLIData)
	if cmd.cli == nil {
		t.Fatal("expected cli set on InitCommand")
	}
}

func TestPromptForProjectNameValid(t *testing.T) {
	cmd := newTestInitCommand()
	var got string
	var err error
	withStdin(t, "my-app\n", func() { got, err = cmd.promptForProjectName() })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "my-app" {
		t.Errorf("got %q, want my-app", got)
	}
}

func TestPromptForProjectNameInvalid(t *testing.T) {
	cmd := newTestInitCommand()
	var err error
	withStdin(t, "Invalid_Name\n", func() { _, err = cmd.promptForProjectName() })
	if err == nil {
		t.Fatal("expected error for non-kebab project name")
	}
}

func TestPromptForGoModNameValid(t *testing.T) {
	cmd := newTestInitCommand()
	var got string
	var err error
	withStdin(t, "example.com/mymod\n", func() { got, err = cmd.promptForGoModName() })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "example.com/mymod" {
		t.Errorf("got %q, want example.com/mymod", got)
	}
}

func TestPromptForGoModNameEmpty(t *testing.T) {
	cmd := newTestInitCommand()
	var err error
	withStdin(t, "\n", func() { _, err = cmd.promptForGoModName() })
	if err == nil {
		t.Fatal("expected error for empty go module name")
	}
}
