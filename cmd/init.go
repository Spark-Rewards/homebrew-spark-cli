package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/Spark-Rewards/homebrew-spark-cli/internal/assets"
	"github.com/Spark-Rewards/homebrew-spark-cli/internal/nix"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init [path]",
	Short: "Initialize a SparkRewards workspace (installs Nix, creates workspace, ready to build)",
	Long: `Set up a complete SparkRewards development environment in one command.

What it does:
  1. Creates the workspace directory (default: ~/SparkRewards)
  2. Drops flake.nix, setup.sh, .gitignore, README
  3. Installs Nix if not present
  4. Configures GitHub token for private repos
  5. Pre-warms the dev shell

After init, you can immediately:
  spark-cli dev          # enter dev environment
  spark-cli use InternalModel --build
  spark-cli build-all    # build everything

Examples:
  spark-cli init                    # uses ~/SparkRewards
  spark-cli init ~/my-workspace     # custom path
  spark-cli init --skip-setup       # drop files only, no Nix install`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		skipSetup, _ := cmd.Flags().GetBool("skip-setup")

		// Determine workspace path
		home, _ := os.UserHomeDir()
		wsPath := filepath.Join(home, "SparkRewards")
		if len(args) > 0 {
			wsPath = args[0]
			if wsPath[0] == '~' {
				wsPath = filepath.Join(home, wsPath[1:])
			}
		}

		fmt.Println("🔥 Initializing SparkRewards workspace")
		fmt.Printf("   Path: %s\n\n", wsPath)

		// Create workspace directory
		if err := os.MkdirAll(wsPath, 0755); err != nil {
			return fmt.Errorf("failed to create workspace: %w", err)
		}

		// Write embedded files
		filesToWrite := map[string]string{
			"flake.nix":        "flake.nix",
			"flake.lock":       "flake.lock",
			"setup.sh":         "setup.sh",
			"gitignore":        ".gitignore",
			"README.md":        "README.md",
			"buildspec-nix.yml": "buildspec-nix.yml",
		}

		for src, dst := range filesToWrite {
			dstPath := filepath.Join(wsPath, dst)

			// Don't overwrite existing files (user may have customized)
			if _, err := os.Stat(dstPath); err == nil {
				fmt.Printf("  ⏭  %s (already exists, skipping)\n", dst)
				continue
			}

			data, err := assets.FS.ReadFile(src)
			if err != nil {
				return fmt.Errorf("failed to read embedded %s: %w", src, err)
			}

			perm := os.FileMode(0644)
			if src == "setup.sh" {
				perm = 0755
			}

			if err := os.WriteFile(dstPath, data, perm); err != nil {
				return fmt.Errorf("failed to write %s: %w", dst, err)
			}
			fmt.Printf("  ✓  %s\n", dst)
		}

		fmt.Println()

		if skipSetup {
			fmt.Println("  Setup skipped (--skip-setup)")
			fmt.Println()
			fmt.Printf("  To complete setup later:\n")
			fmt.Printf("    cd %s && ./setup.sh\n", wsPath)
			return nil
		}

		// Run setup
		fmt.Println("  Running setup...")
		fmt.Println()

		if !nix.IsInstalled() {
			fmt.Println("  📦 Installing Nix...")
			installCmd := exec.Command("bash", "-c",
				`curl --proto '=https' --tlsv1.2 -sSf https://install.determinate.systems/nix | sh -s -- install --no-confirm`)
			installCmd.Stdout = os.Stdout
			installCmd.Stderr = os.Stderr
			if err := installCmd.Run(); err != nil {
				fmt.Println()
				fmt.Println("  ⚠  Nix install failed (may need sudo)")
				fmt.Printf("  Run manually: curl --proto '=https' --tlsv1.2 -sSf https://install.determinate.systems/nix | sh -s -- install\n")
				fmt.Println()
			}
		} else {
			fmt.Printf("  ✓  Nix already installed (%s)\n", nix.Version())
		}

		// Configure GitHub token
		token := nix.GetGitHubToken()
		if token != "" {
			if !nix.HasGitHubToken() {
				if err := nix.SetGitHubToken(token); err != nil {
					fmt.Printf("  ⚠  Failed to configure GitHub token: %v\n", err)
				} else {
					fmt.Println("  ✓  GitHub token configured for Nix")
				}
			} else {
				fmt.Println("  ✓  GitHub token already configured")
			}

			if !nix.HasNpmGitHubPackages() {
				if err := nix.SetNpmGitHubPackages(token); err != nil {
					fmt.Printf("  ⚠  Failed to configure npm registry: %v\n", err)
				} else {
					fmt.Println("  ✓  npm GitHub Packages registry configured")
				}
			} else {
				fmt.Println("  ✓  npm registry already configured")
			}
		} else {
			fmt.Println("  ⚠  No GitHub token found")
			fmt.Println("     Run: gh auth login")
			fmt.Println("     Then: spark-cli init (again)")
		}

		// Ensure flakes enabled
		if !nix.IsFlakesEnabled() {
			if err := nix.EnsureFlakesEnabled(); err != nil {
				fmt.Printf("  ⚠  Failed to enable flakes: %v\n", err)
			} else {
				fmt.Println("  ✓  Nix flakes enabled")
			}
		} else {
			fmt.Println("  ✓  Nix flakes already enabled")
		}

		// Pre-warm dev shell
		if nix.IsInstalled() {
			fmt.Println()
			fmt.Println("  Pre-warming dev shell (first time takes ~1 min)...")
			warmCmd := exec.Command(nix.NixBin, "develop", "--impure", "--command", "echo", "ready")
			warmCmd.Dir = wsPath
			warmCmd.Env = append(os.Environ(), "PATH="+nix.NixBinPath+":"+os.Getenv("PATH"))
			if err := warmCmd.Run(); err != nil {
				fmt.Println("  ⚠  Dev shell pre-warm failed (non-critical)")
			} else {
				fmt.Println("  ✓  Dev shell cached")
			}
		}

		fmt.Println()
		fmt.Println("  ✅ Workspace ready!")
		fmt.Println()
		fmt.Printf("  Next steps:\n")
		fmt.Printf("    cd %s\n", wsPath)
		fmt.Printf("    spark-cli dev              # enter dev environment\n")
		fmt.Printf("    spark-cli use InternalModel # clone a repo\n")
		fmt.Printf("    spark-cli build-all         # build everything\n")

		return nil
	},
}

func init() {
	initCmd.Flags().Bool("skip-setup", false, "Drop workspace files only, skip Nix install and configuration")
	initCmd.GroupID = "setup"
	rootCmd.AddCommand(initCmd)
}
