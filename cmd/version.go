/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var CURRENT_VERSION string = "v2.16.4"

// CURRENT_COMMIT is the Go pseudo-version of the gothicframework module that
// ships with this binary. Used by init to pin the exact version before go mod
// tidy, so new projects don't resolve a stale proxy-cached version.
// Update this alongside CURRENT_VERSION on every release using:
//
//	GOWORK=off go list -m github.com/felipegenef/gothicframework@<short-hash>
var CURRENT_COMMIT string = "v0.0.0-20260527141254-8f8d0210b2d1"

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
