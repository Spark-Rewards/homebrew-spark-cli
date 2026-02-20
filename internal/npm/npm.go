package npm

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	SmithyBuildBase = "smithy/build/smithyprojections/smithy/source"
	// Default codegen for server SDKs
	SmithyBuildPath = SmithyBuildBase + "/typescript-ssdk-codegen"
)

// Link runs `npm link` in the given directory to register a package globally
func Link(dir string, env map[string]string) error {
	cmd := exec.Command("npm", "link")
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = buildEnv(env)
	return cmd.Run()
}

// LinkPackage runs `npm link <pkg>` in the given directory to consume a linked package
func LinkPackage(dir, pkg string, env map[string]string) error {
	cmd := exec.Command("npm", "link", pkg)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = buildEnv(env)
	return cmd.Run()
}

// Unlink runs `npm unlink <pkg>` in the given directory
func Unlink(dir, pkg string) error {
	cmd := exec.Command("npm", "unlink", pkg)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// IsBuilt checks if a Smithy model directory has built artifacts
func IsBuilt(modelDir string) bool {
	buildDir := filepath.Join(modelDir, SmithyBuildPath)

	packageJSON := filepath.Join(buildDir, "package.json")
	distTypes := filepath.Join(buildDir, "dist-types")

	if _, err := os.Stat(packageJSON); err != nil {
		return false
	}
	if _, err := os.Stat(distTypes); err != nil {
		return false
	}
	return true
}

// BuildOutputDir returns the path to the default Smithy SDK build output
func BuildOutputDir(modelDir string) string {
	return filepath.Join(modelDir, SmithyBuildPath)
}

// BuildOutputDirForCodegen returns the path for a specific codegen type
func BuildOutputDirForCodegen(modelDir, codegen string) string {
	return filepath.Join(modelDir, SmithyBuildBase, codegen)
}

// IsBuiltForCodegen checks if a specific codegen output exists
func IsBuiltForCodegen(modelDir, codegen string) bool {
	buildDir := BuildOutputDirForCodegen(modelDir, codegen)
	packageJSON := filepath.Join(buildDir, "package.json")
	if _, err := os.Stat(packageJSON); err != nil {
		return false
	}
	return true
}

// GetPackageName reads the package name from a package.json file
func GetPackageName(dir string) (string, error) {
	packageJSON := filepath.Join(dir, "package.json")
	if _, err := os.Stat(packageJSON); err != nil {
		return "", fmt.Errorf("package.json not found in %s", dir)
	}

	cmd := exec.Command("node", "-p", "require('./package.json').name")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to read package name: %w", err)
	}

	name := string(out)
	if len(name) > 0 && name[len(name)-1] == '\n' {
		name = name[:len(name)-1]
	}
	return name, nil
}

// IsLinked checks if a package is currently npm-linked in the given directory
func IsLinked(dir, pkg string) bool {
	nodeModulesPath := filepath.Join(dir, "node_modules", pkg)
	info, err := os.Lstat(nodeModulesPath)
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeSymlink != 0
}

// CheckNPM verifies that npm is installed
func CheckNPM() error {
	_, err := exec.LookPath("npm")
	if err != nil {
		return fmt.Errorf("npm not found — install Node.js from https://nodejs.org")
	}
	return nil
}

// buildEnv merges extra env vars into the current process environment.
// Returns nil (inherit process env) if extra is empty.
func buildEnv(extra map[string]string) []string {
	if len(extra) == 0 {
		return nil
	}
	envMap := make(map[string]string)
	for _, e := range os.Environ() {
		if idx := strings.IndexByte(e, '='); idx != -1 {
			envMap[e[:idx]] = e[idx+1:]
		}
	}
	for k, v := range extra {
		envMap[k] = v
	}
	env := make([]string, 0, len(envMap))
	for k, v := range envMap {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	return env
}
