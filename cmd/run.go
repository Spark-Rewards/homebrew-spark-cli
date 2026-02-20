package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Spark-Rewards/homebrew-spk/internal/npm"
	"github.com/Spark-Rewards/homebrew-spk/internal/workspace"
	"github.com/spf13/cobra"
)

var (
	runRecursive bool
	runPublished bool
	runWatch     bool
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

var runCmd = &cobra.Command{
	Use:   "run <script>",
	Short: "Run an npm script in the current repo",
	Long: `Runs an npm script (build, test, etc.) in the current repo.

Must be run from inside a repo directory.

For 'build', automatically links locally-built dependencies (like Amazon's Brazil Build).
Use --recursive (-r) with 'build' to build dependencies first.

Examples:
  spk run build              # npm run build (with local dependency linking)
  spk run build -r           # build dependencies first, then this repo
  spk run test               # npm test
  spk run test --watch       # npm run test:watch
  spk run lint               # npm run lint
  spk run start              # npm run start`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		script := args[0]

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

		if script == "build" && runRecursive {
			return buildRecursivelyRun(wsPath, ws, repoName)
		}

		return runScript(wsPath, ws, repoName, script)
	},
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

func runScript(wsPath string, ws *workspace.Workspace, repoName, script string) error {
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

	command := getScriptCommand(repoName, repo, repoDir, script)
	if command == "" {
		return fmt.Errorf("script '%s' not found in %s", script, repoName)
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

func getScriptCommand(name string, repo workspace.RepoDef, repoDir, script string) string {
	pkgPath := filepath.Join(repoDir, "package.json")
	if !fileExistsRun(pkgPath) {
		return getFallbackCommand(repoDir, script)
	}

	data, err := os.ReadFile(pkgPath)
	if err != nil {
		return ""
	}

	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return ""
	}

	if script == "test" && runWatch {
		if _, ok := pkg.Scripts["test:watch"]; ok {
			return "npm run test:watch"
		}
	}

	actualScript := script
	if script == "build" {
		if _, ok := pkg.Scripts["build:all"]; ok {
			if name == "AppModel" || name == "BusinessModel" {
				actualScript = "build:all"
			}
		}
	}

	if _, ok := pkg.Scripts[actualScript]; ok {
		if actualScript == "test" {
			return "npm test"
		}
		return fmt.Sprintf("npm run %s", actualScript)
	}

	return ""
}

func getFallbackCommand(repoDir, script string) string {
	switch script {
	case "build":
		if fileExistsRun(filepath.Join(repoDir, "build.gradle")) || fileExistsRun(filepath.Join(repoDir, "build.gradle.kts")) {
			return "./gradlew build"
		}
		if fileExistsRun(filepath.Join(repoDir, "Makefile")) {
			return "make"
		}
		if fileExistsRun(filepath.Join(repoDir, "go.mod")) {
			return "go build ./..."
		}
	case "test":
		if fileExistsRun(filepath.Join(repoDir, "build.gradle")) || fileExistsRun(filepath.Join(repoDir, "build.gradle.kts")) {
			return "./gradlew test"
		}
		if fileExistsRun(filepath.Join(repoDir, "go.mod")) {
			return "go test ./..."
		}
	}
	return ""
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

			if err := runScript(wsPath, ws, dep, "build"); err != nil {
				return fmt.Errorf("dependency build failed at '%s': %w", dep, err)
			}
			fmt.Println()
		}
	}

	return runScript(wsPath, ws, target, "build")
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

func listAvailableScripts(repoDir string) []string {
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

	var scripts []string
	for name := range pkg.Scripts {
		if !strings.HasPrefix(name, "pre") && !strings.HasPrefix(name, "post") {
			scripts = append(scripts, name)
		}
	}
	return scripts
}

func init() {
	runCmd.Flags().BoolVarP(&runRecursive, "recursive", "r", false, "Build dependencies first (only for 'build')")
	runCmd.Flags().BoolVar(&runPublished, "published", false, "Force use of published packages (no local linking)")
	runCmd.Flags().BoolVarP(&runWatch, "watch", "w", false, "Run in watch mode (for 'test')")
	rootCmd.AddCommand(runCmd)
}
