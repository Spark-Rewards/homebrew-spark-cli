package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Spark-Rewards/homebrew-spk/internal/npm"
	"github.com/Spark-Rewards/homebrew-spk/internal/workspace"
	"github.com/spf13/cobra"
)

var (
	runRecursive bool
	runPublished bool
)

type consumerMapping struct {
	consumer string
	pkg      string
	codegen  string // smithy codegen folder name
}

// Each model can have multiple consumers with different codegen outputs
var modelConsumers = map[string][]consumerMapping{
	"AppModel": {
		{consumer: "AppAPI", pkg: "@spark-rewards/sra-sdk", codegen: "typescript-ssdk-codegen"},
		{consumer: "MobileApp", pkg: "@spark-rewards/sra-client", codegen: "typescript-client-codegen"},
	},
	"BusinessModel": {
		{consumer: "BusinessAPI", pkg: "@spark-rewards/srw-sdk", codegen: "typescript-ssdk-codegen"},
		{consumer: "BusinessWebsite", pkg: "@spark-rewards/srw-client", codegen: "typescript-client-codegen"},
	},
}

// Reverse lookup: consumer -> (model, mapping)
func findModelForConsumer(consumer string) (string, *consumerMapping) {
	for model, consumers := range modelConsumers {
		for i := range consumers {
			if consumers[i].consumer == consumer {
				return model, &consumers[i]
			}
		}
	}
	return "", nil
}

type projectType int

const (
	projectTypeNode projectType = iota
	projectTypeGradle
	projectTypeGo
	projectTypeMake
	projectTypeUnknown
)

var runCmd = &cobra.Command{
	Use:   "run [script] [args...]",
	Short: "Run a script in the current repo",
	Long:  getDynamicRunHelp(),
	Args:  cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		wsPath, err := workspace.Find()
		if err != nil {
			return err
		}

		ws, err := workspace.Load(wsPath)
		if err != nil {
			return err
		}

		repoName, err := detectCurrentRepoForRun(wsPath, ws)
		if err != nil {
			return fmt.Errorf("must be run from inside a repo directory")
		}

		if len(args) == 0 {
			repoDir := filepath.Join(wsPath, ws.Repos[repoName].Path)
			projType := detectProjectType(repoDir)
			showAvailableScripts(repoDir, projType, repoName)
			return nil
		}

		script := args[0]
		extraArgs := args[1:]

		if script == "build" && runRecursive {
			return buildRecursivelyRun(wsPath, ws, repoName)
		}

		return runScript(wsPath, ws, repoName, script, extraArgs)
	},
}

func getDynamicRunHelp() string {
	base := `Wrapper for running scripts in the current repo. Automatically detects
the project type and runs the appropriate command.

For Node/npm projects:     spk run <script>  ->  npm run <script>
For Gradle projects:       spk run <task>    ->  ./gradlew <task>
For Go projects:           spk run build     ->  go build ./...
For Make projects:         spk run <target>  ->  make <target>

For 'build', automatically links locally-built dependencies (like Amazon's Brazil Build).
Use --recursive (-r) with 'build' to build dependencies first.

Examples:
  spk run                    # list available scripts
  spk run build              # npm run build / ./gradlew build
  spk run build -r           # build dependencies first, then this repo
  spk run test               # npm test / ./gradlew test
  spk run start              # npm run start
  spk run lint               # npm run lint
  spk run clean build        # ./gradlew clean build (Gradle)`

	wsPath, err := workspace.Find()
	if err != nil {
		return base
	}

	ws, err := workspace.Load(wsPath)
	if err != nil {
		return base
	}

	cwd, err := os.Getwd()
	if err != nil {
		return base
	}

	var repoName string
	var repoDir string
	for name, repo := range ws.Repos {
		rd := filepath.Join(wsPath, repo.Path)
		absRepoDir, _ := filepath.Abs(rd)
		if cwd == absRepoDir || (len(cwd) > len(absRepoDir) && strings.HasPrefix(cwd, absRepoDir+"/")) {
			repoName = name
			repoDir = rd
			break
		}
	}

	if repoName == "" {
		return base
	}

	scripts := getNpmScripts(repoDir)
	if scripts == nil || len(scripts) == 0 {
		return base
	}

	var names []string
	for name := range scripts {
		if !strings.HasPrefix(name, "pre") && !strings.HasPrefix(name, "post") {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	base += fmt.Sprintf("\n\nAvailable scripts in %s:", repoName)
	for _, name := range names {
		base += fmt.Sprintf("\n  spk run %s", name)
	}

	return base
}

func detectCurrentRepoForRun(wsPath string, ws *workspace.Workspace) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("could not get current directory: %w", err)
	}

	for name, repo := range ws.Repos {
		repoDir := filepath.Join(wsPath, repo.Path)
		absRepoDir, _ := filepath.Abs(repoDir)

		if cwd == absRepoDir || isSubdirRun(absRepoDir, cwd) {
			return name, nil
		}
	}

	return "", fmt.Errorf("not inside a repo directory")
}

func isSubdirRun(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return !filepath.IsAbs(rel) && len(rel) > 0 && rel[0] != '.'
}

func runScript(wsPath string, ws *workspace.Workspace, repoName, script string, extraArgs []string) error {
	repo, ok := ws.Repos[repoName]
	if !ok {
		return fmt.Errorf("repo '%s' not found in workspace", repoName)
	}

	repoDir := filepath.Join(wsPath, repo.Path)
	if _, err := os.Stat(repoDir); os.IsNotExist(err) {
		return fmt.Errorf("repo directory %s does not exist", repoDir)
	}

	// Build env: workspace .env file + workspace.json env + auto-resolved GITHUB_TOKEN
	wsEnv := make(map[string]string)

	// Load .env file from workspace root (written by `spk sync`)
	dotEnv, _ := workspace.ReadGlobalEnv(wsPath)
	for k, v := range dotEnv {
		wsEnv[k] = v
	}

	// Overlay workspace.json env (higher priority)
	for k, v := range ws.Env {
		wsEnv[k] = v
	}

	// Fallback: if still no GITHUB_TOKEN, try gh auth
	wsEnv = ensureGitHubToken(wsEnv)

	projType := detectProjectType(repoDir)

	// Auto-install node_modules if missing or broken for Node projects
	if projType == projectTypeNode {
		nodeModules := filepath.Join(repoDir, "node_modules")
		needsInstall := false

		if _, err := os.Stat(nodeModules); os.IsNotExist(err) {
			fmt.Printf("node_modules missing — running npm install...\n")
			needsInstall = true
		} else if _, err := os.Stat(filepath.Join(nodeModules, ".package-lock.json")); os.IsNotExist(err) {
			// .package-lock.json is written at the end of a successful install.
			// If it's missing, the previous install was likely incomplete.
			fmt.Printf("node_modules incomplete — running npm install...\n")
			needsInstall = true
		}

		if needsInstall {
			if err := runShellCmdWithEnv(repoDir, "npm install", wsEnv); err != nil {
				return fmt.Errorf("npm install failed: %w", err)
			}
			fmt.Println()
		}
	}

	if script == "build" && !runPublished {
		if err := autoLinkDeps(wsPath, ws, repoName); err != nil {
			fmt.Printf("Warning: dependency linking issue: %v\n", err)
		}
	}
	command := buildCommand(repoDir, projType, script, extraArgs)

	if command == "" {
		showAvailableScripts(repoDir, projType, repoName)
		return fmt.Errorf("script '%s' not available in %s", script, repoName)
	}

	fmt.Printf("=== %s: %s ===\n", repoName, command)
	if err := runShellCmdWithEnv(repoDir, command, wsEnv); err != nil {
		return fmt.Errorf("%s failed: %w", script, err)
	}

	if script == "build" && !runPublished {
		if err := autoLinkConsumers(wsPath, ws, repoName); err != nil {
			fmt.Printf("Note: %v\n", err)
		}
	}

	return nil
}

func detectProjectType(repoDir string) projectType {
	if fileExistsRun(filepath.Join(repoDir, "package.json")) {
		return projectTypeNode
	}
	if fileExistsRun(filepath.Join(repoDir, "build.gradle")) || fileExistsRun(filepath.Join(repoDir, "build.gradle.kts")) {
		return projectTypeGradle
	}
	if fileExistsRun(filepath.Join(repoDir, "go.mod")) {
		return projectTypeGo
	}
	if fileExistsRun(filepath.Join(repoDir, "Makefile")) {
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
				fmt.Printf("  spk run %s\n", name)
			}
		}
	case projectTypeGradle:
		fmt.Println("  spk run build")
		fmt.Println("  spk run test")
		fmt.Println("  spk run clean")
		fmt.Println("  spk run clean build")
		fmt.Println("  (or any Gradle task)")
	case projectTypeGo:
		fmt.Println("  spk run build")
		fmt.Println("  spk run test")
		fmt.Println("  spk run fmt")
		fmt.Println("  spk run vet")
	case projectTypeMake:
		fmt.Println("  spk run <target>")
		fmt.Println("  (any Makefile target)")
	default:
		fmt.Println("  (no recognized project type)")
	}
	fmt.Println()
}

func fileExistsRun(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func autoLinkDeps(wsPath string, ws *workspace.Workspace, name string) error {
	modelName, mapping := findModelForConsumer(name)
	if mapping == nil {
		return nil
	}

	modelRepo, exists := ws.Repos[modelName]
	if !exists {
		return nil
	}

	modelDir := filepath.Join(wsPath, modelRepo.Path)
	consumerDir := filepath.Join(wsPath, ws.Repos[name].Path)

	if !npm.IsBuiltForCodegen(modelDir, mapping.codegen) {
		fmt.Printf("Using published %s (local not built)\n", mapping.pkg)
		return nil
	}

	if npm.IsLinked(consumerDir, mapping.pkg) {
		fmt.Printf("Using local %s (already linked)\n", modelName)
		return nil
	}

	fmt.Printf("Linking local %s -> %s...\n", modelName, name)
	buildDir := npm.BuildOutputDirForCodegen(modelDir, mapping.codegen)

	if err := npm.Link(buildDir); err != nil {
		return fmt.Errorf("npm link in %s failed: %w", modelName, err)
	}

	if err := npm.LinkPackage(consumerDir, mapping.pkg); err != nil {
		return fmt.Errorf("npm link %s failed: %w", mapping.pkg, err)
	}

	fmt.Printf("Linked: %s now uses local %s\n", name, modelName)
	return nil
}

func autoLinkConsumers(wsPath string, ws *workspace.Workspace, name string) error {
	consumers, isModel := modelConsumers[name]
	if !isModel {
		return nil
	}

	modelDir := filepath.Join(wsPath, ws.Repos[name].Path)

	for _, mapping := range consumers {
		consumerRepo, exists := ws.Repos[mapping.consumer]
		if !exists {
			continue
		}

		consumerDir := filepath.Join(wsPath, consumerRepo.Path)
		if _, err := os.Stat(consumerDir); os.IsNotExist(err) {
			continue
		}

		if !npm.IsBuiltForCodegen(modelDir, mapping.codegen) {
			continue
		}

		if npm.IsLinked(consumerDir, mapping.pkg) {
			continue
		}

		buildDir := npm.BuildOutputDirForCodegen(modelDir, mapping.codegen)

		fmt.Printf("Auto-linking to consumer %s (%s)...\n", mapping.consumer, mapping.pkg)

		if err := npm.Link(buildDir); err != nil {
			fmt.Printf("Warning: npm link failed for %s: %v\n", mapping.consumer, err)
			continue
		}

		if err := npm.LinkPackage(consumerDir, mapping.pkg); err != nil {
			fmt.Printf("Warning: npm link %s in %s failed: %v\n", mapping.pkg, mapping.consumer, err)
			continue
		}

		fmt.Printf("Linked: %s now uses local %s\n", mapping.consumer, name)
	}

	return nil
}

func buildRecursivelyRun(wsPath string, ws *workspace.Workspace, target string) error {
	deps := getDepsForRun(ws, target)

	if len(deps) > 0 {
		fmt.Printf("Building dependencies first: %v\n\n", deps)
		for _, dep := range deps {
			repo, exists := ws.Repos[dep]
			if !exists {
				continue
			}

			repoDir := filepath.Join(wsPath, repo.Path)
			if _, err := os.Stat(repoDir); os.IsNotExist(err) {
				fmt.Printf("[skip] %s (not cloned)\n\n", dep)
				continue
			}

			if err := runScript(wsPath, ws, dep, "build", nil); err != nil {
				return fmt.Errorf("dependency build failed at '%s': %w", dep, err)
			}
			fmt.Println()
		}
	}

	return runScript(wsPath, ws, target, "build", nil)
}

func getDepsForRun(ws *workspace.Workspace, name string) []string {
	var deps []string
	seen := make(map[string]bool)

	var collect func(n string)
	collect = func(n string) {
		if seen[n] {
			return
		}
		seen[n] = true

		if modelName, mapping := findModelForConsumer(n); mapping != nil {
			if _, exists := ws.Repos[modelName]; exists {
				collect(modelName)
				if !containsRun(deps, modelName) {
					deps = append(deps, modelName)
				}
			}
		}

		if repo, exists := ws.Repos[n]; exists {
			for _, dep := range repo.Dependencies {
				if _, depExists := ws.Repos[dep]; depExists {
					collect(dep)
					if !containsRun(deps, dep) {
						deps = append(deps, dep)
					}
				}
			}
		}
	}

	collect(name)

	seen[name] = false
	var result []string
	for _, d := range deps {
		if d != name {
			result = append(result, d)
		}
	}
	return result
}

func containsRun(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func runShellCmd(dir, command string) error {
	return runShellCmdWithEnv(dir, command, nil)
}

func runShellCmdWithEnv(dir, command string, wsEnv map[string]string) error {
	// Use the user's login shell to preserve PATH (nvm, homebrew, etc.)
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
		// Start with current environment, overlay workspace env
		envMap := make(map[string]string)
		for _, e := range os.Environ() {
			if idx := strings.IndexByte(e, '='); idx != -1 {
				envMap[e[:idx]] = e[idx+1:]
			}
		}
		// Workspace env overrides existing
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

// ensureGitHubToken checks if GITHUB_TOKEN is set in the environment or workspace env.
// If not, attempts to source it from `gh auth token`.
func ensureGitHubToken(wsEnv map[string]string) map[string]string {
	// Already set in process env
	if os.Getenv("GITHUB_TOKEN") != "" {
		return wsEnv
	}

	// Already set in workspace env
	if wsEnv != nil {
		if _, ok := wsEnv["GITHUB_TOKEN"]; ok {
			return wsEnv
		}
	}

	// Try to get from gh CLI
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
	runCmd.Flags().BoolVarP(&runRecursive, "recursive", "r", false, "Build dependencies first (only for 'build')")
	runCmd.Flags().BoolVar(&runPublished, "published", false, "Force use of published packages (no local linking)")
	rootCmd.AddCommand(runCmd)
}
