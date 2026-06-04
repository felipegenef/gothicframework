package helpers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

type TailwindHelper struct {
	Runtime        string // runtime.GOOS
	Arch           string // runtime.GOARCH
	Version        string // default "v3.4.14"
	ConfigOverride string // from gothic-config.json tailwindBinary field
}

// downloadBaseURL is the base URL for Tailwind standalone CLI release assets.
// It is a package var (not a const) so tests can point it at an httptest server
// to exercise the cache-miss download path without hitting GitHub.
var downloadBaseURL = "https://github.com/tailwindlabs/tailwindcss/releases/download"

func NewTailwindHelper(goos, goarch string) TailwindHelper {
	return TailwindHelper{
		Runtime: goos,
		Arch:    goarch,
		Version: "v3.4.14",
	}
}

// EnsureBinary resolves the Tailwind binary path. Resolution order:
// 1. ConfigOverride if set (returns error if file doesn't exist)
// 2. Cached binary in OS cache directory
// 3. Download from GitHub releases
func (h *TailwindHelper) EnsureBinary() (string, error) {
	if h.ConfigOverride != "" {
		if _, err := os.Stat(h.ConfigOverride); err != nil {
			return "", fmt.Errorf("tailwind binary override not found at %q: %w", h.ConfigOverride, err)
		}
		return h.ConfigOverride, nil
	}

	name, err := h.binaryName()
	if err != nil {
		return "", err
	}

	dir, err := h.cacheDir()
	if err != nil {
		return "", err
	}

	cachedPath := filepath.Join(dir, name)
	if _, err := os.Stat(cachedPath); err == nil {
		return cachedPath, nil
	}

	url := fmt.Sprintf("%s/%s/%s", downloadBaseURL, h.Version, name)
	fmt.Printf("Downloading Tailwind CSS %s for %s/%s...\n", h.Version, h.Runtime, h.Arch)

	// Create cache directory with 0700 per XDG Base Directory Spec
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("failed to create cache directory %q: %w", dir, err)
	}

	if err := h.downloadBinary(url, cachedPath); err != nil {
		return "", err
	}

	fmt.Println("Tailwind CSS downloaded successfully.")
	return cachedPath, nil
}

// Build runs the Tailwind CSS build (one-shot, minified).
func (h *TailwindHelper) Build() error {
	bin, err := h.EnsureBinary()
	if err != nil {
		return fmt.Errorf("error resolving tailwind binary: %w", err)
	}

	if _, err := runner.Run(context.Background(), bin, "-i", "src/css/app.css", "-o", "public/styles.css", "--minify"); err != nil {
		return fmt.Errorf("error generating tailwind css: %w", err)
	}
	return nil
}

// WatchStart starts the Tailwind CSS watcher (non-blocking).
func (h *TailwindHelper) WatchStart() (*exec.Cmd, error) {
	bin, err := h.EnsureBinary()
	if err != nil {
		return nil, fmt.Errorf("error resolving tailwind binary: %w", err)
	}

	cmd := exec.Command(bin, "--watch=always", "-i", "src/css/app.css", "-o", "public/styles.css", "--minify")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start tailwind watch process: %w", err)
	}

	return cmd, nil
}

// binaryName maps GOOS/GOARCH to the official Tailwind standalone CLI asset name.
func (h *TailwindHelper) binaryName() (string, error) {
	key := h.Runtime + "/" + h.Arch
	names := map[string]string{
		"linux/amd64":   "tailwindcss-linux-x64",
		"linux/arm64":   "tailwindcss-linux-arm64",
		"darwin/amd64":  "tailwindcss-macos-x64",
		"darwin/arm64":  "tailwindcss-macos-arm64",
		"windows/amd64": "tailwindcss-windows-x64.exe",
	}

	name, ok := names[key]
	if !ok {
		supported := make([]string, 0, len(names))
		for k := range names {
			supported = append(supported, k)
		}
		return "", fmt.Errorf("unsupported platform %s/%s. Supported platforms: %v", h.Runtime, h.Arch, supported)
	}
	return name, nil
}

// cacheDir returns the OS-appropriate cache directory for Gothic CLI binaries.
// Respects the GOTHIC_CLI_CACHE_DIR environment variable if set, otherwise
// uses os.UserCacheDir() which follows platform conventions:
//   - Linux: $XDG_CACHE_HOME/gothic-cli/bin or ~/.cache/gothic-cli/bin
//   - macOS: ~/Library/Caches/gothic-cli/bin
//   - Windows: %LocalAppData%\gothic-cli\bin
func (h *TailwindHelper) cacheDir() (string, error) {
	if dir := os.Getenv("GOTHIC_CLI_CACHE_DIR"); dir != "" {
		return filepath.Join(dir, "bin"), nil
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("failed to determine user cache directory: %w", err)
	}
	return filepath.Join(base, "gothic-cli", "bin"), nil
}

// downloadBinary downloads a file from url to destPath using atomic write.
// Uses os.CreateTemp in the target directory to guarantee the temp file is on the
// same filesystem (required for atomic os.Rename). Calls Chmod(0755) and Sync()
// before rename so the binary is never visible with wrong permissions or incomplete data.
func (h *TailwindHelper) downloadBinary(url, destPath string) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to download tailwind binary from %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download tailwind binary: HTTP %d from %s", resp.StatusCode, url)
	}

	// Create temp file in the same directory as destPath to ensure same filesystem for atomic rename.
	// os.CreateTemp uses mode 0600 by default; we Chmod to 0755 before rename.
	tmpFile, err := os.CreateTemp(filepath.Dir(destPath), ".tailwindcss-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file in %q: %w", filepath.Dir(destPath), err)
	}
	// Clean up the temp file on any error path
	tmpPath := tmpFile.Name()
	success := false
	defer func() {
		if !success {
			os.Remove(tmpPath)
		}
	}()

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write tailwind binary: %w", err)
	}

	// Set executable permissions before rename so the file is never visible with wrong perms
	if err := tmpFile.Chmod(0755); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to set permissions on tailwind binary: %w", err)
	}

	// Flush to disk to prevent 0-byte file on crash after rename
	if err := tmpFile.Sync(); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to sync tailwind binary to disk: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	// Atomic rename (same filesystem guarantees atomicity on Linux/macOS)
	if err := os.Rename(tmpPath, destPath); err != nil {
		return fmt.Errorf("failed to finalize tailwind binary: %w", err)
	}

	success = true
	return nil
}

// DefaultTailwindHelper creates a TailwindHelper using the current runtime's OS and architecture.
func DefaultTailwindHelper() TailwindHelper {
	return NewTailwindHelper(runtime.GOOS, runtime.GOARCH)
}
