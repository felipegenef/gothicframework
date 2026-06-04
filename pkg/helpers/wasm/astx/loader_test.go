package astx

import (
	"path/filepath"
	"runtime"
	"testing"
)

// astxDir returns this package's directory, derived at runtime so the tests
// work in a fresh clone, on CI, or on any machine — never hardcoded.
func astxDir(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	return filepath.Dir(thisFile)
}

func TestNewLoader_LoadsSelf(t *testing.T) {
	l, err := NewLoader(astxDir(t))
	if err != nil {
		t.Fatalf("NewLoader: %v", err)
	}
	if len(l.byFile) == 0 {
		t.Fatalf("expected at least one indexed file, got 0")
	}
	var anyPath string
	for p := range l.byFile {
		anyPath = p
		break
	}
	entry, err := l.Get(anyPath)
	if err != nil {
		t.Fatalf("Get(%q): %v", anyPath, err)
	}
	if entry.File == nil {
		t.Fatalf("entry.File is nil for %q", anyPath)
	}
}

func TestLoader_Get_NotFound(t *testing.T) {
	l, err := NewLoader(astxDir(t))
	if err != nil {
		t.Fatalf("NewLoader: %v", err)
	}
	if _, err := l.Get("/nonexistent/path.go"); err == nil {
		t.Fatalf("expected error for nonexistent path, got nil")
	}
}
