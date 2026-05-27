/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/mod/modfile"
)

const (
	oldModulePath = "github.com/felipegenef/gothicframework"
	newModulePath = "github.com/felipegenef/gothicframework/v2"
)

// importPattern matches the old module path optionally already followed by /v2.
// When the (?:/v2) group is captured, we leave the match alone (idempotent).
var importPattern = regexp.MustCompile(`github\.com/felipegenef/gothicframework(/v2)?`)

var (
	migrateV2DryRun bool
	migrateV2Path   string
	// skipTidyForTest is set by tests to bypass `go mod tidy` (fixtures use a
	// fake module path that the proxy cannot resolve).
	skipTidyForTest bool
)

var migrateV2Cmd = &cobra.Command{
	Use:   "migrate-v2",
	Short: "Migrate a Gothic project's imports to the /v2 module path",
	Long: `Rewrites Go and templ imports from github.com/felipegenef/gothicframework
to github.com/felipegenef/gothicframework/v2, updates go.mod, and runs
go mod tidy. Idempotent: running it twice is safe.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runMigrateV2(migrateV2Path, migrateV2DryRun, cmd.OutOrStdout())
	},
}

func init() {
	migrateV2Cmd.Flags().BoolVar(&migrateV2DryRun, "dry-run", false, "Do not write files; report what would change")
	migrateV2Cmd.Flags().StringVar(&migrateV2Path, "path", ".", "Project root to migrate")
	rootCmd.AddCommand(migrateV2Cmd)
}

// runMigrateV2 performs the migration. Returns nil and prints to out on success.
func runMigrateV2(root string, dryRun bool, out interface {
	Write(p []byte) (int, error)
}) error {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}

	// 1) Check go.mod references old path.
	goModPath := filepath.Join(absRoot, "go.mod")
	goModBytes, err := os.ReadFile(goModPath)
	if err != nil {
		return fmt.Errorf("read go.mod: %w", err)
	}
	if !referencesOldPath(goModBytes) && !anyFileReferencesOldPath(absRoot) {
		fmt.Fprintln(out, "nothing to migrate")
		return nil
	}

	// 2) git porcelain check (warn only).
	if _, err := os.Stat(filepath.Join(absRoot, ".git")); err == nil {
		gitCmd := exec.Command("git", "status", "--porcelain")
		gitCmd.Dir = absRoot
		if porcelain, gerr := gitCmd.Output(); gerr == nil && len(bytes.TrimSpace(porcelain)) > 0 {
			fmt.Fprintln(out, "warning: uncommitted changes detected in repository")
		}
	}

	// 3) Walk and rewrite .go and .templ files.
	filesUpdated := 0
	linesChanged := 0
	walkErr := filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == "vendor" || name == ".git" || name == "node_modules" {
				return fs.SkipDir
			}
			return nil
		}
		// skip cache file
		rel, _ := filepath.Rel(absRoot, path)
		if rel == filepath.Join(".gothicCli", "wasm-cache.json") {
			return nil
		}
		ext := filepath.Ext(path)
		if ext != ".go" && ext != ".templ" {
			return nil
		}
		changed, n, rerr := rewriteFile(path, dryRun)
		if rerr != nil {
			return rerr
		}
		if changed {
			filesUpdated++
			linesChanged += n
		}
		return nil
	})
	if walkErr != nil {
		return walkErr
	}

	// 4) Update go.mod via modfile.
	goModUpdated, err := rewriteGoMod(goModPath, goModBytes, dryRun)
	if err != nil {
		return err
	}

	// 5) go mod tidy (skip on dry run).
	if !dryRun && !skipTidyForTest && (goModUpdated || filesUpdated > 0) {
		tidy := exec.Command("go", "mod", "tidy")
		tidy.Dir = absRoot
		tidy.Stdout = out
		tidy.Stderr = out
		if terr := tidy.Run(); terr != nil {
			return fmt.Errorf("go mod tidy: %w", terr)
		}
	}

	goModStr := "no"
	if goModUpdated {
		goModStr = "yes"
	}
	fmt.Fprintf(out, "%d files updated, %d lines changed, go.mod updated: %s\n",
		filesUpdated, linesChanged, goModStr)
	return nil
}

// referencesOldPath reports whether content contains old path NOT followed by /v2.
func referencesOldPath(content []byte) bool {
	matches := importPattern.FindAllSubmatchIndex(content, -1)
	for _, m := range matches {
		// group 1 ((/v2)?) start index is m[2]
		if m[2] == -1 {
			return true
		}
	}
	return false
}

func anyFileReferencesOldPath(root string) bool {
	found := false
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || found {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if name == "vendor" || name == ".git" || name == "node_modules" {
				return fs.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(path)
		if ext != ".go" && ext != ".templ" {
			return nil
		}
		b, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		if referencesOldPath(b) {
			found = true
		}
		return nil
	})
	return found
}

// rewriteFile rewrites a single file. Returns (changed, linesChanged, err).
func rewriteFile(path string, dryRun bool) (bool, int, error) {
	original, err := os.ReadFile(path)
	if err != nil {
		return false, 0, err
	}
	updated := importPattern.ReplaceAllFunc(original, func(match []byte) []byte {
		if bytes.HasSuffix(match, []byte("/v2")) {
			return match
		}
		return []byte(newModulePath)
	})
	if bytes.Equal(original, updated) {
		return false, 0, nil
	}
	n := countChangedLines(original, updated)
	if !dryRun {
		info, _ := os.Stat(path)
		mode := fs.FileMode(0o644)
		if info != nil {
			mode = info.Mode()
		}
		if werr := os.WriteFile(path, updated, mode); werr != nil {
			return false, 0, werr
		}
	}
	return true, n, nil
}

func countChangedLines(a, b []byte) int {
	la := strings.Split(string(a), "\n")
	lb := strings.Split(string(b), "\n")
	n := 0
	max := len(la)
	if len(lb) > max {
		max = len(lb)
	}
	for i := 0; i < max; i++ {
		var x, y string
		if i < len(la) {
			x = la[i]
		}
		if i < len(lb) {
			y = lb[i]
		}
		if x != y {
			n++
		}
	}
	return n
}

// rewriteGoMod parses, updates, and writes go.mod if needed. Returns (updated, err).
func rewriteGoMod(path string, content []byte, dryRun bool) (bool, error) {
	mf, err := modfile.Parse(path, content, nil)
	if err != nil {
		return false, fmt.Errorf("parse go.mod: %w", err)
	}
	changed := false
	for _, r := range mf.Require {
		if r.Mod.Path == oldModulePath {
			// /v2 modules require a v2.x.x version per SemVer import compatibility.
			// We seed with v2.0.0; `go mod tidy` will resolve to the actual latest.
			newVersion := r.Mod.Version
			if !strings.HasPrefix(newVersion, "v2.") {
				newVersion = "v2.0.0"
			}
			if err := mf.AddRequire(newModulePath, newVersion); err != nil {
				return false, err
			}
			if err := mf.DropRequire(oldModulePath); err != nil {
				return false, err
			}
			changed = true
		}
	}
	for _, rep := range mf.Replace {
		if rep.Old.Path == oldModulePath {
			if err := mf.AddReplace(newModulePath, rep.Old.Version, rep.New.Path, rep.New.Version); err != nil {
				return false, err
			}
			if err := mf.DropReplace(oldModulePath, rep.Old.Version); err != nil {
				return false, err
			}
			changed = true
		}
	}
	if !changed {
		return false, nil
	}
	mf.Cleanup()
	out, err := mf.Format()
	if err != nil {
		return false, err
	}
	if !dryRun {
		if werr := os.WriteFile(path, out, 0o644); werr != nil {
			return false, werr
		}
	}
	return true, nil
}

