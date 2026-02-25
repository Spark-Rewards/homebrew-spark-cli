package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/Spark-Rewards/homebrew-spark-cli/internal/aws"
	"github.com/Spark-Rewards/homebrew-spark-cli/internal/git"
	"github.com/Spark-Rewards/homebrew-spark-cli/internal/github"
	"github.com/Spark-Rewards/homebrew-spark-cli/internal/workspace"
	"github.com/spf13/cobra"
)

var (
	syncBranch   string
	syncNoRebase bool
	syncEnv      string
	syncInstall  bool
	syncUpdate   bool
)

var syncCmd = &cobra.Command{
	Use:   "sync [repo-name]",
	Short: "Sync repos (git fetch+rebase); use --env to refresh workspace .env",
	Long: `Syncs workspace repos with parallel fetches and rebases all local branches.

  spark-cli workspace sync                # sync all repos (parallel)
  spark-cli workspace sync --install      # sync + npm install where package-lock changed
  spark-cli workspace sync --env beta     # sync and refresh .env from beta
  spark-cli workspace sync BusinessAPI    # sync one repo`,
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
			if err := syncRepo(wsPath, ws, args[0]); err != nil {
				return err
			}
		} else {
			if err := syncAllRepos(wsPath, ws); err != nil {
				return err
			}
		}

		if syncEnv != "" {
			if err := refreshEnvQuiet(wsPath, ws); err != nil {
				fmt.Printf("Warning: failed to refresh .env: %v\n", err)
			} else {
				fmt.Println("Refreshed workspace environment")
			}
		}

		workspace.GenerateVSCodeWorkspace(wsPath)
		return nil
	},
}

// repoSyncResult holds the result of syncing a single repo
type repoSyncResult struct {
	name            string
	branch          string
	status          string // "synced", "skipped", "failed"
	message         string
	ahead           int
	behind          int
	dirty           bool
	dirtyStatus     string
	lockfileChanged bool
}

// SSM parameter suffixes to fetch
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

var ssmToEnvKey = map[string]string{
	"customerUserPoolId":     "USERPOOL_ID",
	"customerWebClientId":    "WEB_CLIENT_ID",
	"identityPoolIdCustomer": "IDENTITY_POOL_ID",
	"businessUserPoolId":     "BUSINESS_USERPOOL_ID",
	"businessWebClientId":    "BUSINESS_WEB_CLIENT_ID",
	"identityPoolIdBusiness": "BUSINESS_IDENTITY_POOL_ID",
	"squareClientId":         "SQUARE_CLIENT_ID",
	"cloverAppId":            "CLOVER_APP_ID",
	"appConfig":              "APP_CONFIG_VALUES",
	"googleApiKey_Android":   "GOOGLE_API_KEY_ANDROID",
	"googleMapsKey":          "GOOGLE_MAPS_KEY",
	"githubToken":            "GITHUB_TOKEN",
	"stripePublicKey":        "STRIPE_PUBLIC_KEY",
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

	envVars := mapSSMToEnv(ssmVars, region, env, ws)

	if err := workspace.WriteGlobalEnv(wsPath, envVars); err != nil {
		return err
	}

	fmt.Printf("Updated %s (%d variables)\n", workspace.GlobalEnvPath(wsPath), len(envVars))
	return nil
}

func refreshEnvQuiet(wsPath string, ws *workspace.Workspace) error {
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

	if err := aws.GetCallerIdentityQuiet(profile); err != nil {
		if err := aws.SSOLogin(profile); err != nil {
			return fmt.Errorf("AWS login failed: %w", err)
		}
	}

	ssmVars, err := github.FetchMultipleFromSSM(profile, env, region, ssmParamSuffixes)
	if err != nil {
		return fmt.Errorf("failed to fetch parameters: %w", err)
	}

	envVars := mapSSMToEnv(ssmVars, region, env, ws)
	return workspace.WriteGlobalEnv(wsPath, envVars)
}

func mapSSMToEnv(ssmVars map[string]string, region, env string, ws *workspace.Workspace) map[string]string {
	envVars := make(map[string]string)
	for ssmKey, value := range ssmVars {
		if envKey, ok := ssmToEnvKey[ssmKey]; ok {
			envVars[envKey] = value
		} else {
			envVars[ssmKey] = value
		}
	}

	// Business Website NEXT_PUBLIC_* mappings
	if v, ok := envVars["BUSINESS_USERPOOL_ID"]; ok && v != "" {
		envVars["NEXT_PUBLIC_USERPOOL_ID"] = v
	}
	if v, ok := envVars["BUSINESS_WEB_CLIENT_ID"]; ok && v != "" {
		envVars["NEXT_PUBLIC_WEB_CLIENT_ID"] = v
	}
	if v, ok := envVars["BUSINESS_IDENTITY_POOL_ID"]; ok && v != "" {
		envVars["NEXT_PUBLIC_IDENTITY_POOL_ID"] = v
	}
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

	envVars["AWS_REGION"] = region
	envVars["NEXT_PUBLIC_AWS_REGION"] = region
	envVars["APP_ENV"] = env
	if env != "" {
		envVars["NEXT_PUBLIC_APP_ENV"] = env
	}

	for k, v := range ws.Env {
		envVars[k] = v
	}
	return envVars
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

// cdkLambdaMappings defines which Lambda repo each CDK repo needs symlinked inside it.
var cdkLambdaMappings = []struct {
	CDK    string
	Lambda string
}{
	{CDK: "InternalServiceCDK", Lambda: "InternalAPILambda"},
	{CDK: "AppServiceCDK", Lambda: "AppAPILambda"},
	{CDK: "BusinessServiceCDK", Lambda: "BusinessAPILambda"},
}

// linkCDKDependencies creates symlinks from each CDK repo to its sibling Lambda repo.
// Uses relative symlinks so they work on any machine.
func linkCDKDependencies(wsPath string) {
	fmt.Println("\nLinking CDK dependencies...")
	anyLinked := false
	for _, m := range cdkLambdaMappings {
		cdkDir := filepath.Join(wsPath, m.CDK)
		lambdaDir := filepath.Join(wsPath, m.Lambda)

		// Both repos must exist
		if _, err := os.Stat(cdkDir); os.IsNotExist(err) {
			continue
		}
		if _, err := os.Stat(lambdaDir); os.IsNotExist(err) {
			continue
		}

		symlinkPath := filepath.Join(cdkDir, m.Lambda)

		// Check if symlink already exists and is valid
		if info, err := os.Lstat(symlinkPath); err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				// Verify it resolves correctly
				if _, err := os.Stat(symlinkPath); err == nil {
					// Valid symlink — skip silently
					continue
				}
				// Broken symlink — remove and recreate
				os.Remove(symlinkPath)
			} else {
				// Something else exists there (real dir/file) — skip
				continue
			}
		}

		// Create relative symlink: ../Lambda from inside CDK dir
		target := filepath.Join("..", m.Lambda)
		if err := os.Symlink(target, symlinkPath); err != nil {
			fmt.Printf("  ✗ %s → %s: %v\n", m.CDK, m.Lambda, err)
		} else {
			fmt.Printf("  🔗 %s → %s\n", m.CDK, m.Lambda)
			anyLinked = true
		}
	}
	if !anyLinked {
		fmt.Println("  CDK dependencies already linked")
	}
}

func syncRepo(wsPath string, ws *workspace.Workspace, name string) error {
	repo, ok := ws.Repos[name]
	if !ok {
		return fmt.Errorf("repo '%s' not found — run 'spark-cli list' to see repos", name)
	}

	repoDir := filepath.Join(wsPath, repo.Path)
	if _, err := os.Stat(repoDir); os.IsNotExist(err) {
		return fmt.Errorf("repo directory missing — run 'spark-cli use %s'", name)
	}

	result := syncRepoFull(wsPath, ws, name, repo, repoDir)
	printResult(result)

	if syncInstall && result.lockfileChanged {
		installRepo(wsPath, ws, name, repoDir)
	}

	// If we just synced a CDK repo, ensure its Lambda symlink is in place
	for _, m := range cdkLambdaMappings {
		if name == m.CDK {
			linkCDKDependencies(wsPath)
			break
		}
	}

	return nil
}

func syncAllRepos(wsPath string, ws *workspace.Workspace) error {
	if len(ws.Repos) == 0 {
		fmt.Println("No repos in workspace — run 'spark-cli use <repo>' to add one")
		return nil
	}

	allNames := make([]string, 0, len(ws.Repos))
	for name := range ws.Repos {
		allNames = append(allNames, name)
	}
	sort.Strings(allNames)

	// Phase 1: parallel fetch all repos
	fmt.Println("Fetching all repos...")
	var wg sync.WaitGroup
	for _, name := range allNames {
		repo := ws.Repos[name]
		repoDir := filepath.Join(wsPath, repo.Path)
		if _, err := os.Stat(repoDir); os.IsNotExist(err) {
			continue
		}
		wg.Add(1)
		go func(dir string) {
			defer wg.Done()
			git.FetchQuiet(dir, "origin")
		}(repoDir)
	}
	wg.Wait()

	// Phase 2: rebase all branches sequentially (safe, needs working tree)
	results := make([]repoSyncResult, 0, len(allNames))
	for _, name := range allNames {
		repo := ws.Repos[name]
		repoDir := filepath.Join(wsPath, repo.Path)

		if _, err := os.Stat(repoDir); os.IsNotExist(err) {
			results = append(results, repoSyncResult{
				name:    name,
				status:  "skipped",
				message: "not cloned",
			})
			continue
		}

		result := syncRepoFull(wsPath, ws, name, repo, repoDir)
		results = append(results, result)
	}

	// Phase 3: print status table
	fmt.Println()
	printStatusTable(results)

	// Phase 4: npm install where package-lock changed
	if syncInstall {
		fmt.Println("\nInstalling dependencies where package-lock.json changed...")
		wsEnv := buildSyncEnv(wsPath, ws)
		var installed int
		for _, r := range results {
			if !r.lockfileChanged {
				continue
			}
			repo := ws.Repos[r.name]
			repoDir := filepath.Join(wsPath, repo.Path)
			if _, err := os.Stat(filepath.Join(repoDir, "package.json")); os.IsNotExist(err) {
				continue
			}
			fmt.Printf("  npm install %s...", r.name)
			if err := runSyncCmd(repoDir, "npm install", wsEnv); err != nil {
				fmt.Printf(" ✗ %v\n", err)
			} else {
				fmt.Printf(" ✓\n")
				installed++
			}
		}
		if installed > 0 {
			fmt.Printf("%d repo(s) installed\n", installed)
		} else {
			fmt.Println("No repos needed npm install")
		}
	}

	if syncUpdate {
		fmt.Println("\nUpdating @spark-rewards packages to latest...")
		wsEnv := buildSyncEnv(wsPath, ws)
		var updated int
		for _, name := range allNames {
			repo := ws.Repos[name]
			repoDir := filepath.Join(wsPath, repo.Path)

			// Skip if no package.json
			if _, err := os.Stat(filepath.Join(repoDir, "package.json")); os.IsNotExist(err) {
				continue
			}

			// Find @spark-rewards/* dependencies
			pkgs := findSparkPackages(repoDir)
			if len(pkgs) == 0 {
				continue
			}

			// Update each package to latest
			for _, pkg := range pkgs {
				fmt.Printf("  %s: %s@latest...", name, pkg)
				cmd := fmt.Sprintf("npm install %s@latest --save", pkg)
				if err := runSyncCmd(repoDir, cmd, wsEnv); err != nil {
					fmt.Printf(" ✗\n")
				} else {
					fmt.Printf(" ✓\n")
					updated++
				}
			}
		}
		if updated > 0 {
			fmt.Printf("%d package(s) updated across repos\n", updated)
		} else {
			fmt.Println("All @spark-rewards packages already up to date")
		}
	}

	// Phase 5: link CDK dependencies
	linkCDKDependencies(wsPath)

	return nil
}

// syncRepoFull fetches, rebases all local branches onto main, and returns status
func syncRepoFull(wsPath string, ws *workspace.Workspace, name string, repo workspace.RepoDef, repoDir string) repoSyncResult {
	currentBranch := git.GetCurrentBranch(repoDir)
	targetBranch := getTargetBranch(ws, &repo, repoDir)
	upstream := fmt.Sprintf("origin/%s", targetBranch)

	result := repoSyncResult{
		name:   name,
		branch: currentBranch,
	}

	// Get ahead/behind for current branch vs origin/main
	result.ahead, result.behind = git.AheadBehind(repoDir, currentBranch, upstream)

	// Check dirty
	if git.IsDirty(repoDir) {
		result.dirty = true
		status, err := git.StatusShortColor(repoDir)
		if err != nil || status == "" {
			status, _ = git.Status(repoDir)
		}
		result.dirtyStatus = status
		result.status = "skipped"
		result.message = "dirty working tree"
		return result
	}

	if syncNoRebase {
		if err := git.Pull(repoDir); err != nil {
			result.status = "failed"
			result.message = err.Error()
			return result
		}
		result.status = "synced"
		return result
	}

	// Record package-lock hash before rebase
	lockBefore := fileHash(filepath.Join(repoDir, "package-lock.json"))

	// Get all local branches
	branches := git.ListLocalBranches(repoDir)

	// Rebase current branch first
	if err := git.RebaseQuiet(repoDir, upstream); err != nil {
		git.RebaseAbortQuiet(repoDir)
		result.status = "failed"
		result.message = fmt.Sprintf("rebase %s onto %s failed", currentBranch, upstream)
		return result
	}

	// Rebase other local branches onto main
	var rebasedOthers []string
	var failedOthers []string
	for _, branch := range branches {
		if branch == currentBranch || branch == targetBranch {
			continue
		}
		// Checkout, rebase, come back
		if err := git.CheckoutQuiet(repoDir, branch); err != nil {
			continue
		}
		if err := git.RebaseQuiet(repoDir, upstream); err != nil {
			git.RebaseAbortQuiet(repoDir)
			failedOthers = append(failedOthers, branch)
		} else {
			rebasedOthers = append(rebasedOthers, branch)
		}
	}

	// Return to original branch
	git.CheckoutQuiet(repoDir, currentBranch)

	// Check if package-lock changed
	lockAfter := fileHash(filepath.Join(repoDir, "package-lock.json"))
	result.lockfileChanged = lockBefore != lockAfter

	// Recompute ahead/behind after rebase
	result.ahead, result.behind = git.AheadBehind(repoDir, currentBranch, upstream)

	result.status = "synced"
	if len(rebasedOthers) > 0 {
		result.message = fmt.Sprintf("+%d branches rebased", len(rebasedOthers))
	}
	if len(failedOthers) > 0 {
		if result.message != "" {
			result.message += ", "
		}
		result.message += fmt.Sprintf("%d branch rebase(s) failed: %s", len(failedOthers), strings.Join(failedOthers, ", "))
	}

	return result
}

func printResult(r repoSyncResult) {
	icon := "✓"
	if r.status == "skipped" {
		icon = "⏭"
	} else if r.status == "failed" {
		icon = "✗"
	}
	line := fmt.Sprintf("%s %-25s %-20s", icon, r.name, r.branch)
	if r.ahead > 0 || r.behind > 0 {
		line += fmt.Sprintf(" ↑%d ↓%d", r.ahead, r.behind)
	}
	if r.dirty {
		line += " [dirty]"
	}
	if r.lockfileChanged {
		line += " [lock changed]"
	}
	if r.message != "" {
		line += " — " + r.message
	}
	fmt.Println(line)
}

func printStatusTable(results []repoSyncResult) {
	var synced, skipped, failed int
	for _, r := range results {
		printResult(r)
		switch r.status {
		case "synced":
			synced++
		case "skipped":
			skipped++
		case "failed":
			failed++
		}
	}
	fmt.Printf("\n%d synced, %d skipped, %d failed\n", synced, skipped, failed)
}

func fileHash(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return ""
	}
	// Use size+modtime as a cheap hash
	return fmt.Sprintf("%d-%d", info.Size(), info.ModTime().UnixNano())
}

func installRepo(wsPath string, ws *workspace.Workspace, name, repoDir string) {
	if _, err := os.Stat(filepath.Join(repoDir, "package.json")); os.IsNotExist(err) {
		return
	}
	wsEnv := buildSyncEnv(wsPath, ws)
	fmt.Printf("  npm install %s...", name)
	if err := runSyncCmd(repoDir, "npm install", wsEnv); err != nil {
		fmt.Printf(" ✗ %v\n", err)
	} else {
		fmt.Printf(" ✓\n")
	}
}

func buildSyncEnv(wsPath string, ws *workspace.Workspace) map[string]string {
	wsEnv := make(map[string]string)
	dotEnv, _ := workspace.ReadGlobalEnv(wsPath)
	for k, v := range dotEnv {
		wsEnv[k] = v
	}
	for k, v := range ws.Env {
		wsEnv[k] = v
	}
	return ensureGitHubTokenSync(wsEnv)
}

func runSyncCmd(dir, command string, wsEnv map[string]string) error {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/zsh"
	}
	cmd := exec.Command(shell, "-l", "-c", command)
	cmd.Dir = dir
	cmd.Stdout = nil
	cmd.Stderr = nil

	if len(wsEnv) > 0 {
		envMap := make(map[string]string)
		for _, e := range os.Environ() {
			if idx := strings.IndexByte(e, '='); idx != -1 {
				envMap[e[:idx]] = e[idx+1:]
			}
		}
		for k, v := range wsEnv {
			envMap[k] = v
		}
		var env []string
		for k, v := range envMap {
			env = append(env, fmt.Sprintf("%s=%s", k, v))
		}
		cmd.Env = env
	}
	return cmd.Run()
}

// findSparkPackages reads package.json and returns all @spark-rewards/* dependency names
func findSparkPackages(repoDir string) []string {
	pkgPath := filepath.Join(repoDir, "package.json")
	data, err := os.ReadFile(pkgPath)
	if err != nil {
		return nil
	}

	var pkg struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil
	}

	seen := make(map[string]bool)
	var result []string
	for name := range pkg.Dependencies {
		if strings.HasPrefix(name, "@spark-rewards/") && !seen[name] {
			seen[name] = true
			result = append(result, name)
		}
	}
	for name := range pkg.DevDependencies {
		if strings.HasPrefix(name, "@spark-rewards/") && !seen[name] {
			seen[name] = true
			result = append(result, name)
		}
	}
	sort.Strings(result)
	return result
}

func ensureGitHubTokenSync(wsEnv map[string]string) map[string]string {
	if os.Getenv("GITHUB_TOKEN") != "" {
		return wsEnv
	}
	if wsEnv != nil {
		if _, ok := wsEnv["GITHUB_TOKEN"]; ok {
			return wsEnv
		}
	}
	out, err := exec.Command("gh", "auth", "token").Output()
	if err != nil {
		return wsEnv
	}
	token := strings.TrimSpace(string(out))
	if token != "" {
		if wsEnv == nil {
			wsEnv = make(map[string]string)
		}
		wsEnv["GITHUB_TOKEN"] = token
	}
	return wsEnv
}

func init() {
	syncCmd.Flags().StringVar(&syncBranch, "branch", "", "Target branch (default: main)")
	syncCmd.Flags().BoolVar(&syncNoRebase, "no-rebase", false, "Use git pull instead of rebase")
	syncCmd.Flags().StringVar(&syncEnv, "env", "", "Refresh .env from this SSM environment (e.g. beta, prod)")
	syncCmd.Flags().BoolVarP(&syncInstall, "install", "i", false, "Run npm install on repos where package-lock.json changed")
	syncCmd.Flags().BoolVarP(&syncUpdate, "update", "u", false, "Update @spark-rewards/* packages to latest in all repos")
	workspaceCmd.AddCommand(syncCmd)
}
