package helpers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/mod/modfile"
)

func TestReadUserModulePath_Fixture(t *testing.T) {
	root := filepath.Join("testdata", "module_link")
	modulePath, goVersion, err := ReadUserModulePath(root)
	if err != nil {
		t.Fatalf("ReadUserModulePath: %v", err)
	}
	if modulePath != "example.com/fixture/project" {
		t.Errorf("module path = %q, want example.com/fixture/project", modulePath)
	}
	if goVersion != "1.22" {
		t.Errorf("go version = %q, want 1.22", goVersion)
	}
}

func TestReadUserModulePath_Missing(t *testing.T) {
	_, _, err := ReadUserModulePath(filepath.Join("testdata", "nonexistent"))
	if err == nil {
		t.Fatal("expected error for missing go.mod, got nil")
	}
}

func TestWriteBridgeGoMod_RoundTrip(t *testing.T) {
	tempDir := t.TempDir()
	userRoot := t.TempDir()

	const modulePath = "github.com/example/userproj"
	const goVersion = "1.22"
	if err := WriteBridgeGoMod(tempDir, modulePath, userRoot, goVersion); err != nil {
		t.Fatalf("WriteBridgeGoMod: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tempDir, "go.mod"))
	if err != nil {
		t.Fatalf("read generated go.mod: %v", err)
	}
	f, err := modfile.Parse("go.mod", data, nil)
	if err != nil {
		t.Fatalf("parse generated go.mod: %v\n---\n%s", err, data)
	}

	if f.Module == nil || f.Module.Mod.Path != "wasm-runtime" {
		t.Errorf("module = %+v, want wasm-runtime", f.Module)
	}
	if f.Go == nil || f.Go.Version != goVersion {
		t.Errorf("go version = %+v, want %s", f.Go, goVersion)
	}
	if f.Toolchain != nil {
		t.Errorf("toolchain directive present: %+v (should be absent)", f.Toolchain)
	}

	// Require
	gotRequire := false
	for _, r := range f.Require {
		if r.Mod.Path == modulePath {
			gotRequire = true
			break
		}
	}
	if !gotRequire {
		t.Errorf("require for %s not found; got %+v", modulePath, f.Require)
	}

	// Replace
	absUserRoot, _ := filepath.Abs(userRoot)
	gotReplace := false
	for _, r := range f.Replace {
		if r.Old.Path == modulePath && r.New.Path == absUserRoot {
			gotReplace = true
			break
		}
	}
	if !gotReplace {
		t.Errorf("replace %s => %s not found; got %+v", modulePath, absUserRoot, f.Replace)
	}

	if strings.Contains(string(data), "toolchain ") {
		t.Errorf("generated go.mod contains toolchain directive:\n%s", data)
	}
}

func TestWriteBridgeGoMod_DefaultGoVersion(t *testing.T) {
	tempDir := t.TempDir()
	userRoot := t.TempDir()
	if err := WriteBridgeGoMod(tempDir, "github.com/example/userproj", userRoot, ""); err != nil {
		t.Fatalf("WriteBridgeGoMod: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(tempDir, "go.mod"))
	f, err := modfile.Parse("go.mod", data, nil)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if f.Go == nil || f.Go.Version == "" {
		t.Errorf("expected a default go version, got %+v", f.Go)
	}
}

func TestWriteBridgeGoMod_CopiesGoSum(t *testing.T) {
	tempDir := t.TempDir()
	userRoot := t.TempDir()
	sumContent := []byte("example.com/foo v1.0.0 h1:abc\n")
	if err := os.WriteFile(filepath.Join(userRoot, "go.sum"), sumContent, 0644); err != nil {
		t.Fatal(err)
	}
	if err := WriteBridgeGoMod(tempDir, "github.com/example/userproj", userRoot, "1.22"); err != nil {
		t.Fatalf("WriteBridgeGoMod: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(tempDir, "go.sum"))
	if err != nil {
		t.Fatalf("read copied go.sum: %v", err)
	}
	if string(got) != string(sumContent) {
		t.Errorf("go.sum content mismatch: got %q want %q", got, sumContent)
	}
}
