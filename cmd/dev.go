package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/Spark-Rewards/homebrew-spark-cli/internal/nix"
	"github.com/spf13/cobra"
)

var devCmd = &cobra.Command{
	Use:   "dev",
	Short: "Enter the Nix dev shell (Node 22, Java 17, AWS CLI, Gradle | -h)",
	Long: `Enter the SparkRewards Nix development shell.

The dev shell provides a pinned, reproducible toolchain:
  Node.js 22    TypeScript services and tooling
  Java 17       Gradle / Smithy codegen
  AWS CLI 2     Deployments
  Gradle 8      InternalModel build system
  git           Version control
  jq            JSON processing

Guaranteed identical on every developer machine and in CI.
Exit the shell with Ctrl+D or 'exit'.

Inside the dev shell:
  # Run a Lambda in watch mode (hot-reload on TypeScript changes):
  cd ~/SparkRewards/InternalAPILambda && npm run watch

  # Start a website client with hot-reload:
  cd ~/SparkRewards/InternalWebsiteClient && npm start
  cd ~/SparkRewards/BusinessWebsite       && npm start

  # Build the Smithy model (generates SDK):
  cd ~/SparkRewards/InternalModel && ./gradlew build

  # Deploy to beta (from inside dev shell):
  spark-cli cdk --profile pipeline deploy PipelineStack/beta/InternalServiceStack

Examples:
  spark-cli dev`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ws, err := nix.FindWorkspace()
		if err != nil {
			return err
		}

		// Detect which repos are cloned locally to give targeted hints
		tips := devTips(ws)

		fmt.Println("🔥 Entering SparkRewards dev shell")
		fmt.Printf("   Workspace: %s\n", ws)
		fmt.Println("   Toolchain: Node.js 22 · Java 17 · AWS CLI · Gradle · git")
		fmt.Println("   Exit:      Ctrl+D or 'exit'")
		if len(tips) > 0 {
			fmt.Println()
			fmt.Println("   Quick commands (your local repos):")
			for _, tip := range tips {
				fmt.Printf("     %s\n", tip)
			}
		}
		fmt.Println()

		c := exec.Command(nix.NixBin, "develop", "--impure")
		c.Dir = ws
		c.Stdin = os.Stdin
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		c.Env = nixEnvWithToken()

		if err := c.Run(); err != nil {
			if exit, ok := err.(*exec.ExitError); ok {
				os.Exit(exit.ExitCode())
			}
			return fmt.Errorf("nix develop failed: %w", err)
		}
		return nil
	},
}

// nixEnvWithToken returns os.Environ() with NixBinPath prepended and GITHUB_TOKEN injected.
func nixEnvWithToken() []string {
	env := os.Environ()

	// Prepend Nix bin path
	for i, e := range env {
		if strings.HasPrefix(e, "PATH=") {
			env[i] = "PATH=" + nix.NixBinPath + ":" + e[len("PATH="):]
			break
		}
	}

	// Inject GITHUB_TOKEN if not already set
	hasToken := false
	for _, e := range env {
		if strings.HasPrefix(e, "GITHUB_TOKEN=") {
			hasToken = true
			break
		}
	}
	if !hasToken {
		if token := nix.GetGitHubToken(); token != "" {
			env = append(env, "GITHUB_TOKEN="+token)
		}
	}

	return env
}

// devTips returns contextual quick-command hints based on which repos are cloned.
func devTips(ws string) []string {
	type repoTip struct {
		repo string
		cmd  string
	}
	hotReload := []repoTip{
		{"InternalWebsiteClient", "cd InternalWebsiteClient && npm start        # hot-reload website"},
		{"BusinessWebsite", "cd BusinessWebsite       && npm start        # hot-reload website"},
		{"InternalAPILambda", "cd InternalAPILambda     && npm run watch     # TypeScript watch mode"},
		{"BusinessAPILambda", "cd BusinessAPILambda     && npm run watch     # TypeScript watch mode"},
		{"AppAPILambda", "cd AppAPILambda          && npm run watch     # TypeScript watch mode"},
		{"InternalModel", "cd InternalModel         && ./gradlew build   # build Smithy SDK"},
	}

	var tips []string
	for _, t := range hotReload {
		if isGitDir(ws + "/" + t.repo) {
			tips = append(tips, t.cmd)
		}
	}
	return tips
}

func init() {
	devCmd.GroupID = "dev"
	rootCmd.AddCommand(devCmd)
}
