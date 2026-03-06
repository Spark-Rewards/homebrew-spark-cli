package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

var rootCmd = &cobra.Command{
	Use:     "spark-cli",
	Short:   "spark-cli — SparkRewards multi-repo workspace CLI",
	Version: Version,
	Long: `spark-cli manages the SparkRewards multi-repo workspace.

Reproducible builds with Nix — every developer gets the same toolchain,
every build produces the same output.

Quick start (new developer):
  spark-cli setup        # One-time setup: Nix + GitHub token + dev cache
  spark-cli dev          # Enter the dev shell (Node 22, Java 17, AWS CLI...)
  spark-cli build-all    # Build everything

Run any command with -h for details.
`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.SetVersionTemplate(fmt.Sprintf("spark-cli %s (%s %s)\n", Version, Commit, Date))
	rootCmd.CompletionOptions.DisableDefaultCmd = true

	// No "help" subcommand — use -h/--help only
	rootCmd.SetHelpCommand(&cobra.Command{Hidden: true})

	// Command groups for organised --help output
	rootCmd.AddGroup(
		&cobra.Group{ID: "setup", Title: "Setup & Diagnostics:"},
		&cobra.Group{ID: "dev", Title: "Development:"},
		&cobra.Group{ID: "workspace", Title: "Workspace Management:"},
		&cobra.Group{ID: "infra", Title: "Infrastructure:"},
	)
}
