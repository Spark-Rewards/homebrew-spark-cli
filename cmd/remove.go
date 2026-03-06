package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Spark-Rewards/homebrew-spark-cli/internal/workspace"
	"github.com/spf13/cobra"
)

var removeCmd = &cobra.Command{
	Use:   "remove <repo-name>",
	Short: "Remove a repo and delete its folder (rm)",
	Long: `Unregisters a repo from workspace.json and deletes the repo directory.

Example:
  spark-cli remove BusinessAPI`,
	Aliases: []string{"rm"},
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		wsPath, err := workspace.Find()
		if err != nil {
			return err
		}

		ws, err := workspace.Load(wsPath)
		if err != nil {
			return err
		}

		repo, ok := ws.Repos[name]
		if !ok {
			return fmt.Errorf("repo '%s' not found in workspace", name)
		}

		repoDir := filepath.Join(wsPath, repo.Path)
		rel, err := filepath.Rel(wsPath, repoDir)
		if err != nil {
			return fmt.Errorf("invalid repo path: %w", err)
		}
		if strings.HasPrefix(rel, "..") || rel == ".." {
			return fmt.Errorf("repo path escapes workspace — refusing to delete %s", repoDir)
		}

		if err := workspace.RemoveRepo(wsPath, name); err != nil {
			return err
		}

		if err := os.RemoveAll(repoDir); err != nil {
			return fmt.Errorf("removed from manifest but failed to delete directory %s: %w", repoDir, err)
		}

		if err := workspace.GenerateVSCodeWorkspace(wsPath); err != nil {
			fmt.Printf("Warning: failed to update VS Code workspace file: %v\n", err)
		}

		fmt.Printf("Removed '%s' from workspace and deleted %s\n", name, repoDir)
		return nil
	},
}

func init() {
	removeCmd.GroupID = "workspace"
	rootCmd.AddCommand(removeCmd)
}
