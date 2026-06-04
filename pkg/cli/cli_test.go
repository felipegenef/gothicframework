package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// fakeRunner records the commands InitializeModule issues and returns a
// canned result, so tests never invoke the real Go toolchain.
type fakeRunner struct {
	output []byte
	err    error
	// failOn lets a test fail a specific subcommand (e.g. "get", "tidy").
	failOn string

	calls [][]string
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	call := append([]string{name}, args...)
	f.calls = append(f.calls, call)
	if f.failOn != "" && len(args) > 0 && args[0] == f.failOn {
		return f.output, errors.New("boom")
	}
	return f.output, f.err
}

// withFakeRunner swaps the package-level cliRunner for the duration of t.
func withFakeRunner(t *testing.T, f *fakeRunner) {
	t.Helper()
	orig := cliRunner
	cliRunner = f
	t.Cleanup(func() { cliRunner = orig })
}

func TestNewCli(t *testing.T) {
	cli := NewCli()
	if cli.Runtime != runtime.GOOS {
		t.Errorf("Runtime = %q, want %q", cli.Runtime, runtime.GOOS)
	}
	if cli.Logger == nil {
		t.Error("Logger is nil")
	}
	if cli.config != nil {
		t.Error("config should be nil before GetConfig")
	}
	if cli.appID != nil {
		t.Error("appID should be nil before GetAppId")
	}
}

func TestGetAppId(t *testing.T) {
	t.Run("reads file and caches", func(t *testing.T) {
		dir := t.TempDir()
		chdir(t, dir)
		if err := os.MkdirAll(".gothicCli", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(".gothicCli", "app-id.txt"), []byte("abc123"), 0o644); err != nil {
			t.Fatal(err)
		}

		cli := GothicCli{}
		got, err := cli.GetAppId()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "abc123" {
			t.Errorf("GetAppId() = %q, want %q", got, "abc123")
		}
		// Mutate file to prove the cached value is returned, not a re-read.
		if err := os.WriteFile(filepath.Join(".gothicCli", "app-id.txt"), []byte("changed"), 0o644); err != nil {
			t.Fatal(err)
		}
		got2, err := cli.GetAppId()
		if err != nil {
			t.Fatal(err)
		}
		if got2 != "abc123" {
			t.Errorf("cached GetAppId() = %q, want %q", got2, "abc123")
		}
	})

	t.Run("missing file errors", func(t *testing.T) {
		dir := t.TempDir()
		chdir(t, dir)
		cli := GothicCli{}
		if _, err := cli.GetAppId(); err == nil {
			t.Error("expected error for missing app-id.txt")
		}
	})
}

func TestGetConfig(t *testing.T) {
	t.Run("decodes and applies overrides", func(t *testing.T) {
		dir := t.TempDir()
		chdir(t, dir)
		cfg := `{
			"projectName": "demo",
			"goModuleName": "example.com/demo",
			"tailwindBinary": "/bin/tw",
			"wasmBinary": "/bin/wasm",
			"wasmTinyGoVersion": "0.31.0",
			"optimizeImages": {"lowResolutionRate": 10}
		}`
		if err := os.WriteFile("gothic-config.json", []byte(cfg), 0o644); err != nil {
			t.Fatal(err)
		}

		cli := NewCli()
		got, err := cli.GetConfig()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.ProjectName != "demo" {
			t.Errorf("ProjectName = %q", got.ProjectName)
		}
		if cli.Tailwind.ConfigOverride != "/bin/tw" {
			t.Errorf("Tailwind.ConfigOverride = %q", cli.Tailwind.ConfigOverride)
		}
		if cli.Wasm.Version != "0.31.0" {
			t.Errorf("Wasm.Version = %q", cli.Wasm.Version)
		}
		if cli.Wasm.ConfigOverride != "/bin/wasm" {
			t.Errorf("Wasm.ConfigOverride = %q", cli.Wasm.ConfigOverride)
		}

		// Second call returns the cached config without reopening the file.
		if err := os.Remove("gothic-config.json"); err != nil {
			t.Fatal(err)
		}
		got2, err := cli.GetConfig()
		if err != nil {
			t.Fatalf("cached GetConfig errored: %v", err)
		}
		if got2.ProjectName != "demo" {
			t.Errorf("cached ProjectName = %q", got2.ProjectName)
		}
	})

	t.Run("minimal config leaves overrides untouched", func(t *testing.T) {
		dir := t.TempDir()
		chdir(t, dir)
		if err := os.WriteFile("gothic-config.json", []byte(`{"projectName":"x","goModuleName":"y"}`), 0o644); err != nil {
			t.Fatal(err)
		}
		cli := NewCli()
		if _, err := cli.GetConfig(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cli.Tailwind.ConfigOverride != "" {
			t.Errorf("Tailwind.ConfigOverride should stay empty, got %q", cli.Tailwind.ConfigOverride)
		}
	})

	t.Run("missing file errors", func(t *testing.T) {
		dir := t.TempDir()
		chdir(t, dir)
		cli := GothicCli{}
		if _, err := cli.GetConfig(); err == nil {
			t.Error("expected error for missing gothic-config.json")
		}
	})

	t.Run("invalid json errors", func(t *testing.T) {
		dir := t.TempDir()
		chdir(t, dir)
		if err := os.WriteFile("gothic-config.json", []byte("{not json"), 0o644); err != nil {
			t.Fatal(err)
		}
		cli := GothicCli{}
		if _, err := cli.GetConfig(); err == nil {
			t.Error("expected decode error for invalid json")
		}
	})
}

func TestInitializeModule(t *testing.T) {
	t.Run("runs init, get and tidy with correct args", func(t *testing.T) {
		f := &fakeRunner{}
		withFakeRunner(t, f)

		cli := GothicCli{}
		if err := cli.InitializeModule("example.com/demo", "v2.3.4"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(f.calls) != 3 {
			t.Fatalf("expected 3 commands, got %d: %v", len(f.calls), f.calls)
		}
		wantInit := []string{"go", "mod", "init", "example.com/demo"}
		if !equalArgs(f.calls[0], wantInit) {
			t.Errorf("init cmd = %v, want %v", f.calls[0], wantInit)
		}
		wantGet := []string{"go", "get", "github.com/felipegenef/gothicframework/v2@v2.3.4"}
		if !equalArgs(f.calls[1], wantGet) {
			t.Errorf("get cmd = %v, want %v", f.calls[1], wantGet)
		}
		wantTidy := []string{"go", "mod", "tidy"}
		if !equalArgs(f.calls[2], wantTidy) {
			t.Errorf("tidy cmd = %v, want %v", f.calls[2], wantTidy)
		}
	})

	t.Run("skips pin when version empty", func(t *testing.T) {
		f := &fakeRunner{}
		withFakeRunner(t, f)
		cli := GothicCli{}
		if err := cli.InitializeModule("example.com/demo", ""); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(f.calls) != 2 {
			t.Fatalf("expected 2 commands (no pin), got %d: %v", len(f.calls), f.calls)
		}
		if f.calls[1][1] != "mod" || f.calls[1][2] != "tidy" {
			t.Errorf("second cmd should be go mod tidy, got %v", f.calls[1])
		}
	})

	t.Run("init failure surfaces error", func(t *testing.T) {
		f := &fakeRunner{failOn: "mod"}
		withFakeRunner(t, f)
		cli := GothicCli{}
		err := cli.InitializeModule("example.com/demo", "v2.0.0")
		if err == nil {
			t.Fatal("expected error from go mod init")
		}
	})

	t.Run("pin failure surfaces error", func(t *testing.T) {
		f := &fakeRunner{failOn: "get"}
		withFakeRunner(t, f)
		cli := GothicCli{}
		err := cli.InitializeModule("example.com/demo", "v2.0.0")
		if err == nil {
			t.Fatal("expected error from go get")
		}
	})

	t.Run("tidy failure surfaces error", func(t *testing.T) {
		// fail only the tidy step: init is "mod init", tidy is "mod tidy".
		// failOn "mod" would catch init too, so use a runner that fails the
		// third call specifically.
		f := &thirdCallFailRunner{}
		orig := cliRunner
		cliRunner = f
		t.Cleanup(func() { cliRunner = orig })

		cli := GothicCli{}
		if err := cli.InitializeModule("example.com/demo", "v2.0.0"); err == nil {
			t.Fatal("expected error from go mod tidy")
		}
	})
}

// thirdCallFailRunner fails on its third Run call (the go mod tidy step).
type thirdCallFailRunner struct{ n int }

func (r *thirdCallFailRunner) Run(_ context.Context, _ string, _ ...string) ([]byte, error) {
	r.n++
	if r.n == 3 {
		return nil, errors.New("tidy boom")
	}
	return nil, nil
}

func equalArgs(a, b []string) bool {
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

// chdir changes into dir and restores the original working directory on cleanup.
func chdir(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })
}
