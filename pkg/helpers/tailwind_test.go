package helpers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBinaryName(t *testing.T) {
	tests := []struct {
		name    string
		goos    string
		goarch  string
		want    string
		wantErr bool
	}{
		{"linux amd64", "linux", "amd64", "tailwindcss-linux-x64", false},
		{"linux arm64", "linux", "arm64", "tailwindcss-linux-arm64", false},
		{"darwin amd64", "darwin", "amd64", "tailwindcss-macos-x64", false},
		{"darwin arm64", "darwin", "arm64", "tailwindcss-macos-arm64", false},
		{"windows amd64", "windows", "amd64", "tailwindcss-windows-x64.exe", false},
		{"unsupported freebsd", "freebsd", "amd64", "", true},
		{"unsupported linux 386", "linux", "386", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewTailwindHelper(tt.goos, tt.goarch)
			got, err := h.binaryName()
			if tt.wantErr {
				if err == nil {
					t.Errorf("binaryName() expected error, got nil")
				}
				if !strings.Contains(err.Error(), "unsupported platform") {
					t.Errorf("binaryName() error = %v, want 'unsupported platform'", err)
				}
				return
			}
			if err != nil {
				t.Errorf("binaryName() unexpected error: %v", err)
				return
			}
			if got != tt.want {
				t.Errorf("binaryName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCacheDir(t *testing.T) {
	// Unset env var to test default behavior
	t.Setenv("GOTHIC_CLI_CACHE_DIR", "")

	h := NewTailwindHelper("linux", "amd64")
	dir, err := h.cacheDir()
	if err != nil {
		t.Fatalf("cacheDir() unexpected error: %v", err)
	}
	if !strings.HasSuffix(dir, filepath.Join("gothic-cli", "bin")) {
		t.Errorf("cacheDir() = %q, want suffix %q", dir, filepath.Join("gothic-cli", "bin"))
	}
}

func TestCacheDirEnvOverride(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("GOTHIC_CLI_CACHE_DIR", tmpDir)

	h := NewTailwindHelper("linux", "amd64")
	dir, err := h.cacheDir()
	if err != nil {
		t.Fatalf("cacheDir() unexpected error: %v", err)
	}
	want := filepath.Join(tmpDir, "bin")
	if dir != want {
		t.Errorf("cacheDir() = %q, want %q", dir, want)
	}
}

func TestEnsureBinaryWithOverride(t *testing.T) {
	// Create a temp file to act as the override binary
	tmpFile, err := os.CreateTemp("", "tailwindcss-override-*")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	h := NewTailwindHelper("linux", "amd64")
	h.ConfigOverride = tmpFile.Name()

	got, err := h.EnsureBinary()
	if err != nil {
		t.Fatalf("EnsureBinary() unexpected error: %v", err)
	}
	if got != tmpFile.Name() {
		t.Errorf("EnsureBinary() = %q, want %q", got, tmpFile.Name())
	}
}

func TestEnsureBinaryWithOverrideNotFound(t *testing.T) {
	h := NewTailwindHelper("linux", "amd64")
	h.ConfigOverride = "/nonexistent/path/tailwindcss"

	_, err := h.EnsureBinary()
	if err == nil {
		t.Fatal("EnsureBinary() expected error for nonexistent override, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("EnsureBinary() error = %v, want 'not found'", err)
	}
}

func TestEnsureBinaryWithCachedFile(t *testing.T) {
	// Create a temp directory to act as cache and point GOTHIC_CLI_CACHE_DIR to it
	tmpDir := t.TempDir()
	t.Setenv("GOTHIC_CLI_CACHE_DIR", tmpDir)

	binDir := filepath.Join(tmpDir, "bin")
	if err := os.MkdirAll(binDir, 0700); err != nil {
		t.Fatalf("failed to create bin dir: %v", err)
	}

	// Create a fake cached binary with the expected name
	fakeBinary := filepath.Join(binDir, "tailwindcss-linux-x64")
	if err := os.WriteFile(fakeBinary, []byte("fake"), 0755); err != nil {
		t.Fatalf("failed to write fake binary: %v", err)
	}

	h := NewTailwindHelper("linux", "amd64")

	got, err := h.EnsureBinary()
	if err != nil {
		t.Fatalf("EnsureBinary() unexpected error: %v", err)
	}
	if got != fakeBinary {
		t.Errorf("EnsureBinary() = %q, want %q", got, fakeBinary)
	}
}

func TestNewTailwindHelperDefaults(t *testing.T) {
	h := NewTailwindHelper("darwin", "arm64")
	if h.Runtime != "darwin" {
		t.Errorf("Runtime = %q, want %q", h.Runtime, "darwin")
	}
	if h.Arch != "arm64" {
		t.Errorf("Arch = %q, want %q", h.Arch, "arm64")
	}
	if h.Version != "v3.4.14" {
		t.Errorf("Version = %q, want %q", h.Version, "v3.4.14")
	}
	if h.ConfigOverride != "" {
		t.Errorf("ConfigOverride = %q, want empty", h.ConfigOverride)
	}
}
