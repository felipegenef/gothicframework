package cmd

import (
	"os"
	"testing"

	gothic_cli "github.com/felipegenef/gothicframework/v2/pkg/cli"
	"github.com/spf13/cobra"
)

func TestWasmBuildCommandScanFailsOutsideModule(t *testing.T) {
	chdirTemp(t)
	writeConfig(t, `{"projectName":"demo","goModuleName":"demo"}`)
	// No go.mod / no Go packages: ScanPages -> astx.NewLoader(".") must fail,
	// and the RunE wraps it as "wasm: scan".
	runE := newWasmBuildCommand(gothic_cli.NewCli())
	if err := runE(&cobra.Command{}, nil); err == nil {
		t.Fatal("expected wasm build RunE to fail scanning outside a Go module")
	}
}

func TestWasmBuildCommandNoPages(t *testing.T) {
	chdirTemp(t)
	writeGoMod(t, "demo")
	scaffoldSrc(t)
	writeConfig(t, `{"projectName":"demo","goModuleName":"demo"}`)
	// Valid (empty) Go module + empty src tree: ScanPages succeeds returning no
	// pages, so the RunE prints "no pages" and returns nil without TinyGo.
	runE := newWasmBuildCommand(gothic_cli.NewCli())
	if err := runE(&cobra.Command{}, nil); err != nil {
		t.Fatalf("wasm build RunE with no pages should succeed, got %v", err)
	}
}

func TestWasmCleanCommand(t *testing.T) {
	chdirTemp(t)
	if err := os.MkdirAll("public/wasm", 0o755); err != nil {
		t.Fatalf("mkdir public/wasm: %v", err)
	}
	if err := os.WriteFile("public/wasm/app.wasm.gz", []byte("x"), 0o644); err != nil {
		t.Fatalf("write wasm: %v", err)
	}
	if err := os.WriteFile("public/wasm_exec.js", []byte("x"), 0o644); err != nil {
		t.Fatalf("write wasm_exec: %v", err)
	}

	if err := wasmCleanCmd.RunE(&cobra.Command{}, nil); err != nil {
		t.Fatalf("wasm clean RunE error: %v", err)
	}
	if _, err := os.Stat("public/wasm"); !os.IsNotExist(err) {
		t.Error("expected public/wasm removed")
	}
	if _, err := os.Stat("public/wasm_exec.js"); !os.IsNotExist(err) {
		t.Error("expected public/wasm_exec.js removed")
	}
}

func TestWasmCleanCommandTolerantWhenAbsent(t *testing.T) {
	chdirTemp(t)
	// Nothing to clean: must succeed (os.IsNotExist tolerated).
	if err := wasmCleanCmd.RunE(&cobra.Command{}, nil); err != nil {
		t.Fatalf("wasm clean RunE on empty dir error: %v", err)
	}
}

func TestWasmCommandsRegistered(t *testing.T) {
	// wasmCmd must own install/clean/version subcommands.
	want := map[string]bool{"install": false, "clean": false, "version": false}
	for _, c := range wasmCmd.Commands() {
		if _, ok := want[c.Name()]; ok {
			want[c.Name()] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("wasm subcommand %q not registered", name)
		}
	}
}
