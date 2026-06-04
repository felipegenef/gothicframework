package helpers

import (
	"embed"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

//go:embed testdata/greet.tmpl testdata/raw.txt
var testFixtures embed.FS

func TestUpdateFromTemplate(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "in.tmpl")
	if err := os.WriteFile(src, []byte("Project: {{.ProjectName}}"), 0644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out.txt")

	h := NewTemplateHelper()
	info := InitCmdTemplateInfo{ProjectName: "demo"}
	if err := h.UpdateFromTemplate(src, out, info); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "Project: demo" {
		t.Errorf("got %q, want %q", string(got), "Project: demo")
	}
}

func TestUpdateFromTemplateReadError(t *testing.T) {
	h := NewTemplateHelper()
	err := h.UpdateFromTemplate("/nonexistent/file.tmpl", filepath.Join(t.TempDir(), "o.txt"), nil)
	if err == nil {
		t.Fatal("expected error for missing source")
	}
}

func TestUpdateFromTemplateCreateError(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "in.tmpl")
	if err := os.WriteFile(src, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	// Output path inside a nonexistent directory -> os.Create fails.
	out := filepath.Join(dir, "missing", "o.txt")
	h := NewTemplateHelper()
	if err := h.UpdateFromTemplate(src, out, nil); err == nil {
		t.Fatal("expected error creating output in missing dir")
	}
}

func TestUpdateFromTemplateFS(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "nested", "out.txt")

	h := NewTemplateHelper()
	info := InitCmdTemplateInfo{ProjectName: "fsdemo"}
	if err := h.UpdateFromTemplateFS(testFixtures, "testdata/greet.tmpl", out, info); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "Hello fsdemo!" {
		t.Errorf("got %q", string(got))
	}
}

func TestUpdateFromTemplateFSMissing(t *testing.T) {
	h := NewTemplateHelper()
	err := h.UpdateFromTemplateFS(testFixtures, "testdata/nope.tmpl", filepath.Join(t.TempDir(), "o.txt"), nil)
	if err == nil {
		t.Fatal("expected error for missing embedded template")
	}
}

func TestCreateFromTemplate(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "sub", "out.txt")

	h := NewTemplateHelper()
	info := InitCmdTemplateInfo{ProjectName: "created"}
	if err := h.CreateFromTemplate(testFixtures, "testdata/greet.tmpl", out, info); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "Hello created!" {
		t.Errorf("got %q", string(got))
	}
}

func TestCreateFromTemplateMissing(t *testing.T) {
	h := NewTemplateHelper()
	err := h.CreateFromTemplate(testFixtures, "testdata/nope.tmpl", filepath.Join(t.TempDir(), "o.txt"), nil)
	if err == nil {
		t.Fatal("expected error for missing embedded template")
	}
}

func TestRenderToString(t *testing.T) {
	h := NewTemplateHelper()
	got, err := h.RenderToString(testFixtures, "testdata/greet.tmpl", InitCmdTemplateInfo{ProjectName: "str"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "Hello str!" {
		t.Errorf("got %q", got)
	}
}

func TestRenderToStringMissing(t *testing.T) {
	h := NewTemplateHelper()
	_, err := h.RenderToString(testFixtures, "testdata/nope.tmpl", nil)
	if err == nil {
		t.Fatal("expected error for missing template")
	}
}

func TestCopyFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	if err := os.WriteFile(src, []byte("payload"), 0644); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "dst.txt")

	h := NewTemplateHelper()
	if err := h.CopyFile(src, dst); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, _ := os.ReadFile(dst)
	if string(got) != "payload" {
		t.Errorf("got %q", string(got))
	}
}

func TestCopyFileMissingSource(t *testing.T) {
	h := NewTemplateHelper()
	if err := h.CopyFile("/nonexistent/src.txt", filepath.Join(t.TempDir(), "d.txt")); err == nil {
		t.Fatal("expected error for missing source")
	}
}

func TestDeleteFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "del.txt")
	if err := os.WriteFile(f, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	h := NewTemplateHelper()
	if err := h.DeleteFile(f); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(f); !os.IsNotExist(err) {
		t.Error("file should be deleted")
	}
	// Deleting again should error.
	if err := h.DeleteFile(f); err == nil {
		t.Error("expected error deleting missing file")
	}
}

func TestCopyFromFs(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "deep", "raw.txt")

	h := NewTemplateHelper()
	if err := h.CopyFromFs(testFixtures, "testdata/raw.txt", out); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "raw file content") {
		t.Errorf("got %q", string(got))
	}
}

func TestCopyFromFsMissing(t *testing.T) {
	h := NewTemplateHelper()
	if err := h.CopyFromFs(testFixtures, "testdata/nope.txt", filepath.Join(t.TempDir(), "o.txt")); err == nil {
		t.Fatal("expected error for missing embedded file")
	}
}
