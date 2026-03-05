package cmd

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/Spark-Rewards/homebrew-spark-cli/internal/nix"
	"github.com/spf13/cobra"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check Nix dev environment health (Nix, flakes, token, repos | -h)",
	Long: `Diagnose your SparkRewards Nix development environment.

Checks:
  1. Nix installed
  2. Nix flakes enabled
  3. GitHub token configured (for private repo access)
  4. GitHub reachable (SSH or gh CLI)
  5. Nix-tracked repos (local or remote)
  6. npm GitHub Packages registry configured

Prints a clear pass/warn/fail for each check and tells you exactly how to fix issues.

Examples:
  spark-cli doctor`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("🔥 SparkRewards Nix Doctor")
		fmt.Println("═══════════════════════════════════════════════════════")
		fmt.Println("   Checking your development environment...")
		fmt.Println()

		allOK := true

		// ── Check 1: Nix installed ──────────────────────────────────────────
		fmt.Println("   [1/6] Nix installation")
		if nix.IsInstalled() {
			fmt.Printf("   ✓  %s\n", nix.Version())
		} else {
			fmt.Println("   ✗  Nix not found at " + nix.NixBin)
			fmt.Println("      Fix: spark-cli setup")
			allOK = false
		}

		// ── Check 2: Flakes enabled ─────────────────────────────────────────
		fmt.Println()
		fmt.Println("   [2/6] Nix flakes enabled")
		if nix.IsFlakesEnabled() {
			fmt.Println("   ✓  experimental-features = nix-command flakes")
		} else {
			fmt.Println("   ✗  Flakes not enabled in ~/.config/nix/nix.conf")
			fmt.Println("      Fix: spark-cli setup")
			allOK = false
		}

		// ── Check 3: GitHub token in nix.conf ──────────────────────────────
		fmt.Println()
		fmt.Println("   [3/6] GitHub token (private repo access)")
		if nix.HasGitHubToken() {
			fmt.Println("   ✓  access-tokens = github.com=*** (configured in ~/.config/nix/nix.conf)")
		} else if t := nix.GetGitHubToken(); t != "" {
			fmt.Println("   ⚠  GITHUB_TOKEN available but not persisted to nix.conf")
			fmt.Println("      Builds work now but won't after shell restart")
			fmt.Println("      Fix: spark-cli setup  (will persist the token)")
		} else {
			fmt.Println("   ✗  GitHub token not configured")
			fmt.Println("      Fix: spark-cli setup  (will prompt for token)")
			allOK = false
		}

		// ── Check 4: GitHub access ──────────────────────────────────────────
		fmt.Println()
		fmt.Println("   [4/6] GitHub repo access")
		ghAccess := checkGitHubAccess()
		switch ghAccess {
		case "ssh":
			fmt.Println("   ✓  SSH access to GitHub confirmed")
		case "gh":
			fmt.Println("   ✓  gh CLI authenticated (HTTPS access confirmed)")
		default:
			fmt.Println("   ⚠  Could not verify GitHub access")
			fmt.Println("      Check: ssh -T git@github.com")
			fmt.Println("      Check: gh auth status")
		}

		// ── Check 5: Nix-tracked repos ──────────────────────────────────────
		fmt.Println()
		fmt.Println("   [5/6] Nix-tracked repos")

		ws, wsErr := nix.FindWorkspace()
		if wsErr != nil {
			fmt.Println("   ✗  Workspace not found:", wsErr)
			allOK = false
		} else {
			localCount := 0
			remoteCount := 0
			for _, repo := range nix.TrackedRepos {
				repoPath := ws + "/" + repo
				if isGitDir(repoPath) {
					fmt.Printf("   ✓  %-30s 📁 local\n", repo)
					localCount++
				} else {
					fmt.Printf("   →  %-30s 🌐 GitHub (will fetch remotely)\n", repo)
					remoteCount++
				}
			}
			if remoteCount > 0 {
				fmt.Printf("      %d repo(s) will be fetched from GitHub during builds\n", remoteCount)
			}
		}

		// ── Check 6: npm GitHub Packages ───────────────────────────────────
		fmt.Println()
		fmt.Println("   [6/6] npm GitHub Packages registry")
		if nix.HasNpmGitHubPackages() {
			fmt.Println("   ✓  @spark-rewards registry configured in ~/.npmrc")
		} else if wsErr == nil {
			// Check if any local .npmrc exists in tracked repos
			hasLocalNpmrc := false
			for _, repo := range nix.TrackedRepos {
				if fileExistsCheck(ws + "/" + repo + "/.npmrc") {
					hasLocalNpmrc = true
					break
				}
			}
			if hasLocalNpmrc {
				fmt.Println("   ⚠  Local .npmrc found in repo(s) but not global ~/.npmrc")
				fmt.Println("      Builds with local clones work, but fresh installs may fail")
				fmt.Println("      Fix: spark-cli setup  (configures global ~/.npmrc)")
			} else {
				fmt.Println("   ⚠  npm not configured for GitHub Packages")
				fmt.Println("      Fix: spark-cli setup")
			}
		} else {
			fmt.Println("   ⚠  npm not configured for GitHub Packages")
			fmt.Println("      Fix: spark-cli setup")
		}

		// ── Summary ─────────────────────────────────────────────────────────
		fmt.Println()
		fmt.Println("═══════════════════════════════════════════════════════")
		if allOK {
			fmt.Println()
			fmt.Println("   ✓  All checks passed — you're ready to build!")
			fmt.Println()
			fmt.Println("   spark-cli dev         # Enter dev shell")
			fmt.Println("   spark-cli build-all   # Build everything")
		} else {
			fmt.Println()
			fmt.Println("   Some checks failed. Run 'spark-cli setup' to fix automatically.")
			fmt.Println()
			fmt.Println("   spark-cli setup")
		}
		fmt.Println()
		return nil
	},
}

// checkGitHubAccess tests GitHub connectivity.
// Returns "ssh", "gh", or "none".
func checkGitHubAccess() string {
	// Try SSH
	out, _ := exec.Command("ssh", "-T", "-o", "StrictHostKeyChecking=no",
		"-o", "ConnectTimeout=5", "git@github.com").CombinedOutput()
	if strings.Contains(string(out), "successfully authenticated") {
		return "ssh"
	}

	// Try gh CLI
	if err := exec.Command("gh", "auth", "status").Run(); err == nil {
		return "gh"
	}

	return "none"
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}
