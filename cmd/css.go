/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	gothic_cli "github.com/felipegenef/gothicframework/v2/pkg/cli"
	"github.com/spf13/cobra"
)

var cssCmd = &cobra.Command{
	Use:   "css",
	Short: "Build Tailwind CSS from source.",
	Long: `Runs the Tailwind CSS standalone CLI to compile your CSS.

The Tailwind binary is automatically downloaded to an OS cache directory
on first use. You can override the binary path by setting "tailwindBinary"
in gothic-config.json.`,
	RunE: newCssCommand(gothic_cli.NewCli()),
}

func init() {
	rootCmd.AddCommand(cssCmd)
}

type CssCommand struct {
	cli *gothic_cli.GothicCli
}

func newCssCommandCli(cli *gothic_cli.GothicCli) CssCommand {
	return CssCommand{
		cli: cli,
	}
}

func newCssCommand(cli gothic_cli.GothicCli) RunEFunc {
	return func(cmd *cobra.Command, args []string) error {
		command := newCssCommandCli(&cli)
		// Load config to pick up tailwindBinary override if present
		command.cli.GetConfig()
		return command.cli.Tailwind.Build()
	}
}
