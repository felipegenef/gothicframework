/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	gothic_cli "github.com/felipegenef/gothicframework/v2/pkg/cli"
	"github.com/spf13/cobra"
)

// generateCmd represents the generate command
var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Compiles all Templ files into go files.",
	Long:  `Internal command intented to be called before deploy and between hot reloads to build golang files from templ files.`,
	RunE:  newBuildCommand(gothic_cli.NewCli()),
}

func init() {
	rootCmd.AddCommand(buildCmd)

}

type BuildCommand struct {
	cli *gothic_cli.GothicCli
}

func newBuildCommandCli(cli *gothic_cli.GothicCli) BuildCommand {
	return BuildCommand{
		cli: cli,
	}
}

func (command *BuildCommand) Build() error {
	if err := command.cli.Templ.Render(); err != nil {
		return err
	}

	config, err := command.cli.GetConfig()
	if err != nil {
		return err
	}

	if err := command.cli.FileBasedRouter.Render(config.GoModName); err != nil {
		return err
	}

	return nil
}

func newBuildCommand(cli gothic_cli.GothicCli) RunEFunc {
	return func(cmd *cobra.Command, args []string) error {
		command := newBuildCommandCli(&cli)
		return command.Build()
	}
}
