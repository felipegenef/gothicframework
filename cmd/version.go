/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var CURRENT_VERSION string = "v2.17.0"


// versionCmd represents the version command
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show current Gothic Framework Version",
	Long:  `Show current Gothic Framework Version`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("Gothic Framework - %s\n", CURRENT_VERSION)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
