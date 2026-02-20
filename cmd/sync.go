package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Spark-Rewards/homebrew-spk/internal/aws"
	"github.com/Spark-Rewards/homebrew-spk/internal/git"
	"github.com/Spark-Rewards/homebrew-spk/internal/github"
	"github.com/Spark-Rewards/homebrew-spk/internal/workspace"
	"github.com/spf13/cobra"
)

var (
	syncBranch   string
	syncNoRebase bool
	syncNoEnv    bool
	syncEnv      string
)

var syncCmd = &cobra.Command{
	Use:   "sync [repo-name]",
	Short: "Sync repos and refresh environment",
	Long: `Syncs all workspace repos (git fetch + rebase) and refreshes the .env file
with fresh credentials from AWS SSM. Automatically logs in to AWS if needed.

When run without arguments, syncs all repos and refreshes .env.
When a repo name is provided, only syncs that specific repo.

Examples:
  spk sync                    # sync all repos + refresh .env
  spk sync BusinessAPI        # sync specific repo only
  spk sync --no-env           # skip .env refresh
  spk sync --env prod         # use prod environment for .env`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		wsPath, err := workspace.Find()
		if err != nil {
			return err
		}

		ws, err := workspace.Load(wsPath)
		if err != nil {
			return err
		}

		if len(args) == 1 {
			return syncRepo(wsPath, ws, args[0])
		}

		if err := syncAllRepos(wsPath, ws); err != nil {
			return err
		}

		if !syncNoEnv {
			fmt.Println("\n--- Refreshing Environment ---")
			if err := refreshEnv(wsPath, ws); err != nil {
				fmt.Printf("Warning: failed to refresh .env: %v\n", err)
			}
		}

		fmt.Println("\n--- Updating Workspace Files ---")
		if err := workspace.GenerateVSCodeWorkspace(wsPath); err != nil {
			fmt.Printf("Warning: failed to update VS Code workspace: %v\n", err)
		} else {
			fmt.Printf("Updated %s\n", workspace.VSCodeWorkspacePath(wsPath))
		}

		return nil
	},
}

// SSM parameter suffixes to fetch — mirrors sync.sh + business website (bizz-website)
var ssmParamSuffixes = []string{
	"customerUserPoolId",
	"customerWebClientId",
	"identityPoolIdCustomer",
	"businessUserPoolId",
	"businessWebClientId",
	"identityPoolIdBusiness",
	"squareClientId",
	"cloverAppId",
	"appConfig",
	"googleApiKey_Android",
	"googleMapsKey",
	"githubToken",
	"stripePublicKey",
}

// Maps SSM param suffix → .env key name
var ssmToEnvKey = map[string]string{
	"customerUserPoolId":      "USERPOOL_ID",
	"customerWebClientId":     "WEB_CLIENT_ID",
	"identityPoolIdCustomer":  "IDENTITY_POOL_ID",
	"businessUserPoolId":      "BUSINESS_USERPOOL_ID",
	"businessWebClientId":     "BUSINESS_WEB_CLIENT_ID",
	"identityPoolIdBusiness":  "BUSINESS_IDENTITY_POOL_ID",
	"squareClientId":          "SQUARE_CLIENT_ID",
	"cloverAppId":             "CLOVER_APP_ID",
	"appConfig":               "APP_CONFIG_VALUES",
	"googleApiKey_Android":    "GOOGLE_API_KEY_ANDROID",
	"googleMapsKey":           "GOOGLE_MAPS_KEY",
	"githubToken":             "GITHUB_TOKEN",
	"stripePublicKey":         "STRIPE_PUBLIC_KEY",
}

func refreshEnv(wsPath string, ws *workspace.Workspace) error {
	if err := aws.CheckCLI(); err != nil {
		return err
	}

	profile := ws.AWSProfile
	region := ws.AWSRegion
	if region == "" {
		region = "us-east-1"
	}

	env := syncEnv
	if env == "" && ws.SSMEnvPath != "" {
		env = ws.SSMEnvPath
	}
	if env == "" {
		env = "beta"
	}

	fmt.Printf("Checking AWS credentials (profile: %s)...\n", orDefault(profile, "default"))
	if err := aws.GetCallerIdentity(profile); err != nil {
		fmt.Println("AWS session expired, logging in...")
		if err := aws.SSOLogin(profile); err != nil {
			return fmt.Errorf("AWS login failed: %w", err)
		}
	}

	fmt.Printf("Fetching environment from /app/%s/... (%d parameters)\n", env, len(ssmParamSuffixes))
	ssmVars, err := github.FetchMultipleFromSSM(profile, env, region, ssmParamSuffixes)
	if err != nil {
		return fmt.Errorf("failed to fetch parameters: %w", err)
	}

	// Map SSM keys to .env keys
	envVars := make(map[string]string)
	for ssmKey, value := range ssmVars {
		if envKey, ok := ssmToEnvKey[ssmKey]; ok {
			envVars[envKey] = value
		} else {
			envVars[ssmKey] = value
		}
	}

	// Business Website (Next.js) needs NEXT_PUBLIC_* from business SSM params (bizz-website)
	if v, ok := envVars["BUSINESS_USERPOOL_ID"]; ok && v != "" {
		envVars["NEXT_PUBLIC_USERPOOL_ID"] = v
	}
	if v, ok := envVars["BUSINESS_WEB_CLIENT_ID"]; ok && v != "" {
		envVars["NEXT_PUBLIC_WEB_CLIENT_ID"] = v
	}
	if v, ok := envVars["BUSINESS_IDENTITY_POOL_ID"]; ok && v != "" {
		envVars["NEXT_PUBLIC_IDENTITY_POOL_ID"] = v
	}
	// Fallback to customer params if business not set (e.g. older SSM)
	if envVars["NEXT_PUBLIC_USERPOOL_ID"] == "" {
		if v, ok := envVars["USERPOOL_ID"]; ok && v != "" {
			envVars["NEXT_PUBLIC_USERPOOL_ID"] = v
		}
	}
	if envVars["NEXT_PUBLIC_WEB_CLIENT_ID"] == "" {
		if v, ok := envVars["WEB_CLIENT_ID"]; ok && v != "" {
			envVars["NEXT_PUBLIC_WEB_CLIENT_ID"] = v
		}
	}
	if envVars["NEXT_PUBLIC_IDENTITY_POOL_ID"] == "" {
		if v, ok := envVars["IDENTITY_POOL_ID"]; ok && v != "" {
			envVars["NEXT_PUBLIC_IDENTITY_POOL_ID"] = v
		}
	}

	// Business Website: Square and Clover from SSM
	if v, ok := envVars["SQUARE_CLIENT_ID"]; ok && v != "" {
		envVars["NEXT_PUBLIC_SQUARE_CLIENT"] = v
	}
	if v, ok := envVars["CLOVER_APP_ID"]; ok && v != "" {
		envVars["NEXT_PUBLIC_CLOVER_APP_ID"] = v
	}
	if v, ok := envVars["GOOGLE_MAPS_KEY"]; ok && v != "" {
		envVars["NEXT_PUBLIC_GOOGLE_MAPS_API_KEY"] = v
	}
	if v, ok := envVars["STRIPE_PUBLIC_KEY"]; ok && v != "" {
		envVars["NEXT_PUBLIC_STRIPE_KEY"] = v
	}

	// Add static env vars
	envVars["AWS_REGION"] = region
	envVars["NEXT_PUBLIC_AWS_REGION"] = region
	envVars["APP_ENV"] = env
	if env != "" {
		envVars["NEXT_PUBLIC_APP_ENV"] = env
	}
	envVars["NEXT_PUBLIC_API_DOMAIN"] = fmt.Sprintf("api-%s.sparkrewards.com", env)

	// Merge workspace env vars
	for k, v := range ws.Env {
		envVars[k] = v
	}

	if err := workspace.WriteGlobalEnv(wsPath, envVars); err != nil {
		return err
	}

	fmt.Printf("Updated %s (%d variables)\n", workspace.GlobalEnvPath(wsPath), len(envVars))
	return nil
}

func getTargetBranch(ws *workspace.Workspace, repo *workspace.RepoDef, repoDir string) string {
	if syncBranch != "" {
		return syncBranch
	}
	if repo != nil && repo.DefaultBranch != "" {
		return repo.DefaultBranch
	}
	if ws.DefaultBranch != "" {
		return ws.DefaultBranch
	}
	return git.GetDefaultBranch(repoDir)
}

func syncRepo(wsPath string, ws *workspace.Workspace, name string) error {
	repo, ok := ws.Repos[name]
	if !ok {
		return fmt.Errorf("repo '%s' not found — run 'spk list' to see repos", name)
	}

	repoDir := filepath.Join(wsPath, repo.Path)
	if _, err := os.Stat(repoDir); os.IsNotExist(err) {
		return fmt.Errorf("repo directory missing — run 'spk use %s'", name)
	}

	return syncRepoInternal(wsPath, ws, name, repo, repoDir)
}

func syncAllRepos(wsPath string, ws *workspace.Workspace) error {
	if len(ws.Repos) == 0 {
		fmt.Println("No repos in workspace — run 'spk use <repo>' to add one")
		return nil
	}

	fmt.Println("--- Syncing Repositories ---")
	var errors []string
	var synced int

	for name, repo := range ws.Repos {
		repoDir := filepath.Join(wsPath, repo.Path)
		if _, err := os.Stat(repoDir); os.IsNotExist(err) {
			fmt.Printf("[skip] %s (not cloned)\n", name)
			continue
		}

		if err := syncRepoInternal(wsPath, ws, name, repo, repoDir); err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", name, err))
			fmt.Printf("[fail] %s\n", name)
		} else {
			fmt.Printf("[ok]   %s\n", name)
			synced++
		}
	}

	if len(errors) > 0 {
		fmt.Printf("\n%d repo(s) failed:\n", len(errors))
		for _, e := range errors {
			fmt.Printf("  - %s\n", e)
		}
	}

	fmt.Printf("\n%d repo(s) synced\n", synced)
	return nil
}

func syncRepoInternal(wsPath string, ws *workspace.Workspace, name string, repo workspace.RepoDef, repoDir string) error {
	targetBranch := getTargetBranch(ws, &repo, repoDir)

	if syncNoRebase {
		return git.Pull(repoDir)
	}

	isDirty := git.IsDirty(repoDir)
	if isDirty {
		if err := git.Stash(repoDir); err != nil {
			return fmt.Errorf("stash failed: %w", err)
		}
	}

	if err := git.Fetch(repoDir, "origin"); err != nil {
		if isDirty {
			git.StashPop(repoDir)
		}
		return fmt.Errorf("fetch failed: %w", err)
	}

	upstream := fmt.Sprintf("origin/%s", targetBranch)
	if err := git.Rebase(repoDir, upstream); err != nil {
		git.RebaseAbort(repoDir)
		if isDirty {
			git.StashPop(repoDir)
		}
		return fmt.Errorf("rebase onto %s failed", upstream)
	}

	if isDirty {
		git.StashPop(repoDir)
	}

	return nil
}

func init() {
	syncCmd.Flags().StringVar(&syncBranch, "branch", "", "Target branch (default: main)")
	syncCmd.Flags().BoolVar(&syncNoRebase, "no-rebase", false, "Use git pull instead of rebase")
	syncCmd.Flags().BoolVar(&syncNoEnv, "no-env", false, "Skip .env refresh")
	syncCmd.Flags().StringVar(&syncEnv, "env", "", "SSM environment (beta/prod)")
	rootCmd.AddCommand(syncCmd)
}
