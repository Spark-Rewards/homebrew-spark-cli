package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/Spark-Rewards/homebrew-spark-cli/internal/nix"
	"github.com/spf13/cobra"
)

var nixStatusCmd = &cobra.Command{
	Use:     "status",
	Short:   "Show Nix workspace status — which repos are local vs GitHub (| -h)",
	Aliases: []string{"st"},
	Long: `Show the current state of Nix-tracked repos in the workspace.

For each repo, shows whether Nix will use your local clone (📁) or
fetch the repo from GitHub at the pinned flake.lock revision (🌐).

Brazil analogy: like 'brazil ws show' — but for Nix inputs.

Examples:
  spark-cli status`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ws, err := nix.FindWorkspace()
		if err != nil {
			return err
		}

		fmt.Println("🔥 SparkRewards Nix Workspace Status")
		fmt.Println("═══════════════════════════════════════════════════════")
		fmt.Printf("   Workspace:  %s\n", ws)
		fmt.Printf("   GitHub Org: %s\n\n", nix.GitHubOrg)

		// ── Nix-tracked repos ─────────────────────────────────────────────────
		fmt.Println("   📦 Nix-tracked packages:")
		fmt.Printf("   %-30s %-25s %s\n", "REPO", "NIX PACKAGE", "STATUS")
		fmt.Printf("   %-30s %-25s %s\n",
			"─────────────────────────────",
			"────────────────────────",
			"─────────────────────────")

		for _, repo := range nix.TrackedRepos {
			pkg := nix.RepoToPackage(repo)
			repoPath := ws + "/" + repo

			if isGitDir(repoPath) {
				branch := gitBranch(repoPath)
				dirty := ""
				if isGitDirty(repoPath) {
					dirty = " (modified)"
				}
				fmt.Printf("   %-30s %-25s 📁 local [%s]%s\n", repo, pkg, branch, dirty)
			} else {
				fmt.Printf("   %-30s %-25s 🌐 GitHub (main)\n", repo, pkg)
			}
		}

		// ── Other workspace repos ─────────────────────────────────────────────
		fmt.Println()
		fmt.Println("   📂 Other workspace repos:")
		anyOther := false
		for _, repo := range nix.AllRepos {
			if nix.IsTracked(repo) {
				continue
			}
			anyOther = true
			repoPath := ws + "/" + repo
			if isGitDir(repoPath) {
				branch := gitBranch(repoPath)
				fmt.Printf("   %-30s 📁 local [%s]\n", repo, branch)
			} else {
				fmt.Printf("   %-30s (not cloned)\n", repo)
			}
		}
		if !anyOther {
			fmt.Println("   (none)")
		}

		fmt.Println()
		fmt.Println("   Legend:")
		fmt.Println("   📁 local  — Nix uses your local clone (edits picked up immediately)")
		fmt.Println("   🌐 GitHub — Nix fetches pinned commit from GitHub (flake.lock)")
		fmt.Println()
		fmt.Println("   Tip: spark-cli use <repo>   to clone for local editing")
		fmt.Println("        spark-cli build-all     to build everything")
		return nil
	},
}

// isGitDirty reports whether the working tree has uncommitted changes.
func isGitDirty(path string) bool {
	cmd := exec.Command("git", "-C", path, "diff", "--quiet")
	return cmd.Run() != nil
}

// runGit runs a git command in the given directory and returns trimmed stdout.
func runGit(path string, args ...string) (string, error) {
	fullArgs := append([]string{"-C", path}, args...)
	out, err := exec.Command("git", fullArgs...).Output()
	return strings.TrimSpace(string(out)), err
}

// isLocalNpmrc returns true if the global ~/.npmrc exists and is non-empty.
func isLocalNpmrc() bool {
	home, _ := os.UserHomeDir()
	info, err := os.Stat(home + "/.npmrc")
	return err == nil && info.Size() > 0
}

func init() {
	nixStatusCmd.GroupID = "dev"
	rootCmd.AddCommand(nixStatusCmd)
}
