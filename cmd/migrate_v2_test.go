package cmd

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/mod/modfile"
)

// copyDir recursively copies src into dst.
func copyDir(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		b, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		return os.WriteFile(target, b, 0o644)
	})
	if err != nil {
		t.Fatalf("copyDir: %v", err)
	}
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	return string(b)
}

// equalGoMod compares two go.mod contents semantically (module + requires).
func equalGoMod(t *testing.T, gotPath, wantPath string) bool {
	t.Helper()
	gb, err := os.ReadFile(gotPath)
	if err != nil {
		t.Fatalf("read got: %v", err)
	}
	wb, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("read want: %v", err)
	}
	g, err := modfile.Parse(gotPath, gb, nil)
	if err != nil {
		t.Fatalf("parse got: %v", err)
	}
	w, err := modfile.Parse(wantPath, wb, nil)
	if err != nil {
		t.Fatalf("parse want: %v", err)
	}
	if g.Module.Mod.Path != w.Module.Mod.Path {
		return false
	}
	if len(g.Require) != len(w.Require) {
		return false
	}
	for i := range g.Require {
		if g.Require[i].Mod.Path != w.Require[i].Mod.Path ||
			g.Require[i].Mod.Version != w.Require[i].Mod.Version {
			return false
		}
	}
	return true
}

func setupTempProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	copyDir(t, filepath.Join("testdata", "migrate_v2", "before"), dir)
	return dir
}

func TestMigrateV2_Migration(t *testing.T) {
	skipTidyForTest = true
	defer func() { skipTidyForTest = false }()

	dir := setupTempProject(t)
	var out bytes.Buffer
	if err := runMigrateV2(dir, false, &out); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	afterDir := filepath.Join("testdata", "migrate_v2", "after")

	if !equalGoMod(t, filepath.Join(dir, "go.mod"), filepath.Join(afterDir, "go.mod")) {
		t.Errorf("go.mod mismatch.\ngot:\n%s\nwant:\n%s",
			readFile(t, filepath.Join(dir, "go.mod")),
			readFile(t, filepath.Join(afterDir, "go.mod")))
	}
	for _, name := range []string{"main.go", "page.templ"} {
		got := readFile(t, filepath.Join(dir, name))
		want := readFile(t, filepath.Join(afterDir, name))
		if got != want {
			t.Errorf("%s mismatch.\ngot:\n%s\nwant:\n%s", name, got, want)
		}
	}
	if !strings.Contains(out.String(), "files updated") {
		t.Errorf("expected summary line, got: %q", out.String())
	}
}

func TestMigrateV2_Idempotent(t *testing.T) {
	skipTidyForTest = true
	defer func() { skipTidyForTest = false }()

	dir := setupTempProject(t)
	var out1 bytes.Buffer
	if err := runMigrateV2(dir, false, &out1); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	var out2 bytes.Buffer
	if err := runMigrateV2(dir, false, &out2); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	// Second run should report nothing to migrate OR 0 files updated.
	s := out2.String()
	if !strings.Contains(s, "nothing to migrate") && !strings.Contains(s, "0 files updated") {
		t.Errorf("second run should be a no-op, got: %q", s)
	}
}

func TestMigrateV2_DryRun(t *testing.T) {
	skipTidyForTest = true
	defer func() { skipTidyForTest = false }()

	dir := setupTempProject(t)
	beforeDir := filepath.Join("testdata", "migrate_v2", "before")

	var out bytes.Buffer
	if err := runMigrateV2(dir, true, &out); err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	for _, name := range []string{"go.mod", "main.go", "page.templ"} {
		got := readFile(t, filepath.Join(dir, name))
		want := readFile(t, filepath.Join(beforeDir, name))
		if got != want {
			t.Errorf("dry-run mutated %s.\ngot:\n%s\nwant:\n%s", name, got, want)
		}
	}
}

func TestMigrateV2_NothingToMigrate(t *testing.T) {
	skipTidyForTest = true
	defer func() { skipTidyForTest = false }()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"),
		[]byte("module example.com/other\n\ngo 1.23\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"),
		[]byte("package main\nfunc main(){}\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	var out bytes.Buffer
	if err := runMigrateV2(dir, false, &out); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if !strings.Contains(out.String(), "nothing to migrate") {
		t.Errorf("expected 'nothing to migrate', got: %q", out.String())
	}
}
