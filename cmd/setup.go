package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Spark-Rewards/homebrew-spark-cli/internal/nix"
	"github.com/spf13/cobra"
)

var setupSkipCache bool

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "First-time developer setup — install Nix, configure GitHub token, pre-warm dev shell (| -h)",
	Long: `One-shot setup for the SparkRewards Nix build system.

Idempotent — safe to run multiple times. Skips steps already completed.

Steps:
  1. Install Nix (Determinate Systems — fastest, most reliable)
  2. Enable Nix flakes in ~/.config/nix/nix.conf
  3. Fix shell PATH so 'nix' is available after login
  4. Configure GitHub token for private repo access
     • Auto-reads from 'gh auth token' if available
     • Writes to ~/.config/nix/nix.conf and ~/.npmrc
  5. Pre-warm the dev shell (downloads toolchain packages)
     • Node.js 22, Java 17, AWS CLI, Gradle — takes 3-10 min first time

After setup:
  spark-cli dev          # Enter the dev shell
  spark-cli build-all    # Build everything
  spark-cli doctor       # Verify everything looks correct

Flags:
  --skip-cache    Skip pre-warming the dev shell (faster, but first 'spark-cli dev' is slower)

Examples:
  spark-cli setup
  spark-cli setup --skip-cache`,
	RunE: func(cmd *cobra.Command, args []string) error {
		printSetupBanner()

		var stepErr error

		// ── Step 1: Nix ─────────────────────────────────────────────────────
		stepErr = setupStepNix()
		if stepErr != nil {
			return stepErr
		}

		// ── Step 2: Flakes ───────────────────────────────────────────────────
		setupStepFlakes()

		// ── Step 3: Shell PATH ───────────────────────────────────────────────
		setupStepShellPath()

		// ── Step 4: GitHub token ─────────────────────────────────────────────
		setupStepGitHubToken()

		// ── Step 5: Pre-warm devShell ────────────────────────────────────────
		setupStepPrewarm(setupSkipCache)

		// ── Done ─────────────────────────────────────────────────────────────
		printSetupDone()
		return nil
	},
}

// ── Step implementations ──────────────────────────────────────────────────────

func setupStepNix() error {
	printStep("1/5", "Nix installation")

	if nix.IsInstalled() {
		printSkip("Nix already installed — " + nix.Version())
		return nil
	}

	fmt.Println()
	printWarn("Nix is not installed. Installing via Determinate Systems...")
	printWarn("Requires sudo access. Takes ~2 minutes.")
	fmt.Println()
	fmt.Print("   Proceed? [Y/n] ")

	reader := bufio.NewReader(os.Stdin)
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(answer)
	if answer == "" {
		answer = "Y"
	}
	if answer != "Y" && answer != "y" {
		return fmt.Errorf("aborted — install Nix manually: https://install.determinate.systems/")
	}

	fmt.Println()
	fmt.Println("   Downloading and running Determinate Systems installer...")
	fmt.Println("   (This may ask for your sudo password)")
	fmt.Println()

	// The installer script handles OS detection automatically.
	installCmd := exec.Command("sh", "-c",
		`curl --proto '=https' --tlsv1.2 -sSf https://install.determinate.systems/nix | `+
			`sh -s -- install --no-confirm `+
			`--extra-conf "sandbox = false" `+
			`--extra-conf "experimental-features = nix-command flakes"`)
	installCmd.Stdin = os.Stdin
	installCmd.Stdout = os.Stdout
	installCmd.Stderr = os.Stderr
	if err := installCmd.Run(); err != nil {
		return fmt.Errorf("Nix installation failed: %w\n   Try manually: https://install.determinate.systems/", err)
	}

	if !nix.IsInstalled() {
		return fmt.Errorf("Nix binary not found after installation — please restart your terminal and re-run 'spark-cli setup'")
	}

	printOK("Nix installed — " + nix.Version())
	return nil
}

func setupStepFlakes() {
	printStep("2/5", "Nix flakes")

	if nix.IsFlakesEnabled() {
		printSkip("Flakes already enabled in ~/.config/nix/nix.conf")
		return
	}

	if err := nix.EnsureFlakesEnabled(); err != nil {
		printWarn("Could not write nix.conf: " + err.Error())
		return
	}
	printOK("experimental-features = nix-command flakes → ~/.config/nix/nix.conf")
}

// setupStepShellPath ensures the Nix daemon profile script is sourced in the
// user's shell startup files (~/.zshrc and ~/.zprofile), so that 'nix' is on
// PATH in every interactive and login shell without any extra manual steps.
func setupStepShellPath() {
	printStep("3/5", "Shell PATH (nix in PATH after login)")

	nixBlock := "# Nix — added by spark-cli setup\n" +
		"if [ -e '/nix/var/nix/profiles/default/etc/profile.d/nix-daemon.sh' ]; then\n" +
		"    . '/nix/var/nix/profiles/default/etc/profile.d/nix-daemon.sh'\n" +
		"fi\n"

	home, _ := os.UserHomeDir()
	patchedAny := false

	for _, rc := range []string{".zshrc", ".zprofile"} {
		path := filepath.Join(home, rc)
		content, _ := os.ReadFile(path)
		if strings.Contains(string(content), "nix-daemon.sh") {
			// Already has it — leave untouched
			continue
		}

		// Prepend the nix block so it runs before anything that might need nix
		existing := string(content)
		newContent := nixBlock + "\n" + existing
		if existing == "" {
			newContent = nixBlock
		}
		if err := os.WriteFile(path, []byte(newContent), 0644); err != nil {
			printWarn("Could not write " + rc + ": " + err.Error())
		} else {
			printOK("nix-daemon.sh sourced in ~/" + rc)
			patchedAny = true
		}
	}

	if !patchedAny {
		printSkip("nix-daemon.sh already sourced in shell profiles")
	}
}

func setupStepGitHubToken() {
	printStep("4/5", "GitHub token (private repos + npm packages)")

	if nix.HasGitHubToken() {
		printSkip("GitHub token already in ~/.config/nix/nix.conf")
		// Still make sure npm is configured even if nix.conf already has the token
		if !nix.HasNpmGitHubPackages() {
			if token := nix.GetGitHubToken(); token != "" {
				if err := nix.SetNpmGitHubPackages(token); err == nil {
					printOK("npm configured for GitHub Packages registry (→ ~/.npmrc)")
				}
			}
		}
		return
	}

	// Try to get token automatically
	token := nix.GetGitHubToken()

	if token != "" {
		fmt.Println("   → Using GitHub token from 'gh auth token'")
	} else {
		// Prompt the user
		fmt.Println()
		fmt.Println("   SparkRewards repos are private — a GitHub token is required.")
		fmt.Println("   Scopes needed: read:packages, repo (read)")
		fmt.Println("   Create one at: https://github.com/settings/tokens/new")
		fmt.Println("   Or run:        gh auth login")
		fmt.Println()
		fmt.Print("   GitHub token (Enter to skip): ")

		// Read without echoing (password-style)
		token = readTokenSilent()
		fmt.Println()
	}

	if token == "" {
		printWarn("Skipped — remote repo fetching and npm installs may fail")
		printWarn("Add manually: echo 'access-tokens = github.com=<TOKEN>' >> ~/.config/nix/nix.conf")
		return
	}

	// Write to nix.conf
	if err := nix.SetGitHubToken(token); err != nil {
		printWarn("Could not write to nix.conf: " + err.Error())
	} else {
		printOK("GitHub token → ~/.config/nix/nix.conf")
	}

	// Write to ~/.npmrc for @spark-rewards packages
	if nix.HasNpmGitHubPackages() {
		printSkip("npm GitHub Packages already in ~/.npmrc")
	} else {
		if err := nix.SetNpmGitHubPackages(token); err != nil {
			printWarn("Could not configure ~/.npmrc: " + err.Error())
		} else {
			printOK("@spark-rewards registry → ~/.npmrc")
		}
	}
}

func setupStepPrewarm(skip bool) {
	printStep("5/5", "Pre-warming dev shell")

	if skip {
		printSkip("Skipped (--skip-cache)")
		return
	}

	ws, err := nix.FindWorkspace()
	if err != nil {
		printWarn("Workspace not found — skipping pre-warm: " + err.Error())
		return
	}

	fmt.Println("   Downloading Node 22, Java 17, AWS CLI, Gradle...")
	fmt.Println("   This takes 3–10 minutes on first run, then is instant. ☕")
	fmt.Println()

	if err := nix.Run(ws, "develop", "--impure", "--command", "echo", "→ Dev shell ready ✓"); err != nil {
		printWarn("Pre-warm failed: " + err.Error())
		printWarn("Run 'spark-cli dev' later to download on first use")
		return
	}

	printOK("Dev environment cached")
}

// ── Print helpers ─────────────────────────────────────────────────────────────

func printSetupBanner() {
	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════╗")
	fmt.Println("║   🔥 SparkRewards Developer Setup                ║")
	fmt.Println("╚══════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Println("  Steps:")
	fmt.Println("  1. Install Nix (Determinate Systems)")
	fmt.Println("  2. Enable Nix flakes")
	fmt.Println("  3. Fix shell PATH (nix in PATH after login)")
	fmt.Println("  4. Configure GitHub token (private repos + npm)")
	fmt.Println("  5. Pre-warm dev environment (Node 22, Java 17, AWS CLI...)")
	fmt.Println()
}

func printSetupDone() {
	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════╗")
	fmt.Println("║   ✅ Setup Complete!                              ║")
	fmt.Println("╚══════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Println("  Next steps:")
	fmt.Println()
	fmt.Println("    spark-cli dev                           # Enter dev shell")
	fmt.Println("    spark-cli status                        # See local vs remote repos")
	fmt.Println("    spark-cli use InternalAPILambda         # Clone a repo to work on")
	fmt.Println("    spark-cli build-all                     # Build everything")
	fmt.Println()
	fmt.Println("  Run 'spark-cli --help' for all commands.")
	fmt.Println()
}

func printStep(num, label string) {
	fmt.Printf("\n── Step %s: %s\n", num, label)
}

func printOK(msg string) {
	fmt.Println("   ✓  " + msg)
}

func printSkip(msg string) {
	fmt.Println("   ⊘  " + msg + " (skipped)")
}

func printWarn(msg string) {
	fmt.Println("   ⚠  " + msg)
}

// readTokenSilent reads a line from stdin. On terminals, it disables echo
// via stty if available (best-effort — falls back to plain ReadString).
func readTokenSilent() string {
	// Try to disable echo via stty (unix only, best-effort)
	_ = exec.Command("stty", "-echo").Run()
	defer func() { _ = exec.Command("stty", "echo").Run() }()

	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	return strings.TrimSpace(line)
}

// ensureSparkCliInstalled creates a symlink spark-cli → /usr/local/bin if not present.
// This is a no-op when installed via Homebrew.
func ensureSparkCliInstalled() {
	selfPath, err := filepath.Abs(os.Args[0])
	if err != nil {
		return
	}

	target := "/usr/local/bin/spark-cli"
	if _, err := os.Stat(target); err == nil {
		return // already installed
	}

	if err := os.Symlink(selfPath, target); err != nil {
		// Try with sudo
		_ = exec.Command("sudo", "ln", "-sf", selfPath, target).Run()
	}
}

func init() {
	setupCmd.Flags().BoolVar(&setupSkipCache, "skip-cache", false,
		"Skip pre-warming the dev shell (faster; first 'spark-cli dev' will download instead)")
	setupCmd.GroupID = "setup"
	rootCmd.AddCommand(setupCmd)
}
