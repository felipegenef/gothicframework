package helpers

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// touch creates an empty executable file at path, making parent dirs.
func touch(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestEnsureBinary_ConfigOverride(t *testing.T) {
	h := linuxAmd64Helper()

	// Valid override path → no error, no download.
	override := filepath.Join(t.TempDir(), "my-tinygo")
	touch(t, override)
	h.ConfigOverride = override
	if err := h.EnsureBinary(); err != nil {
		t.Errorf("EnsureBinary with valid override: %v", err)
	}

	// Missing override path → error.
	h.ConfigOverride = filepath.Join(t.TempDir(), "does-not-exist")
	if err := h.EnsureBinary(); err == nil {
		t.Error("EnsureBinary with missing override should error")
	}
}

func TestEnsureBinary_AlreadyReady(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("GOTHIC_CLI_CACHE_DIR", tmp)
	h := linuxAmd64Helper()

	// Pre-create both the tinygo binary and the managed binaryen binary so both
	// readiness checks pass and EnsureBinary returns without downloading.
	touch(t, h.TinyGoBinary())
	touch(t, h.BinaryenBinary())

	if err := h.EnsureBinary(); err != nil {
		t.Errorf("EnsureBinary when ready: %v", err)
	}
}

func TestEnsureBinaryen_WasmOptInPath(t *testing.T) {
	// If wasm-opt is already on PATH, EnsureBinaryen is a no-op. Simulate by
	// placing a fake wasm-opt in a dir we prepend to PATH.
	binDir := t.TempDir()
	touch(t, filepath.Join(binDir, "wasm-opt"))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	if _, err := exec.LookPath("wasm-opt"); err != nil {
		t.Skip("fake wasm-opt not resolvable on this platform")
	}

	h := linuxAmd64Helper()
	if err := h.EnsureBinaryen(); err != nil {
		t.Errorf("EnsureBinaryen with wasm-opt in PATH: %v", err)
	}
}

func TestEnsureBinaryen_ManagedBinaryExists(t *testing.T) {
	// Ensure wasm-opt is NOT in PATH by pointing PATH at an empty dir.
	emptyDir := t.TempDir()
	t.Setenv("PATH", emptyDir)
	if _, err := exec.LookPath("wasm-opt"); err == nil {
		t.Skip("wasm-opt still resolvable; cannot isolate")
	}

	tmp := t.TempDir()
	t.Setenv("GOTHIC_CLI_CACHE_DIR", tmp)
	h := linuxAmd64Helper()
	// Pre-create the managed binaryen binary → early return before download.
	touch(t, h.BinaryenBinary())
	if err := h.EnsureBinaryen(); err != nil {
		t.Errorf("EnsureBinaryen with managed binary present: %v", err)
	}
}

func TestEnsureTinyGo_AlreadyInstalled(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("GOTHIC_CLI_CACHE_DIR", tmp)
	h := linuxAmd64Helper()
	touch(t, h.TinyGoBinary())
	if err := h.ensureTinyGo(); err != nil {
		t.Errorf("ensureTinyGo when installed: %v", err)
	}
}

func TestEnsureTinyGo_UnsupportedPlatform(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("GOTHIC_CLI_CACHE_DIR", tmp)
	h := DefaultWasmHelper()
	h.Runtime = "plan9"
	h.Arch = "mips"
	h.Version = "0.41.1"
	h.BinaryenVersion = "117"
	// Binary not present + unsupported platform → binaryName() error before any
	// network access.
	if err := h.ensureTinyGo(); err == nil {
		t.Error("ensureTinyGo on unsupported platform should error")
	}
}
