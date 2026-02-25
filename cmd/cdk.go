package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Spark-Rewards/homebrew-spark-cli/internal/workspace"
	"github.com/spf13/cobra"
)

const cdkConfigFile = "cdk.json"

// profileMap maps short profile names to AWS CLI profile names.
var profileMap = map[string]string{
	"pipeline": "openclaw-pipeline",
	"beta":     "openclaw-beta",
	"prod":     "openclaw-prod",
}

var cdkCmd = &cobra.Command{
	Use:   "cdk [cdk-args...]",
	Short: "Run AWS CDK CLI in the workspace CDK repo (e.g. list, deploy, diff | -h)",
	Long: `Runs the AWS CDK CLI in the workspace context. Resolves the CDK app directory
from the current repo (if it contains cdk.json) or from CorePipeline (or any
workspace repo that contains cdk.json). Passes all arguments through to cdk.

A --profile / -p flag is available to select an AWS account:
  pipeline  →  AWS_PROFILE=openclaw-pipeline
  beta      →  AWS_PROFILE=openclaw-beta
  prod      →  AWS_PROFILE=openclaw-prod

AWS_DEFAULT_OUTPUT=json is always injected. Workspace env (GITHUB_TOKEN etc.)
is also injected so cdk synth can resolve private npm packages.

Examples:
  spark-cli cdk list
  spark-cli cdk --profile pipeline list
  spark-cli cdk -p beta deploy PipelineStack/beta/SomeStack
  spark-cli cdk diff
  spark-cli cdk synth`,
	Args:               cobra.ArbitraryArgs,
	DisableFlagParsing: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// --- Parse --profile / -p from args manually (before forwarding to cdk) ---
		profileShort := ""
		var cdkArgs []string

		for i := 0; i < len(args); i++ {
			arg := args[i]
			switch {
			case arg == "--profile" || arg == "-p":
				if i+1 < len(args) {
					profileShort = args[i+1]
					i++ // skip value
				}
			case strings.HasPrefix(arg, "--profile="):
				profileShort = strings.TrimPrefix(arg, "--profile=")
			case strings.HasPrefix(arg, "-p="):
				profileShort = strings.TrimPrefix(arg, "-p=")
			default:
				cdkArgs = append(cdkArgs, arg)
			}
		}

		// --- Load workspace ---
		wsPath, err := workspace.Find()
		if err != nil {
			return err
		}

		ws, err := workspace.Load(wsPath)
		if err != nil {
			return err
		}

		// --- Resolve AWS profile ---
		awsProfileEnvVal := ""

		if profileShort != "" {
			mapped, ok := profileMap[profileShort]
			if !ok {
				return fmt.Errorf("unknown profile %q — valid options: pipeline, beta, prod", profileShort)
			}
			awsProfileEnvVal = mapped
		} else if ws.AWSProfile != "" {
			// Fall back to workspace default
			awsProfileEnvVal = ws.AWSProfile
		}

		if awsProfileEnvVal != "" {
			fmt.Printf("Using AWS profile: %s\n", awsProfileEnvVal)
			if profileShort == "prod" {
				fmt.Println("⚠️  Using PROD profile — be careful!")
			}
		}

		// --- Find CDK repo dir ---
		cdkDir, err := findCDKRepoDir(wsPath, ws)
		if err != nil {
			return err
		}

		cdkPath, err := exec.LookPath("cdk")
		if err != nil {
			return fmt.Errorf("cdk not found in PATH — install with: npm install -g aws-cdk")
		}

		// --- Build env ---
		// Start from current os env
		envMap := make(map[string]string)
		for _, e := range os.Environ() {
			if idx := strings.IndexByte(e, '='); idx != -1 {
				envMap[e[:idx]] = e[idx+1:]
			}
		}

		// Overlay workspace env (GITHUB_TOKEN, .env, workspace.json env)
		wsEnv := buildWorkspaceEnv(wsPath, ws)
		for k, v := range wsEnv {
			envMap[k] = v
		}

		// Always inject AWS_DEFAULT_OUTPUT=json (uppercase JSON in config breaks CLI)
		envMap["AWS_DEFAULT_OUTPUT"] = "json"

		// Inject AWS_PROFILE if resolved
		if awsProfileEnvVal != "" {
			envMap["AWS_PROFILE"] = awsProfileEnvVal
		}

		// Flatten env map back to slice
		var env []string
		for k, v := range envMap {
			env = append(env, fmt.Sprintf("%s=%s", k, v))
		}

		c := exec.Command(cdkPath, cdkArgs...)
		c.Dir = cdkDir
		c.Stdin = os.Stdin
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		c.Env = env

		if err := c.Run(); err != nil {
			if exit, ok := err.(*exec.ExitError); ok {
				os.Exit(exit.ExitCode())
			}
			return err
		}
		return nil
	},
}

// findCDKRepoDir returns the repo directory that contains cdk.json.
// Prefers the repo containing the current working dir; otherwise the first workspace repo with cdk.json (e.g. CorePipeline).
func findCDKRepoDir(wsPath string, ws *workspace.Workspace) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	// If cwd is inside a repo that has cdk.json, use it.
	for _, repo := range ws.Repos {
		repoDir := filepath.Join(wsPath, repo.Path)
		absRepo, _ := filepath.Abs(repoDir)
		if cwd == absRepo || isSubdir(absRepo, cwd) {
			if hasCDK(repoDir) {
				return repoDir, nil
			}
			break
		}
	}

	// Else use first workspace repo that has cdk.json (e.g. CorePipeline).
	for _, repo := range ws.Repos {
		repoDir := filepath.Join(wsPath, repo.Path)
		if hasCDK(repoDir) {
			return repoDir, nil
		}
	}

	return "", fmt.Errorf("no CDK app (cdk.json) found in workspace — run from CorePipeline or add cdk.json to a repo")
}

func hasCDK(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, cdkConfigFile))
	return err == nil
}

func init() {
	rootCmd.AddCommand(cdkCmd)
}
