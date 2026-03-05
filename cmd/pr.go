package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/Spark-Rewards/homebrew-spark-cli/internal/git"
	"github.com/spf13/cobra"
)

var (
	prBase  string
	prDraft bool
	prOpen  bool
	prTitle string
)

var prCmd = &cobra.Command{
	Use:   "pr",
	Short: "Create a PR from the current branch (push + gh pr create | -h)",
	Long: `Push your current branch and open a pull request on GitHub.

Reads the repo from the current working directory. Follows SparkRewards
PR conventions: base branch is flame-develop, template includes
Description / Reason / Testing sections.

Interactive by default — pre-fills title/body so you just review and submit.
Use --draft to open a draft PR (e.g. work in progress).

Examples:
  spark-cli pr                            # interactive PR creation
  spark-cli pr --title "fix: typo in docs"
  spark-cli pr --draft                    # draft PR
  spark-cli pr --base main                # target a different base branch`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Locate git repo from cwd
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("could not determine current directory: %w", err)
		}

		repoRoot, err := findGitRoot(cwd)
		if err != nil {
			return fmt.Errorf("not inside a git repository — run this from a repo directory\n   e.g. cd ~/SparkRewards/InternalAPILambda && spark-cli pr")
		}

		// Get current branch
		branch, err := git.CurrentBranch(repoRoot)
		if err != nil {
			return fmt.Errorf("could not determine current branch: %w", err)
		}

		// Safety checks
		protected := []string{"main", "master", "flame-develop", "develop"}
		for _, p := range protected {
			if branch == p {
				return fmt.Errorf("branch '%s' is protected — create a feature branch first:\n   git checkout -b feature/my-change", branch)
			}
		}

		repoName := gitRepoName(repoRoot)

		fmt.Printf("🔥 Creating PR\n\n")
		fmt.Printf("   Repo:   %s\n", repoName)
		fmt.Printf("   Branch: %s  →  %s\n", branch, prBase)
		fmt.Println()

		// Push branch
		fmt.Printf("   Pushing %s to origin...\n", branch)
		pushCmd := exec.Command("git", "push", "-u", "origin", branch)
		pushCmd.Dir = repoRoot
		pushCmd.Stdout = os.Stdout
		pushCmd.Stderr = os.Stderr
		if err := pushCmd.Run(); err != nil {
			return fmt.Errorf("git push failed: %w\n   Make sure you have write access to this repo", err)
		}
		fmt.Println()

		// Build gh pr create args
		ghArgs := []string{"pr", "create",
			"--base", prBase,
		}

		if prTitle != "" {
			ghArgs = append(ghArgs, "--title", prTitle)
		}

		if prDraft {
			ghArgs = append(ghArgs, "--draft")
		}

		// Pre-fill the PR body with SparkRewards template
		if prTitle == "" {
			// Interactive: use --fill for title suggestion, but still prompt body
			prBody := prTemplate(branch)
			ghArgs = append(ghArgs, "--body", prBody)
		} else {
			prBody := prTemplate(branch)
			ghArgs = append(ghArgs, "--body", prBody)
		}

		fmt.Println("   Opening PR creation...")
		fmt.Println()

		ghCmd := exec.Command("gh", ghArgs...)
		ghCmd.Dir = repoRoot
		ghCmd.Stdin = os.Stdin
		ghCmd.Stdout = os.Stdout
		ghCmd.Stderr = os.Stderr
		if err := ghCmd.Run(); err != nil {
			return fmt.Errorf("gh pr create failed: %w\n   Ensure 'gh auth login' has been run", err)
		}

		return nil
	},
}

// prTemplate returns a pre-filled SparkRewards PR body for the given branch.
func prTemplate(branch string) string {
	// Try to infer a description hint from the branch name
	hint := branch
	for _, prefix := range []string{"feature/", "feat/", "fix/", "chore/", "refactor/", "docs/"} {
		if strings.HasPrefix(hint, prefix) {
			hint = strings.TrimPrefix(hint, prefix)
			break
		}
	}
	hint = strings.ReplaceAll(hint, "-", " ")
	hint = strings.ReplaceAll(hint, "_", " ")

	return fmt.Sprintf(`## Description
%s

## Reason
<!-- Why is this change needed? What problem does it solve? -->

## Testing
<!-- How was this tested? (local build, beta deploy, unit tests, etc.) -->
- [ ] spark-cli build passes
- [ ] Tested locally
- [ ] Tested on beta (if applicable)
`, hint)
}

// findGitRoot walks up from dir to find the git repo root.
func findGitRoot(dir string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("not a git repo")
	}
	return strings.TrimSpace(string(out)), nil
}

// gitRepoName returns the repo name from the git remote URL or the directory name.
func gitRepoName(repoRoot string) string {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		// Fallback: use directory name
		parts := strings.Split(repoRoot, "/")
		return parts[len(parts)-1]
	}
	remote := strings.TrimSpace(string(out))
	// Extract repo name from URL: git@github.com:Org/Repo.git or https://github.com/Org/Repo
	remote = strings.TrimSuffix(remote, ".git")
	parts := strings.Split(remote, "/")
	return parts[len(parts)-1]
}

func init() {
	prCmd.Flags().StringVarP(&prBase, "base", "b", "flame-develop", "Base branch for the PR")
	prCmd.Flags().StringVarP(&prTitle, "title", "t", "", "PR title (prompted interactively if not set)")
	prCmd.Flags().BoolVar(&prDraft, "draft", false, "Open as a draft PR")
	prCmd.GroupID = "dev"
	rootCmd.AddCommand(prCmd)
}
