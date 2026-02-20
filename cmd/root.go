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
	Use:     "spk",
	Short:   "spk — multi-repo workspace CLI",
	Version: Version,
	Long: `spk manages multi-repo workspaces with shared environment and smart builds.

Core Commands:
  create workspace <path>   Create a new workspace
  use <repo>                Add a repo to the workspace  
  sync                      Sync repos + refresh .env (auto-login)
  run <script>              Run npm/gradle script (build, test, etc.)

Quick Start:
  spk create workspace ./my-project
  cd my-project
  spk use AppModel
  spk use AppAPI
  spk sync
  cd AppAPI && spk run build`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.SetVersionTemplate(fmt.Sprintf("spk %s (%s %s)\n", Version, Commit, Date))
	rootCmd.CompletionOptions.DisableDefaultCmd = true
}
