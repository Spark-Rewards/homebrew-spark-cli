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

type depMapping struct {
	api string
	pkg string
}

var modelToAPI = map[string]depMapping{
	"AppModel":      {api: "AppAPI", pkg: "@spark-rewards/sra-sdk"},
	"BusinessModel": {api: "BusinessAPI", pkg: "@spark-rewards/srw-sdk"},
}

var apiToModel = map[string]string{
	"AppAPI":      "AppModel",
	"BusinessAPI": "BusinessModel",
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

	if script == "build" && !runPublished {
		if err := autoLinkDeps(wsPath, ws, repoName); err != nil {
			fmt.Printf("Warning: dependency linking issue: %v\n", err)
		}
	}

	projType := detectProjectType(repoDir)
	command := buildCommand(repoDir, projType, script, extraArgs)

	if command == "" {
		showAvailableScripts(repoDir, projType, repoName)
		return fmt.Errorf("script '%s' not available in %s", script, repoName)
	}

	fmt.Printf("=== %s: %s ===\n", repoName, command)
	if err := runShellCmd(repoDir, command); err != nil {
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
	modelName, isAPI := apiToModel[name]
	if !isAPI {
		return nil
	}

	modelRepo, exists := ws.Repos[modelName]
	if !exists {
		return nil
	}

	modelDir := filepath.Join(wsPath, modelRepo.Path)
	apiDir := filepath.Join(wsPath, ws.Repos[name].Path)
	mapping := modelToAPI[modelName]

	if !npm.IsBuilt(modelDir) {
		fmt.Printf("Using published %s (local not built)\n", mapping.pkg)
		return nil
	}

	if npm.IsLinked(apiDir, mapping.pkg) {
		fmt.Printf("Using local %s (already linked)\n", modelName)
		return nil
	}

	fmt.Printf("Linking local %s -> %s...\n", modelName, name)
	buildDir := npm.BuildOutputDir(modelDir)

	if err := npm.Link(buildDir); err != nil {
		return fmt.Errorf("npm link in %s failed: %w", modelName, err)
	}

	if err := npm.LinkPackage(apiDir, mapping.pkg); err != nil {
		return fmt.Errorf("npm link %s failed: %w", mapping.pkg, err)
	}

	fmt.Printf("Linked: %s now uses local %s\n", name, modelName)
	return nil
}

func autoLinkConsumers(wsPath string, ws *workspace.Workspace, name string) error {
	mapping, isModel := modelToAPI[name]
	if !isModel {
		return nil
	}

	apiRepo, exists := ws.Repos[mapping.api]
	if !exists {
		return nil
	}

	apiDir := filepath.Join(wsPath, apiRepo.Path)
	if _, err := os.Stat(apiDir); os.IsNotExist(err) {
		return nil
	}

	modelDir := filepath.Join(wsPath, ws.Repos[name].Path)
	buildDir := npm.BuildOutputDir(modelDir)

	if !npm.IsBuilt(modelDir) {
		return nil
	}

	if npm.IsLinked(apiDir, mapping.pkg) {
		return nil
	}

	fmt.Printf("Auto-linking to consumer %s...\n", mapping.api)

	if err := npm.Link(buildDir); err != nil {
		return fmt.Errorf("npm link failed: %w", err)
	}

	if err := npm.LinkPackage(apiDir, mapping.pkg); err != nil {
		return fmt.Errorf("npm link %s in %s failed: %w", mapping.pkg, mapping.api, err)
	}

	fmt.Printf("Linked: %s now uses local %s\n", mapping.api, name)
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

		if modelName, isAPI := apiToModel[n]; isAPI {
			if _, exists := ws.Repos[modelName]; exists {
				collect(modelName)
				deps = append(deps, modelName)
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
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func init() {
	runCmd.Flags().BoolVarP(&runRecursive, "recursive", "r", false, "Build dependencies first (only for 'build')")
	runCmd.Flags().BoolVar(&runPublished, "published", false, "Force use of published packages (no local linking)")
	rootCmd.AddCommand(runCmd)
}
