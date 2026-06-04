package helpers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDefaultTailwindHelper(t *testing.T) {
	h := DefaultTailwindHelper()
	if h.Runtime != runtime.GOOS {
		t.Errorf("Runtime = %q, want %q", h.Runtime, runtime.GOOS)
	}
	if h.Arch != runtime.GOARCH {
		t.Errorf("Arch = %q, want %q", h.Arch, runtime.GOARCH)
	}
	if h.Version != "v3.4.14" {
		t.Errorf("Version = %q, want v3.4.14", h.Version)
	}
}

func TestDownloadBinarySuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("binary-bytes"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	dest := filepath.Join(dir, "tailwindcss")

	h := NewTailwindHelper("linux", "amd64")
	if err := h.downloadBinary(srv.URL, dest); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "binary-bytes" {
		t.Errorf("got %q", string(got))
	}
	info, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0111 == 0 {
		t.Error("downloaded binary should be executable")
	}
}

func TestDownloadBinaryHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	dir := t.TempDir()
	dest := filepath.Join(dir, "tailwindcss")

	h := NewTailwindHelper("linux", "amd64")
	if err := h.downloadBinary(srv.URL, dest); err == nil {
		t.Fatal("expected error for HTTP 404")
	}
	// Temp file must be cleaned up.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Errorf("leftover temp file: %s", e.Name())
		}
	}
}

func TestDownloadBinaryUnreachable(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "tailwindcss")

	h := NewTailwindHelper("linux", "amd64")
	// Port 0 / closed connection -> http.Get returns an error.
	if err := h.downloadBinary("http://127.0.0.1:1/never", dest); err == nil {
		t.Fatal("expected error for unreachable host")
	}
}

func TestEnsureBinaryCacheMissDownloads(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("fake-tailwind-binary"))
	}))
	defer srv.Close()

	// Redirect the download base URL at the test server and the cache dir at a
	// fresh temp dir so EnsureBinary takes the cache-miss download path.
	origBase := downloadBaseURL
	downloadBaseURL = srv.URL
	defer func() { downloadBaseURL = origBase }()

	cacheRoot := t.TempDir()
	t.Setenv("GOTHIC_CLI_CACHE_DIR", cacheRoot)

	h := NewTailwindHelper("linux", "amd64")
	path, err := h.EnsureBinary()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The constructed URL must encode version + platform asset name.
	wantPath := "/v3.4.14/tailwindcss-linux-x64"
	if gotPath != wantPath {
		t.Errorf("download URL path = %q, want %q", gotPath, wantPath)
	}

	wantPath = filepath.Join(cacheRoot, "bin", "tailwindcss-linux-x64")
	if path != wantPath {
		t.Errorf("returned path = %q, want %q", path, wantPath)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("binary not present at cache path: %v", err)
	}
	if info.Mode().Perm()&0111 == 0 {
		t.Error("cached binary should be executable")
	}
	got, _ := os.ReadFile(path)
	if string(got) != "fake-tailwind-binary" {
		t.Errorf("cached binary content = %q", string(got))
	}
}

func TestEnsureBinaryUnsupportedPlatform(t *testing.T) {
	h := NewTailwindHelper("plan9", "mips")
	if _, err := h.EnsureBinary(); err == nil {
		t.Fatal("expected error for unsupported platform")
	}
}
