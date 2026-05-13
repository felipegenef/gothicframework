package cmd

import (
	"fmt"
	"os"
	"os/exec"

	gothic_cli "github.com/felipegenef/gothicframework/pkg/cli"
	"github.com/spf13/cobra"
)

var wasmCmd = &cobra.Command{
	Use:   "wasm",
	Short: "Manage WASM reactive pages.",
	Long: `Scans src/pages and src/components for routes with PageState set,
then compiles each one to a .wasm.gz file using the managed TinyGo toolchain.

TinyGo is downloaded automatically on first use and cached in the OS cache dir.
You can override the binary path with "wasmBinary" in gothic-config.json.`,
	RunE: newWasmBuildCommand(gothic_cli.NewCli()),
}

var wasmInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Download and cache TinyGo (re-downloads even if already cached).",
	RunE: func(cmd *cobra.Command, args []string) error {
		cli := gothic_cli.NewCli()
		cli.GetConfig()
		// Force re-download by clearing the binary first.
		bin := cli.Wasm.TinyGoBinary()
		_ = os.Remove(bin)
		if err := cli.Wasm.EnsureBinary(); err != nil {
			return err
		}
		fmt.Printf("wasm: TinyGo %s installed at %s\n", cli.Wasm.Version, cli.Wasm.TinyGoRoot())
		return nil
	},
}

var wasmCleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Remove public/wasm/ and public/wasm_exec.js.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := os.RemoveAll("public/wasm"); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove public/wasm: %w", err)
		}
		if err := os.Remove("public/wasm_exec.js"); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove public/wasm_exec.js: %w", err)
		}
		fmt.Println("wasm: cleaned public/wasm/ and public/wasm_exec.js")
		return nil
	},
}

var wasmVersionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the managed TinyGo version.",
	RunE: func(cmd *cobra.Command, args []string) error {
		cli := gothic_cli.NewCli()
		cli.GetConfig()
		if err := cli.Wasm.EnsureBinary(); err != nil {
			return err
		}
		tinygo := cli.Wasm.TinyGoBinary()
		if cli.Wasm.ConfigOverride != "" {
			tinygo = cli.Wasm.ConfigOverride
		}
		c := exec.Command(tinygo, "version")
		c.Env = append(os.Environ(), cli.Wasm.Environ()...)
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		return c.Run()
	},
}

func init() {
	wasmCmd.AddCommand(wasmInstallCmd)
	wasmCmd.AddCommand(wasmCleanCmd)
	wasmCmd.AddCommand(wasmVersionCmd)
	rootCmd.AddCommand(wasmCmd)
}

type WasmBuildCommand struct {
	cli *gothic_cli.GothicCli
}

func newWasmBuildCommand(cli gothic_cli.GothicCli) RunEFunc {
	return func(cmd *cobra.Command, args []string) error {
		cli.GetConfig()
		pages, err := cli.Wasm.ScanPages("src/pages", "src/components")
		if err != nil {
			return fmt.Errorf("wasm: scan: %w", err)
		}
		if len(pages) == 0 {
			fmt.Println("wasm: no pages with PageState found")
			return nil
		}
		fmt.Printf("wasm: found %d page(s) with PageState\n", len(pages))
		return cli.Wasm.GenerateAll(pages, "public/wasm")
	}
}
