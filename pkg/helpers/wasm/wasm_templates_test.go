package helpers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEnsureWasmTemplates_CreatesMissing covers the cold-start path: when no
// on-disk template exists at all, EnsureWasmTemplates writes the embedded
// copy into place (creating parent directories as needed).
func TestEnsureWasmTemplates_CreatesMissing(t *testing.T) {
	dir := t.TempDir()
	cwd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	h := DefaultWasmHelper()
	if err := h.EnsureWasmTemplates(); err != nil {
		t.Fatalf("EnsureWasmTemplates: %v", err)
	}

	pagePath := filepath.Join(dir, tmplWasmPageMain)
	got, err := os.ReadFile(pagePath)
	if err != nil {
		t.Fatalf("read written template: %v", err)
	}
	if !strings.Contains(string(got), "func main()") {
		t.Errorf("written page template is missing `func main()`; got: %s", got)
	}
	if !strings.Contains(string(got), "select {}") {
		t.Errorf("written page template is missing trailing `select {}`; got: %s", got)
	}

	tmplPath := filepath.Join(dir, tmplTopicManagerMain)
	if _, err := os.Stat(tmplPath); err != nil {
		t.Errorf("topic manager template not written: %v", err)
	}
}

// TestEnsureWasmTemplates_RewritesStale guards the template-refresh path:
// an existing project whose on-disk template predates the trailing `select {}`
// fix must be rewritten from the embedded copy so compiled WASMs no longer
// exit immediately after init.
func TestEnsureWasmTemplates_RewritesStale(t *testing.T) {
	dir := t.TempDir()
	cwd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	staleTemplate := "func main() {\n{{.Body}}}\n"
	stalePath := filepath.Join(dir, tmplWasmPageMain)
	if err := os.MkdirAll(filepath.Dir(stalePath), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(stalePath, []byte(staleTemplate), 0644); err != nil {
		t.Fatalf("seed stale template: %v", err)
	}

	h := DefaultWasmHelper()
	if err := h.EnsureWasmTemplates(); err != nil {
		t.Fatalf("EnsureWasmTemplates: %v", err)
	}

	got, err := os.ReadFile(stalePath)
	if err != nil {
		t.Fatalf("read template after refresh: %v", err)
	}
	if !strings.Contains(string(got), "select {}") {
		t.Errorf("stale template was not refreshed with `select {}`; got: %s", got)
	}
}

// TestEnsureWasmTemplates_IdempotentSecondCall asserts that a second call when
// the on-disk copy is already in sync does not rewrite the file (mtime stays
// stable). This keeps incremental builds free of spurious file-system churn.
func TestEnsureWasmTemplates_IdempotentSecondCall(t *testing.T) {
	dir := t.TempDir()
	cwd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	h := DefaultWasmHelper()
	if err := h.EnsureWasmTemplates(); err != nil {
		t.Fatalf("first EnsureWasmTemplates: %v", err)
	}
	pagePath := filepath.Join(dir, tmplWasmPageMain)
	info1, err := os.Stat(pagePath)
	if err != nil {
		t.Fatalf("stat first write: %v", err)
	}

	// Force a measurable mtime difference window before the second call.
	pastTime := info1.ModTime().Add(-2 * (1 << 0))
	if err := os.Chtimes(pagePath, pastTime, pastTime); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	info1Adjusted, err := os.Stat(pagePath)
	if err != nil {
		t.Fatalf("stat after chtimes: %v", err)
	}

	if err := h.EnsureWasmTemplates(); err != nil {
		t.Fatalf("second EnsureWasmTemplates: %v", err)
	}
	info2, err := os.Stat(pagePath)
	if err != nil {
		t.Fatalf("stat second pass: %v", err)
	}

	if !info1Adjusted.ModTime().Equal(info2.ModTime()) {
		t.Errorf("idempotent second call rewrote the template (mtime changed: %v -> %v)",
			info1Adjusted.ModTime(), info2.ModTime())
	}
}
