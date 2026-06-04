package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// chdirTemp creates a fresh temp directory, chdir's into it for the duration of
// the test, and restores the original working dir on cleanup. Returns the temp
// dir path.
func chdirTemp(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })
	return dir
}

// writeConfig writes a gothic-config.json into the current working dir.
func writeConfig(t *testing.T, contents string) {
	t.Helper()
	if err := os.WriteFile("gothic-config.json", []byte(contents), 0o644); err != nil {
		t.Fatalf("write gothic-config.json: %v", err)
	}
}

// writeGoMod writes a minimal go.mod so astx.NewLoader / packages.Load can run
// against the temp project. With no .go files, ScanPages returns zero pages
// without error, letting wasm-driving code paths proceed past the scan stage.
func writeGoMod(t *testing.T, module string) {
	t.Helper()
	contents := "module " + module + "\n\ngo 1.23\n"
	if err := os.WriteFile("go.mod", []byte(contents), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
}

// writeFakeTailwind writes an executable no-op script that any TailwindHelper
// pointed at it (via the tailwindBinary config override) will treat as the
// Tailwind CLI. It simply exits 0, so Build()/EnsureBinary() succeed without a
// real download or compile. Skips the test on Windows where shell scripts are
// not executable as-is. Returns the absolute binary path.
func writeFakeTailwind(t *testing.T, ok bool) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake shell binary not supported on windows")
	}
	exit := "0"
	if !ok {
		exit = "1"
	}
	path := filepath.Join(t.TempDir(), "faketailwind")
	script := "#!/bin/sh\nexit " + exit + "\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake tailwind: %v", err)
	}
	return path
}
