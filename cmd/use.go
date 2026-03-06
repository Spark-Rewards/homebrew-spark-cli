package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Spark-Rewards/homebrew-spark-cli/internal/config"
	"github.com/Spark-Rewards/homebrew-spark-cli/internal/git"
	"github.com/Spark-Rewards/homebrew-spark-cli/internal/workspace"
	"github.com/spf13/cobra"
)

var (
	useBuildCmd string
	useDeps     []string
)

const defaultGitHubOrg = "Spark-Rewards"

var useCmd = &cobra.Command{
	Use:   "use <repo>",
	Short: "Clone a repo into workspace (--build, --deps | -h)",
	Long: `Clones a GitHub repository into the current workspace and registers it
in the workspace manifest.

If only a repo name is provided, it defaults to the Spark-Rewards org.

Examples:
  spark-cli use BusinessAPI                              # clones Spark-Rewards/BusinessAPI
  spark-cli use other-org/SomeRepo                       # clones other-org/SomeRepo
  spark-cli use git@github.com:other-org/Repo.git        # full URL`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		repoArg := args[0]

		// Find workspace
		wsPath, err := workspace.Find()
		if err != nil {
			return fmt.Errorf("you must be inside a spark-cli workspace — run 'spark-cli create workspace <path>' first")
		}

		// Resolve the remote URL
		remote := resolveRemote(repoArg)
		repoName := git.RepoNameFromRemote(repoArg)
		targetDir := filepath.Join(wsPath, repoName)

		// Check if already cloned
		if _, err := os.Stat(targetDir); err == nil {
			if git.IsRepo(targetDir) {
				fmt.Printf("Repository '%s' already exists at %s\n", repoName, targetDir)
				// Still register it in manifest if not present
				return registerRepo(wsPath, repoName, remote, targetDir)
			}
			return fmt.Errorf("directory %s exists but is not a git repository", targetDir)
		}

		// Clone
		fmt.Printf("Cloning %s into %s...\n", remote, targetDir)
		if err := git.Clone(remote, targetDir); err != nil {
			return fmt.Errorf("git clone failed: %w", err)
		}

		// Register in workspace manifest
		if err := registerRepo(wsPath, repoName, remote, targetDir); err != nil {
			return err
		}

		fmt.Printf("Repository '%s' added to workspace\n", repoName)
		return nil
	},
}

func resolveRemote(arg string) string {
	// If it's already a full URL, use as-is
	if git.BuildRemoteURL(arg) == arg {
		return arg
	}

	// If no slash, prepend Spark-Rewards org (or config override)
	if !containsSlash(arg) {
		org := defaultGitHubOrg
		cfg, err := config.LoadGlobal()
		if err == nil && cfg.DefaultGithubOrg != "" {
			org = cfg.DefaultGithubOrg
		}
		return git.BuildRemoteURL(org + "/" + arg)
	}

	return git.BuildRemoteURL(arg)
}

func containsSlash(s string) bool {
	for _, c := range s {
		if c == '/' {
			return true
		}
	}
	return false
}

func registerRepo(wsPath, name, remote, targetDir string) error {
	relPath, _ := filepath.Rel(wsPath, targetDir)
	repo := workspace.RepoDef{
		Remote:       remote,
		Path:         relPath,
		BuildCommand: useBuildCmd,
		Dependencies: useDeps,
	}
	if err := workspace.AddRepo(wsPath, name, repo); err != nil {
		return err
	}

	if err := workspace.GenerateVSCodeWorkspace(wsPath); err != nil {
		fmt.Printf("Warning: failed to update VS Code workspace: %v\n", err)
	}
	return nil
}

func init() {
	useCmd.Flags().StringVar(&useBuildCmd, "build", "", "Build command for this repo (e.g., 'npm run build')")
	useCmd.Flags().StringSliceVar(&useDeps, "deps", nil, "Dependencies (other repo names that must build first)")
	useCmd.GroupID = "workspace"
	rootCmd.AddCommand(useCmd)
}
