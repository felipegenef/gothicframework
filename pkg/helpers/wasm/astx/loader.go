package astx

import (
	"fmt"
	"go/ast"
	"go/token"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/packages"
)

// Entry pairs a loaded package with a single file's syntax tree.
type Entry struct {
	Pkg  *packages.Package
	File *ast.File
}

// Loader is a one-shot loader over a directory's Go packages, indexed by
// absolute file path.
type Loader struct {
	Fset   *token.FileSet
	byFile map[string]Entry
}

// NewLoader loads "./..." rooted at dir using go/packages and indexes every
// compiled Go file by its absolute path.
func NewLoader(dir string) (*Loader, error) {
	fset := token.NewFileSet()
	cfg := &packages.Config{
		Mode: packages.NeedName |
			packages.NeedFiles |
			packages.NeedCompiledGoFiles |
			packages.NeedSyntax |
			packages.NeedTypes |
			packages.NeedTypesInfo |
			packages.NeedImports |
			packages.NeedDeps |
			packages.NeedModule,
		Dir:  dir,
		Fset: fset,
	}

	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		return nil, fmt.Errorf("astx: packages.Load: %w", err)
	}

	byFile := make(map[string]Entry)
	var errMsgs []string

	for _, pkg := range pkgs {
		if len(pkg.Errors) > 0 {
			for _, e := range pkg.Errors {
				errMsgs = append(errMsgs, fmt.Sprintf("%s: %s", pkg.PkgPath, e.Error()))
			}
			continue
		}
		// Index-align CompiledGoFiles with Syntax.
		n := len(pkg.CompiledGoFiles)
		if len(pkg.Syntax) < n {
			n = len(pkg.Syntax)
		}
		for i := 0; i < n; i++ {
			abs, absErr := filepath.Abs(pkg.CompiledGoFiles[i])
			if absErr != nil {
				return nil, fmt.Errorf("astx: filepath.Abs(%q): %w", pkg.CompiledGoFiles[i], absErr)
			}
			byFile[abs] = Entry{Pkg: pkg, File: pkg.Syntax[i]}
		}
	}

	if len(errMsgs) > 0 {
		return nil, fmt.Errorf("astx: package load errors:\n%s", strings.Join(errMsgs, "\n"))
	}

	return &Loader{Fset: fset, byFile: byFile}, nil
}

// Get returns the loaded Entry for an absolute file path.
func (l *Loader) Get(absPath string) (Entry, error) {
	e, ok := l.byFile[absPath]
	if !ok {
		return Entry{}, fmt.Errorf("astx: no entry for %q", absPath)
	}
	return e, nil
}
