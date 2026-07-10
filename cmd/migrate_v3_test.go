package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMigrateV3FileExists(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "x")
	if err := os.WriteFile(f, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !fileExists(f) {
		t.Errorf("fileExists(%q) = false, want true", f)
	}
	if fileExists(filepath.Join(dir, "missing")) {
		t.Error("fileExists on a missing file = true, want false")
	}
	if fileExists(dir) {
		t.Error("fileExists on a directory = true, want false")
	}
}

func TestMigrateV3GoEnv(t *testing.T) {
	got := goEnv("GOPATH")
	if got == "" {
		t.Fatal("goEnv(GOPATH) = empty, want a path")
	}
	if got != strings.TrimSpace(got) {
		t.Errorf("goEnv(GOPATH) not trimmed: %q", got)
	}
}

// TestMigrateV3LocateGothicOnPath puts a fake `gothic` first on PATH and asserts
// locateGothic resolves it via exec.LookPath.
func TestMigrateV3LocateGothicOnPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("exec-bit / .exe resolution differs on Windows")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "gothic")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	got, err := locateGothic()
	if err != nil {
		t.Fatalf("locateGothic: %v", err)
	}
	if filepath.Base(got) != "gothic" {
		t.Errorf("locateGothic() = %q, want a path ending in 'gothic'", got)
	}
	if !fileExists(got) {
		t.Errorf("locateGothic() returned a non-existent path %q", got)
	}
}
