package helpers

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/mod/modfile"
)

// Module bridging: when we compile a per-page or topic-manager WASM, the
// generated main.go may reference symbols from the user's project (e.g. helper
// functions or types defined in the user's package). To make those imports
// resolvable, we write a temp go.mod that:
//
//   - declares the temp module as `wasm-runtime`
//   - requires the user's module at a synthetic pseudo-version
//   - replaces that requirement with the absolute path to the user's project
//
// The Go toolchain (and TinyGo, which delegates module resolution to `go`)
// will then resolve any `user.module/...` import directly off the user's
// working tree.

// ReadUserModulePath reads the module path and Go version from the user's
// go.mod at projectRoot. Uses a lax parse so that unknown or future directives
// do not cause failures.
func ReadUserModulePath(projectRoot string) (modulePath string, goVersion string, err error) {
	gomodPath := filepath.Join(projectRoot, "go.mod")
	data, err := os.ReadFile(gomodPath)
	if err != nil {
		return "", "", fmt.Errorf("module bridge: read %s: %w", gomodPath, err)
	}
	parsed, err := modfile.ParseLax(gomodPath, data, nil)
	if err != nil {
		return "", "", fmt.Errorf("module bridge: parse %s: %w", gomodPath, err)
	}
	if parsed.Module == nil {
		return "", "", fmt.Errorf("module bridge: %s has no module directive", gomodPath)
	}
	modulePath = parsed.Module.Mod.Path
	if parsed.Go != nil {
		goVersion = parsed.Go.Version
	}
	return modulePath, goVersion, nil
}

// WriteBridgeGoMod writes a temp go.mod into tempDir that links back to the
// user's project via a replace directive. It also copies the user's go.sum
// (if present) into tempDir so module verification succeeds.
//
// The generated go.mod intentionally omits any `toolchain` directive so that
// the temp build is not forced onto a specific toolchain.
func WriteBridgeGoMod(tempDir, userModulePath, userProjectRoot, goVersion string) error {
	if goVersion == "" {
		goVersion = "1.21"
	}
	absUserRoot, err := filepath.Abs(userProjectRoot)
	if err != nil {
		return fmt.Errorf("module bridge: abs %s: %w", userProjectRoot, err)
	}

	f := new(modfile.File)
	if err := f.AddModuleStmt("wasm-runtime"); err != nil {
		return fmt.Errorf("module bridge: add module: %w", err)
	}
	if err := f.AddGoStmt(goVersion); err != nil {
		return fmt.Errorf("module bridge: add go %s: %w", goVersion, err)
	}
	const pseudo = "v0.0.0-00010101000000-000000000000"
	if err := f.AddRequire(userModulePath, pseudo); err != nil {
		return fmt.Errorf("module bridge: add require %s: %w", userModulePath, err)
	}
	if err := f.AddReplace(userModulePath, "", absUserRoot, ""); err != nil {
		return fmt.Errorf("module bridge: add replace %s: %w", userModulePath, err)
	}
	// Strip any toolchain directive that may have been inserted by modfile
	// helpers. (AddGoStmt does NOT add one as of x/mod v0.26, but be defensive.)
	f.Toolchain = nil

	out, err := f.Format()
	if err != nil {
		return fmt.Errorf("module bridge: format go.mod: %w", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "go.mod"), out, 0644); err != nil {
		return fmt.Errorf("module bridge: write go.mod: %w", err)
	}

	// Copy the user's go.sum if it exists so that `go build -mod=mod` does
	// not re-fetch indirect deps when it can validate them off-disk.
	srcSum := filepath.Join(absUserRoot, "go.sum")
	if _, err := os.Stat(srcSum); err == nil {
		if err := copyFile(srcSum, filepath.Join(tempDir, "go.sum")); err != nil {
			return fmt.Errorf("module bridge: copy go.sum: %w", err)
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
