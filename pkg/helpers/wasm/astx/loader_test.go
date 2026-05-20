package astx

import (
	"testing"
)

const astxDir = "/home/felipe/DEV/gothic-cli/pkg/helpers/wasm/astx"

func TestNewLoader_LoadsSelf(t *testing.T) {
	l, err := NewLoader(astxDir)
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
	l, err := NewLoader(astxDir)
	if err != nil {
		t.Fatalf("NewLoader: %v", err)
	}
	if _, err := l.Get("/nonexistent/path.go"); err == nil {
		t.Fatalf("expected error for nonexistent path, got nil")
	}
}
