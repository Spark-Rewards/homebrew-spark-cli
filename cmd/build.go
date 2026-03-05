package cmd

import (
	"fmt"
	"os"

	"github.com/Spark-Rewards/homebrew-spark-cli/internal/nix"
	"github.com/spf13/cobra"
)

var buildCmd = &cobra.Command{
	Use:   "build [package]",
	Short: "Build a Nix package (nix build --impure .#<package> | -h)",
	Long: `Build a SparkRewards package using the Nix build system.

Accepts either a Nix package name or a repo name:
  internal-model          ←→  InternalModel
  internal-api-lambda     ←→  InternalAPILambda
  internal-website-client ←→  InternalWebsiteClient
  internal-service-cdk    ←→  InternalServiceCDK

Builds are fully reproducible. Local clones are used when present;
otherwise the repo is fetched from GitHub at the pinned flake.lock revision.

Output is available at ./result (a symlink into the Nix store).

Examples:
  spark-cli build internal-model       # build Smithy SDK
  spark-cli build InternalAPILambda    # repo name also works
  spark-cli build-all                  # build everything`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ws, err := nix.FindWorkspace()
		if err != nil {
			return err
		}

		if len(args) == 0 {
			printBuildUsage()
			return nil
		}

		verbose, _ := cmd.Flags().GetBool("verbose")
		pkg := nix.RepoToPackage(args[0])

		fmt.Printf("🔥 Building %s\n\n", pkg)

		run := nix.Run
		if verbose {
			run = nix.RunRaw
		}

		if err := run(ws, "build", "--impure", ".#"+pkg, "--print-build-logs"); err != nil {
			return fmt.Errorf("build failed: %w", err)
		}

		fmt.Println()
		fmt.Println("✓ Build complete → ./result")
		return nil
	},
}

var buildAllCmd = &cobra.Command{
	Use:   "build-all",
	Short: "Build all Nix packages in dependency order (| -h)",
	Long: `Build all SparkRewards Internal packages in the correct dependency order:
  InternalModel → InternalAPILambda + InternalWebsiteClient → InternalServiceCDK

Local clones are used when present; uncloned repos are fetched from GitHub.
Output at ./result (a symlink into the Nix store).

Examples:
  spark-cli build-all`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ws, err := nix.FindWorkspace()
		if err != nil {
			return err
		}

		verbose, _ := cmd.Flags().GetBool("verbose")

		fmt.Println("🔥 Building all packages")
		fmt.Println()

		// Show source resolution
		for _, repo := range nix.TrackedRepos {
			repoPath := ws + "/" + repo
			if isGitDir(repoPath) {
				branch := gitBranch(repoPath)
				fmt.Printf("  📁 %-28s local [%s]\n", repo, branch)
			} else {
				fmt.Printf("  🌐 %-28s GitHub\n", repo)
			}
		}
		fmt.Println()

		run := nix.Run
		if verbose {
			run = nix.RunRaw
		}

		if err := run(ws, "build", "--impure", ".#all", "--print-build-logs"); err != nil {
			return fmt.Errorf("build failed: %w", err)
		}

		fmt.Println()
		fmt.Println("✓ All packages built → ./result")
		return nil
	},
}

func printBuildUsage() {
	fmt.Println("Usage: spark-cli build <package>")
	fmt.Println()
	fmt.Println("Available packages:")
	for pkg, desc := range nix.PackageNames {
		if pkg == "all" {
			continue
		}
		fmt.Printf("  %-28s %s\n", pkg, desc)
	}
	fmt.Println()
	fmt.Println("Repo names also work: InternalModel, InternalAPILambda, etc.")
	fmt.Println()
	fmt.Println("To build everything: spark-cli build-all")
}

// isGitDir reports whether path contains a .git directory.
func isGitDir(path string) bool {
	_, err := os.Stat(path + "/.git")
	return err == nil
}

// gitBranch returns the current branch name for a git repo, or "?" on error.
func gitBranch(path string) string {
	out, err := runGit(path, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "?"
	}
	return out
}

func init() {
	buildCmd.Flags().BoolP("verbose", "v", false, "Show raw Nix output (for debugging)")
	buildAllCmd.Flags().BoolP("verbose", "v", false, "Show raw Nix output (for debugging)")
	buildCmd.GroupID = "dev"
	rootCmd.AddCommand(buildCmd)
	buildAllCmd.GroupID = "dev"
	rootCmd.AddCommand(buildAllCmd)
}
