package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "spk",
	Short: "spk — multi-repo workspace CLI",
	Long: `spk manages multi-repo workspaces with shared environment and smart builds.

Core Commands:
  create workspace <path>   Create a new workspace
  use <repo>                Add a repo to the workspace  
  sync                      Sync repos + refresh .env (auto-login)
  run build                 Build with local dependency linking
  run test                  Run tests

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
