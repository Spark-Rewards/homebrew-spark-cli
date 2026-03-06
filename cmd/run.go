package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Spark-Rewards/homebrew-spark-cli/internal/workspace"
	"github.com/spf13/cobra"
)

type projectType int

const (
	projectTypeNode projectType = iota
	projectTypeGradle
	projectTypeGo
	projectTypeMake
	projectTypeUnknown
)

var runCmd = &cobra.Command{
	Use:   "run [command] [args...]",
	Short: "Run any command with workspace environment injected",
	Long: `Wrapper that injects workspace environment variables into any command.

If inside a repo directory, auto-detects project type and maps scripts:
  Node/npm:    spark-cli run <script>  →  npm run <script>
  Gradle:      spark-cli run <task>    →  ./gradlew <task>
  Go:          spark-cli run build     →  go build ./...
  Make:        spark-cli run <target>  →  make <target>

Or pass any arbitrary command:
  spark-cli run -- aws s3 ls
  spark-cli run -- npm install
  spark-cli run -- echo $GITHUB_TOKEN

Workspace env includes:
  - .env file from workspace root
  - workspace.json env overrides
  - GITHUB_TOKEN (auto-resolved from gh auth if not set)

Examples:
  spark-cli run              # list available scripts for current repo
  spark-cli run build        # npm run build / ./gradlew build
  spark-cli run test         # npm test / ./gradlew test
  spark-cli run -- ls -la    # run arbitrary command with workspace env`,
	Args:                  cobra.ArbitraryArgs,
	DisableFlagParsing:    false,
	RunE: func(cmd *cobra.Command, args []string) error {
		wsPath, err := workspace.Find()
		if err != nil {
			return err
		}

		ws, err := workspace.Load(wsPath)
		if err != nil {
			return err
		}

		// Build workspace env
		wsEnv := buildWorkspaceEnv(wsPath, ws)

		// If no args, try to show available scripts for current repo
		if len(args) == 0 {
			repoName, repoDir := detectCurrentRepo(wsPath, ws)
			if repoName != "" {
				projType := detectProjectType(repoDir)
				showAvailableScripts(repoDir, projType, repoName)
			} else {
				fmt.Println("Run any command with workspace env:")
				fmt.Println("  spark-cli run -- <command>")
				fmt.Println("  spark-cli run <script>  (inside a repo)")
			}
			return nil
		}

		// Check if inside a repo — if so, map to project-specific commands
		repoName, _ := detectCurrentRepo(wsPath, ws)
		if repoName != "" {
			return runRepoScript(wsPath, ws, repoName, args[0], args[1:], wsEnv)
		}

		// Not in a repo — run as raw command
		return runRawCommand(wsPath, args, wsEnv)
	},
}

// buildWorkspaceEnv assembles env vars from .env, workspace.json, and gh auth
func buildWorkspaceEnv(wsPath string, ws *workspace.Workspace) map[string]string {
	wsEnv := make(map[string]string)

	// Load .env file from workspace root
	dotEnv, _ := workspace.ReadGlobalEnv(wsPath)
	for k, v := range dotEnv {
		wsEnv[k] = v
	}

	// Overlay workspace.json env (higher priority)
	for k, v := range ws.Env {
		wsEnv[k] = v
	}

	// Auto-resolve GITHUB_TOKEN if not set
	wsEnv = ensureGitHubToken(wsEnv)

	return wsEnv
}

func runRepoScript(wsPath string, ws *workspace.Workspace, repoName, script string, extraArgs []string, wsEnv map[string]string) error {
	repo, ok := ws.Repos[repoName]
	if !ok {
		return fmt.Errorf("repo '%s' not found in workspace", repoName)
	}

	repoDir := filepath.Join(wsPath, repo.Path)
	if _, err := os.Stat(repoDir); os.IsNotExist(err) {
		return fmt.Errorf("repo directory %s does not exist", repoDir)
	}

	projType := detectProjectType(repoDir)

	// Auto-install node_modules if missing for Node projects
	if projType == projectTypeNode {
		if err := ensureNodeModules(repoDir, wsEnv); err != nil {
			return err
		}
	}

	command := buildCommand(repoDir, projType, script, extraArgs)
	if command == "" {
		showAvailableScripts(repoDir, projType, repoName)
		return fmt.Errorf("script '%s' not available in %s", script, repoName)
	}

	fmt.Printf("=== %s: %s ===\n", repoName, command)
	return runShellCmdWithEnv(repoDir, command, wsEnv)
}

func runRawCommand(wsPath string, args []string, wsEnv map[string]string) error {
	command := strings.Join(args, " ")
	fmt.Printf("=== run: %s ===\n", command)
	return runShellCmdWithEnv(wsPath, command, wsEnv)
}

func ensureNodeModules(repoDir string, wsEnv map[string]string) error {
	nodeModules := filepath.Join(repoDir, "node_modules")
	needsInstall := false

	if _, err := os.Stat(nodeModules); os.IsNotExist(err) {
		fmt.Printf("node_modules missing — running npm install...\n")
		needsInstall = true
	} else if _, err := os.Stat(filepath.Join(nodeModules, ".package-lock.json")); os.IsNotExist(err) {
		fmt.Printf("node_modules incomplete — running npm install...\n")
		needsInstall = true
	}

	if needsInstall {
		if err := runShellCmdWithEnv(repoDir, "npm install", wsEnv); err != nil {
			return fmt.Errorf("npm install failed: %w", err)
		}
		fmt.Println()
	}
	return nil
}

func detectCurrentRepo(wsPath string, ws *workspace.Workspace) (string, string) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", ""
	}

	for name, repo := range ws.Repos {
		repoDir := filepath.Join(wsPath, repo.Path)
		absRepoDir, _ := filepath.Abs(repoDir)

		if cwd == absRepoDir || isSubdir(absRepoDir, cwd) {
			return name, absRepoDir
		}
	}
	return "", ""
}

func isSubdir(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return !filepath.IsAbs(rel) && len(rel) > 0 && rel[0] != '.'
}

func detectProjectType(repoDir string) projectType {
	if fileExistsCheck(filepath.Join(repoDir, "package.json")) {
		return projectTypeNode
	}
	if fileExistsCheck(filepath.Join(repoDir, "build.gradle")) || fileExistsCheck(filepath.Join(repoDir, "build.gradle.kts")) {
		return projectTypeGradle
	}
	if fileExistsCheck(filepath.Join(repoDir, "go.mod")) {
		return projectTypeGo
	}
	if fileExistsCheck(filepath.Join(repoDir, "Makefile")) {
		return projectTypeMake
	}
	return projectTypeUnknown
}

func buildCommand(repoDir string, projType projectType, script string, extraArgs []string) string {
	switch projType {
	case projectTypeNode:
		return buildNpmCommand(repoDir, script, extraArgs)
	case projectTypeGradle:
		return buildGradleCommand(script, extraArgs)
	case projectTypeGo:
		return buildGoCommand(script, extraArgs)
	case projectTypeMake:
		return buildMakeCommand(script, extraArgs)
	default:
		return ""
	}
}

func buildNpmCommand(repoDir, script string, extraArgs []string) string {
	scripts := getNpmScripts(repoDir)
	if scripts == nil {
		return ""
	}
	if _, ok := scripts[script]; !ok {
		return ""
	}
	cmd := fmt.Sprintf("npm run %s", script)
	if len(extraArgs) > 0 {
		cmd += " -- " + strings.Join(extraArgs, " ")
	}
	return cmd
}

func buildGradleCommand(script string, extraArgs []string) string {
	allTasks := append([]string{script}, extraArgs...)
	return "./gradlew " + strings.Join(allTasks, " ")
}

func buildGoCommand(script string, extraArgs []string) string {
	switch script {
	case "build":
		args := "./..."
		if len(extraArgs) > 0 {
			args = strings.Join(extraArgs, " ")
		}
		return "go build " + args
	case "test":
		args := "./..."
		if len(extraArgs) > 0 {
			args = strings.Join(extraArgs, " ")
		}
		return "go test " + args
	case "run":
		if len(extraArgs) > 0 {
			return "go run " + strings.Join(extraArgs, " ")
		}
		return "go run ."
	case "fmt":
		return "go fmt ./..."
	case "vet":
		return "go vet ./..."
	default:
		return ""
	}
}

func buildMakeCommand(script string, extraArgs []string) string {
	allTargets := append([]string{script}, extraArgs...)
	return "make " + strings.Join(allTargets, " ")
}

func getNpmScripts(repoDir string) map[string]string {
	pkgPath := filepath.Join(repoDir, "package.json")
	data, err := os.ReadFile(pkgPath)
	if err != nil {
		return nil
	}
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil
	}
	return pkg.Scripts
}

func showAvailableScripts(repoDir string, projType projectType, repoName string) {
	fmt.Printf("\nAvailable scripts in %s:\n", repoName)
	switch projType {
	case projectTypeNode:
		scripts := getNpmScripts(repoDir)
		if scripts != nil {
			var names []string
			for name := range scripts {
				if !strings.HasPrefix(name, "pre") && !strings.HasPrefix(name, "post") {
					names = append(names, name)
				}
			}
			sort.Strings(names)
			for _, name := range names {
				fmt.Printf("  spark-cli run %s\n", name)
			}
		}
	case projectTypeGradle:
		fmt.Println("  spark-cli run build")
		fmt.Println("  spark-cli run test")
		fmt.Println("  spark-cli run clean build")
	case projectTypeGo:
		fmt.Println("  spark-cli run build")
		fmt.Println("  spark-cli run test")
		fmt.Println("  spark-cli run fmt")
		fmt.Println("  spark-cli run vet")
	case projectTypeMake:
		fmt.Println("  spark-cli run <target>")
	default:
		fmt.Println("  (no recognized project type)")
	}
	fmt.Println()
}

func fileExistsCheck(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func runShellCmdWithEnv(dir, command string, wsEnv map[string]string) error {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/zsh"
	}

	cmd := exec.Command(shell, "-l", "-c", command)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

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

// ensureGitHubToken auto-resolves GITHUB_TOKEN from gh auth if not already set
func ensureGitHubToken(wsEnv map[string]string) map[string]string {
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
	if token == "" {
		return wsEnv
	}

	if wsEnv == nil {
		wsEnv = make(map[string]string)
	}
	wsEnv["GITHUB_TOKEN"] = token
	fmt.Println("Using GITHUB_TOKEN from gh auth")
	return wsEnv
}

func init() {
	runCmd.GroupID = "infra"
	rootCmd.AddCommand(runCmd)
}
