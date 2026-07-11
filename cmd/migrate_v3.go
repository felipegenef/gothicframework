/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

// newCLIInstallPath is the `go install` target for the v3 CLI. Gothic v3 no
// longer lives in this repository (github.com/felipegenef/gothicframework); it
// was split into github.com/gothicframework/* and the binary was renamed
// gothicframework -> gothic. This last v2 release only bridges to it.
const newCLIInstallPath = "github.com/gothicframework/cli/v3/cmd/gothic@latest"

var (
	migrateV3DryRun bool
	migrateV3Path   string
)

// migrateV3Cmd is a BRIDGE, not an in-process migration. The real v2->v3
// migration logic lives in the new `gothic` binary; this command installs that
// binary and delegates to it, then tells the user the command name changed.
var migrateV3Cmd = &cobra.Command{
	Use:   "migrate-v3",
	Short: "Migrate this project to Gothic v3 (new github.com/gothicframework repo + gothic CLI)",
	Long: `Gothic v3 moved to a new home: github.com/gothicframework, and the CLI was
renamed from "gothicframework" to "gothic".

This command bridges you there. It:

  1. Installs the v3 CLI:  go install ` + newCLIInstallPath + `
  2. Runs:                 gothic migrate-v3 [--dry-run] [--path <dir>]
  3. From here on, you use "gothic" instead of "gothicframework".

The actual migration is performed by the new gothic binary, so it always
reflects the latest v3 conversion logic.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runMigrateV3Bridge()
	},
}

func init() {
	migrateV3Cmd.Flags().BoolVar(&migrateV3DryRun, "dry-run", false, "Preview the migration without writing files")
	migrateV3Cmd.Flags().StringVar(&migrateV3Path, "path", ".", "Project root to migrate")
	rootCmd.AddCommand(migrateV3Cmd)
}

func runMigrateV3Bridge() error {
	fmt.Println("▸ Gothic v3 moved to github.com/gothicframework — installing the new 'gothic' CLI…")
	fmt.Printf("  go install %s\n\n", newCLIInstallPath)

	// 1. Install the new gothic CLI.
	install := exec.Command("go", "install", newCLIInstallPath)
	install.Stdout = os.Stdout
	install.Stderr = os.Stderr
	install.Env = os.Environ()
	if err := install.Run(); err != nil {
		return fmt.Errorf("failed to install the new gothic CLI (%s): %w\n"+
			"If the new repositories are private, make sure your git/GOPRIVATE access is configured.", newCLIInstallPath, err)
	}

	// 2. Locate the freshly installed binary.
	gothicBin, err := locateGothic()
	if err != nil {
		return err
	}

	// 3. Delegate the real migration to the new binary, forwarding our flags.
	delegated := []string{"migrate-v3"}
	if migrateV3DryRun {
		delegated = append(delegated, "--dry-run")
	}
	if migrateV3Path != "" && migrateV3Path != "." {
		delegated = append(delegated, "--path", migrateV3Path)
	}
	fmt.Printf("\n▸ Running: gothic %s\n\n", strings.Join(delegated, " "))
	run := exec.Command(gothicBin, delegated...)
	run.Stdout = os.Stdout
	run.Stderr = os.Stderr
	run.Stdin = os.Stdin
	run.Env = os.Environ()
	if err := run.Run(); err != nil {
		return fmt.Errorf("gothic migrate-v3 failed: %w", err)
	}

	// 4. The command name changed — make sure the user knows.
	fmt.Println()
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("  ✓ Migrated to Gothic v3")
	fmt.Println()
	fmt.Println("  From now on use the new CLI: 'gothic' (not 'gothicframework')")
	fmt.Println("    gothic dev        # was: gothicframework dev")
	fmt.Println("    gothic build      # was: gothicframework build")
	fmt.Println("    gothic deploy     # was: gothicframework deploy")
	fmt.Println()
	fmt.Println("  If 'gothic' isn't found, add your Go bin dir to PATH:")
	fmt.Println("    export PATH=\"$PATH:$(go env GOPATH)/bin\"")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	return nil
}

// locateGothic finds the gothic binary installed by `go install`, preferring the
// PATH, then GOBIN, then GOPATH/bin.
func locateGothic() (string, error) {
	bin := "gothic"
	if runtime.GOOS == "windows" {
		bin = "gothic.exe"
	}
	if p, err := exec.LookPath(bin); err == nil {
		return p, nil
	}
	if gobin := goEnv("GOBIN"); gobin != "" {
		if p := filepath.Join(gobin, bin); fileExists(p) {
			return p, nil
		}
	}
	if gopath := goEnv("GOPATH"); gopath != "" {
		if p := filepath.Join(gopath, "bin", bin); fileExists(p) {
			return p, nil
		}
	}
	return "", fmt.Errorf("installed the new CLI but couldn't find the '%s' binary on PATH; "+
		"add \"$(go env GOPATH)/bin\" to your PATH and run 'gothic migrate-v3'", bin)
}

func goEnv(key string) string {
	out, err := exec.Command("go", "env", key).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}
