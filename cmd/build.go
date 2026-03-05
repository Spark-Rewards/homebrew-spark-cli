package cmd

import (
	"fmt"
	"os"
	"time"

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
  spark-cli build InternalModel        # build Smithy SDK
  spark-cli build InternalAPILambda    # build Lambda (repo name works too)
  spark-cli build internal-model       # Nix package name also works
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
		desc := nix.PackageNames[pkg]
		if desc == "" {
			desc = pkg
		}

		fmt.Printf("🔥 Building %s\n\n", desc)

		run := nix.Run
		if verbose {
			run = nix.RunRaw
		}

		start := time.Now()
		if err := run(ws, "build", "--impure", ".#"+pkg, "--print-build-logs"); err != nil {
			fmt.Println()
			fmt.Printf("✗ Build failed (%s) — run with --verbose to see full output\n", pkg)
			return fmt.Errorf("build failed: %w", err)
		}

		elapsed := time.Since(start).Round(time.Second)
		fmt.Println()
		fmt.Printf("✓ Build complete  %s  (%s) → ./result\n", pkg, elapsed)
		fmt.Println()
		fmt.Println("  Next:")
		fmt.Println("    spark-cli cdk --profile pipeline diff    # preview changes")
		fmt.Println("    spark-cli cdk --profile pipeline deploy  # deploy to beta")
		return nil
	},
}

var buildAllCmd = &cobra.Command{
	Use:   "build-all",
	Short: "Build all Nix packages in dependency order (| -h)",
	Long: `Build all SparkRewards packages in the correct dependency order.

Local clones are used when present; uncloned repos are fetched from GitHub.
Output at ./result (a symlink into the Nix store).

Examples:
  spark-cli build-all
  spark-cli build-all --verbose    # show raw Nix output for debugging`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ws, err := nix.FindWorkspace()
		if err != nil {
			return err
		}

		verbose, _ := cmd.Flags().GetBool("verbose")

		fmt.Println("🔥 Building all packages")
		fmt.Println()

		// Show source resolution — local clone vs GitHub
		for _, repo := range nix.TrackedRepos {
			repoPath := ws + "/" + repo
			if isGitDir(repoPath) {
				branch := gitBranch(repoPath)
				fmt.Printf("  📁 %-28s local [%s]\n", repo, branch)
			} else {
				fmt.Printf("  🌐 %-28s GitHub (pinned)\n", repo)
			}
		}
		fmt.Println()

		run := nix.Run
		if verbose {
			run = nix.RunRaw
		}

		start := time.Now()
		buildErr := run(ws, "build", "--impure", ".#all", "--print-build-logs")
		elapsed := time.Since(start).Round(time.Second)

		fmt.Println()
		fmt.Println("─────────────────────────────────────────────────────")

		if buildErr != nil {
			fmt.Printf("✗ Build failed (%s)\n", elapsed)
			fmt.Println()
			fmt.Println("  Tip: build individual packages to isolate the failure:")
			fmt.Println("    spark-cli build InternalModel")
			fmt.Println("    spark-cli build InternalAPILambda")
			fmt.Println("    spark-cli build InternalWebsiteClient")
			fmt.Println()
			fmt.Println("  Or add --verbose to see the full Nix log:")
			fmt.Println("    spark-cli build-all --verbose")
			return fmt.Errorf("build failed: %w", buildErr)
		}

		fmt.Printf("✓ All packages built (%s) → ./result\n", elapsed)
		fmt.Println()
		fmt.Println("  Next:")
		fmt.Println("    spark-cli cdk --profile pipeline diff    # preview changes")
		fmt.Println("    spark-cli cdk --profile pipeline deploy  # deploy to beta")
		return nil
	},
}

func printBuildUsage() {
	fmt.Println("Usage: spark-cli build <package>")
	fmt.Println()
	fmt.Println("Available packages:")
	// Print in a logical grouping
	groups := []struct {
		label string
		pkgs  []string
	}{
		{"Internal service chain", []string{"internal-model", "internal-api-lambda", "internal-website-client", "internal-service-cdk", "internal-all"}},
		{"Business service chain", []string{"business-model", "business-api-lambda", "business-website", "business-service-cdk", "business-all"}},
		{"App service chain", []string{"app-model", "app-api-lambda", "app-service-cdk", "app-all"}},
		{"Shared", []string{"auditor-lambda", "core-pipeline", "mobile-app", "shared-all"}},
		{"All", []string{"all"}},
	}
	for _, g := range groups {
		fmt.Printf("\n  %s:\n", g.label)
		for _, pkg := range g.pkgs {
			desc := nix.PackageNames[pkg]
			if desc != "" {
				fmt.Printf("    %-28s %s\n", pkg, desc)
			}
		}
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
	buildAllCmd.GroupID = "dev"
	rootCmd.AddCommand(buildCmd)
	rootCmd.AddCommand(buildAllCmd)
}
