package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestRunMigrateV2MissingGoMod(t *testing.T) {
	dir := t.TempDir()
	// No go.mod: runMigrateV2 must fail reading it.
	var out bytes.Buffer
	if err := runMigrateV2(dir, false, &out); err == nil {
		t.Fatal("expected error when go.mod is missing")
	}
}

func TestReferencesOldPath(t *testing.T) {
	old := []byte(`import "` + oldModulePath + `/pkg/cli"`)
	if !referencesOldPath(old) {
		t.Error("expected old path to be detected")
	}
	v2 := []byte(`import "` + newModulePath + `/pkg/cli"`)
	if referencesOldPath(v2) {
		t.Error("v2 path must not be flagged as old")
	}
}

func TestAnyFileReferencesOldPathFalse(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"),
		[]byte("package main\nfunc main(){}\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if anyFileReferencesOldPath(dir) {
		t.Error("expected no old-path references")
	}
}

func TestAnyFileReferencesOldPathTrue(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"),
		[]byte(`package main`+"\n"+`import _ "`+oldModulePath+`/pkg/cli"`+"\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if !anyFileReferencesOldPath(dir) {
		t.Error("expected old-path reference to be found")
	}
}

func TestRewriteFileNoChange(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "clean.go")
	content := "package main\nfunc main(){}\n"
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	changed, n, err := rewriteFile(p, false)
	if err != nil {
		t.Fatalf("rewriteFile: %v", err)
	}
	if changed || n != 0 {
		t.Errorf("expected no change, got changed=%v n=%d", changed, n)
	}
}

func TestRewriteFileMissing(t *testing.T) {
	if _, _, err := rewriteFile(filepath.Join(t.TempDir(), "nope.go"), false); err == nil {
		t.Fatal("expected error rewriting missing file")
	}
}

func TestRewriteGoModParseError(t *testing.T) {
	p := filepath.Join(t.TempDir(), "go.mod")
	bad := []byte("this is not a valid go.mod !!!\nrequire (((")
	if _, err := rewriteGoMod(p, bad, true); err == nil {
		t.Fatal("expected parse error for malformed go.mod")
	}
}

func TestRewriteGoModNoChange(t *testing.T) {
	p := filepath.Join(t.TempDir(), "go.mod")
	content := []byte("module example.com/app\n\ngo 1.23\n")
	changed, err := rewriteGoMod(p, content, true)
	if err != nil {
		t.Fatalf("rewriteGoMod: %v", err)
	}
	if changed {
		t.Error("expected no change when old path absent")
	}
}

func TestRewriteGoModReplaceDirective(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "go.mod")
	content := []byte("module example.com/app\n\ngo 1.23\n\nrequire " + oldModulePath + " v1.4.0\n\nreplace " + oldModulePath + " => ../local\n")
	if err := os.WriteFile(p, content, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	changed, err := rewriteGoMod(p, content, false)
	if err != nil {
		t.Fatalf("rewriteGoMod: %v", err)
	}
	if !changed {
		t.Fatal("expected change for old require+replace")
	}
	got := readFile(t, p)
	if !bytesContains(got, newModulePath) {
		t.Errorf("expected new module path in output:\n%s", got)
	}
}

// TestRewriteGoModSeedsCurrentVersionForV1 verifies that a v1.x require is
// rewritten to the new /v2 module path seeded with CURRENT_VERSION (since a
// v1.x version is not import-compatible with a /v2 module path).
func TestRewriteGoModSeedsCurrentVersionForV1(t *testing.T) {
	p := filepath.Join(t.TempDir(), "go.mod")
	content := []byte("module example.com/app\n\ngo 1.23\n\nrequire " + oldModulePath + " v1.5.0\n")
	if err := os.WriteFile(p, content, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	changed, err := rewriteGoMod(p, content, false)
	if err != nil {
		t.Fatalf("rewriteGoMod: %v", err)
	}
	if !changed {
		t.Fatal("expected change for v1 require")
	}
	got := readFile(t, p)
	if !bytesContains(got, newModulePath+" "+CURRENT_VERSION) {
		t.Errorf("expected %q seeded with CURRENT_VERSION %q in output:\n%s",
			newModulePath, CURRENT_VERSION, got)
	}
}

// TestRewriteGoModPreservesExistingV2Version verifies that a go.mod already on
// the /v2 module path at a specific v2.x version is left untouched: the
// existing version is preserved and NOT overwritten with CURRENT_VERSION.
//
// Note: the version-seeding branch in rewriteGoMod guards against ever placing
// a v2.x version on the old (non-/v2) path, but that exact state is unreachable
// through a parseable go.mod — modfile rejects "require <old-path> v2.x" under
// SemVer import compatibility. The observable, real-world v2-preservation case
// is therefore a require already on the new /v2 path, which rewriteGoMod must
// leave alone.
func TestRewriteGoModPreservesExistingV2Version(t *testing.T) {
	p := filepath.Join(t.TempDir(), "go.mod")
	content := []byte("module example.com/app\n\ngo 1.23\n\nrequire " + newModulePath + " v2.3.0\n")
	if err := os.WriteFile(p, content, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	changed, err := rewriteGoMod(p, content, false)
	if err != nil {
		t.Fatalf("rewriteGoMod: %v", err)
	}
	if changed {
		t.Error("expected no change when require is already on /v2 path")
	}
	got := readFile(t, p)
	if !bytesContains(got, newModulePath+" v2.3.0") {
		t.Errorf("expected existing v2.3.0 preserved in output:\n%s", got)
	}
	if CURRENT_VERSION != "v2.3.0" && bytesContains(got, CURRENT_VERSION) {
		t.Errorf("existing v2 version must not be overwritten with CURRENT_VERSION %q:\n%s",
			CURRENT_VERSION, got)
	}
}

func bytesContains(haystack, needle string) bool {
	return bytes.Contains([]byte(haystack), []byte(needle))
}

func TestRewriteFileDryRunDoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "old.go")
	content := `package main` + "\n" + `import _ "` + oldModulePath + `/pkg/cli"` + "\n"
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	changed, n, err := rewriteFile(p, true)
	if err != nil {
		t.Fatalf("rewriteFile: %v", err)
	}
	if !changed || n == 0 {
		t.Errorf("expected change detected, got changed=%v n=%d", changed, n)
	}
	// Dry run must leave the file untouched.
	if got := readFile(t, p); got != content {
		t.Errorf("dry-run mutated file: %q", got)
	}
}
