// Package nix provides helpers for the SparkRewards Nix-based build system.
// It wraps nix CLI calls and handles workspace/configuration detection.
package nix

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	// NixBinPath is where Determinate Systems installs the nix binary.
	NixBinPath = "/nix/var/nix/profiles/default/bin"
	// NixBin is the full path to the nix executable.
	NixBin = "/nix/var/nix/profiles/default/bin/nix"
	// GitHubOrg is the Spark Rewards GitHub organisation.
	GitHubOrg = "Spark-Rewards"
)

// TrackedRepos are the repos currently covered by the Nix flake.
// Local clone → uses local source. Not cloned → fetches from GitHub.
var TrackedRepos = []string{
	// Internal
	"InternalModel",
	"InternalAPILambda",
	"InternalWebsiteClient",
	"InternalServiceCDK",
	// Business
	"BusinessModel",
	"BusinessAPILambda",
	"BusinessWebsite",
	"BusinessServiceCDK",
	// App
	"AppModel",
	"AppAPILambda",
	"AppServiceCDK",
	// Shared
	"AuditorLambda",
	"CorePipeline",
	"MobileApp",
}

// AllRepos is the full list of known workspace repos (Nix-tracked and non-Nix).
var AllRepos = []string{
	"AppAPILambda",
	"AppModel",
	"AppServiceCDK",
	"AuditorLambda",
	"BusinessAPILambda",
	"BusinessModel",
	"BusinessServiceCDK",
	"BusinessWebsite",
	"CorePipeline",
	"InternalAPILambda",
	"InternalModel",
	"InternalServiceCDK",
	"InternalWebsiteClient",
	"MobileApp",
}

// PackageNames maps Nix attribute names to human-readable descriptions.
var PackageNames = map[string]string{
	// Internal
	"internal-model":          "InternalModel (Smithy SDK)",
	"internal-api-lambda":     "InternalAPILambda (Lambda)",
	"internal-website-client": "InternalWebsiteClient (Website)",
	"internal-service-cdk":    "InternalServiceCDK (CDK)",
	"internal-all":            "All Internal packages",
	// Business
	"business-model":       "BusinessModel (Smithy SDK)",
	"business-api-lambda":  "BusinessAPILambda (Lambda)",
	"business-website":     "BusinessWebsite (Website)",
	"business-service-cdk": "BusinessServiceCDK (CDK)",
	"business-all":         "All Business packages",
	// App
	"app-model":       "AppModel (Smithy SDK)",
	"app-api-lambda":  "AppAPILambda (Lambda)",
	"app-service-cdk": "AppServiceCDK (CDK)",
	"app-all":         "All App packages",
	// Shared
	"auditor-lambda": "AuditorLambda (Lambda)",
	"core-pipeline":  "CorePipeline (CDK)",
	"mobile-app":     "MobileApp (React Native)",
	"shared-all":     "All shared packages",
	// Everything
	"all": "All packages across all services",
}

// RepoToPackage converts a repo name or Nix package name to its canonical Nix attribute name.
func RepoToPackage(name string) string {
	repoMap := map[string]string{
		"InternalModel":         "internal-model",
		"InternalAPILambda":     "internal-api-lambda",
		"InternalWebsiteClient": "internal-website-client",
		"InternalServiceCDK":    "internal-service-cdk",
		"BusinessModel":         "business-model",
		"BusinessAPILambda":     "business-api-lambda",
		"BusinessWebsite":       "business-website",
		"BusinessServiceCDK":    "business-service-cdk",
		"AppModel":              "app-model",
		"AppAPILambda":          "app-api-lambda",
		"AppServiceCDK":         "app-service-cdk",
		"AuditorLambda":         "auditor-lambda",
		"CorePipeline":          "core-pipeline",
		"MobileApp":             "mobile-app",
	}
	if pkg, ok := repoMap[name]; ok {
		return pkg
	}
	// Pass through if already a valid nix attribute name
	if _, ok := PackageNames[name]; ok {
		return name
	}
	return name
}

// IsTracked reports whether a repo name is covered by the Nix flake.
func IsTracked(repo string) bool {
	for _, r := range TrackedRepos {
		if r == repo {
			return true
		}
	}
	return false
}

// FindWorkspace locates the directory containing flake.nix.
// Search order:
//  1. SPARK_WORKSPACE env var
//  2. Walk up from cwd
//  3. ~/SparkRewards (default location)
func FindWorkspace() (string, error) {
	if ws := os.Getenv("SPARK_WORKSPACE"); ws != "" {
		if _, err := os.Stat(filepath.Join(ws, "flake.nix")); err == nil {
			return ws, nil
		}
	}

	dir, err := os.Getwd()
	if err == nil {
		for {
			if _, err := os.Stat(filepath.Join(dir, "flake.nix")); err == nil {
				return dir, nil
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}

	home, _ := os.UserHomeDir()
	def := filepath.Join(home, "SparkRewards")
	if _, err := os.Stat(filepath.Join(def, "flake.nix")); err == nil {
		return def, nil
	}

	return "", fmt.Errorf("no Nix workspace found (no flake.nix)\n" +
		"  Run from ~/SparkRewards, or set SPARK_WORKSPACE=/path/to/workspace")
}

// IsInstalled reports whether the Nix binary is present.
func IsInstalled() bool {
	_, err := os.Stat(NixBin)
	return err == nil
}

// Version returns the installed nix version string, or "" if not installed.
func Version() string {
	if !IsInstalled() {
		return ""
	}
	out, err := exec.Command(NixBin, "--version").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// IsFlakesEnabled reports whether nix flakes are configured in ~/.config/nix/nix.conf.
func IsFlakesEnabled() bool {
	home, _ := os.UserHomeDir()
	conf := filepath.Join(home, ".config", "nix", "nix.conf")
	content, err := os.ReadFile(conf)
	if err != nil {
		return false
	}
	s := string(content)
	return strings.Contains(s, "experimental-features") && strings.Contains(s, "flakes")
}

// HasGitHubToken reports whether a GitHub access token is set in ~/.config/nix/nix.conf.
func HasGitHubToken() bool {
	home, _ := os.UserHomeDir()
	conf := filepath.Join(home, ".config", "nix", "nix.conf")
	content, err := os.ReadFile(conf)
	if err != nil {
		return false
	}
	return strings.Contains(string(content), "access-tokens = github.com=")
}

// HasNpmGitHubPackages reports whether ~/.npmrc is configured for the GitHub Packages registry.
func HasNpmGitHubPackages() bool {
	home, _ := os.UserHomeDir()
	content, err := os.ReadFile(filepath.Join(home, ".npmrc"))
	if err != nil {
		return false
	}
	return strings.Contains(string(content), "npm.pkg.github.com")
}

// GetGitHubToken tries to resolve a GitHub token:
//  1. GITHUB_TOKEN env var
//  2. `gh auth token` CLI
//
// Returns "" if neither is available.
func GetGitHubToken() string {
	if t := os.Getenv("GITHUB_TOKEN"); t != "" {
		return t
	}
	out, err := exec.Command("gh", "auth", "token").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// EnsureFlakesEnabled writes the experimental-features line to nix.conf if missing.
func EnsureFlakesEnabled() error {
	if IsFlakesEnabled() {
		return nil
	}
	return appendNixConf("experimental-features = nix-command flakes")
}

// SetGitHubToken writes (or replaces) the access-tokens line in nix.conf.
func SetGitHubToken(token string) error {
	home, _ := os.UserHomeDir()
	confDir := filepath.Join(home, ".config", "nix")
	if err := os.MkdirAll(confDir, 0755); err != nil {
		return err
	}
	confPath := filepath.Join(confDir, "nix.conf")

	// Read existing, strip any old access-tokens line.
	var lines []string
	if data, err := os.ReadFile(confPath); err == nil {
		scanner := bufio.NewScanner(strings.NewReader(string(data)))
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "access-tokens") {
				lines = append(lines, line)
			}
		}
	}
	lines = append(lines, fmt.Sprintf("access-tokens = github.com=%s", token))

	content := strings.Join(lines, "\n") + "\n"
	return os.WriteFile(confPath, []byte(content), 0600)
}

// SetNpmGitHubPackages appends GitHub Packages config to ~/.npmrc (idempotent).
func SetNpmGitHubPackages(token string) error {
	home, _ := os.UserHomeDir()
	npmrc := filepath.Join(home, ".npmrc")

	content, _ := os.ReadFile(npmrc)
	if strings.Contains(string(content), "npm.pkg.github.com") {
		return nil // already configured
	}

	f, err := os.OpenFile(npmrc, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = fmt.Fprintf(f, "\n@spark-rewards:registry=https://npm.pkg.github.com\n//npm.pkg.github.com/:_authToken=%s\n", token)
	return err
}

// Run executes the nix binary with the given args, filtering output for clean dev experience.
// It sets the Nix bin directory on PATH so nix can find its own tools.
func Run(workspace string, args ...string) error {
	cmd := exec.Command(NixBin, args...)
	cmd.Dir = workspace
	cmd.Stdin = os.Stdin
	cmd.Env = nixEnv()

	// Capture both stdout and stderr, filter for clean output
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	// Filter stdout
	go filterNixOutput(stdout)
	// Filter stderr (nix sends most output here)
	filterNixOutput(stderr)

	return cmd.Wait()
}

// RunRaw executes nix with unfiltered output (for debugging with --verbose).
func RunRaw(workspace string, args ...string) error {
	cmd := exec.Command(NixBin, args...)
	cmd.Dir = workspace
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = nixEnv()
	return cmd.Run()
}

// filterNixOutput reads nix output line-by-line and prints only meaningful lines.
// Filters out: store paths, derivation hashes, progress bars, evaluation traces.
// Keeps: [spark] messages, build phase output, errors, warnings.
func filterNixOutput(r io.Reader) {
	scanner := bufio.NewScanner(r)
	// Increase buffer for long nix lines
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		// Strip ANSI escape codes for matching
		clean := stripAnsi(line)

		// Always show [spark] messages (our custom build output)
		if strings.Contains(clean, "[spark]") {
			// Clean up: remove "trace: " prefix
			msg := clean
			if idx := strings.Index(msg, "[spark]"); idx >= 0 {
				msg = "  " + msg[idx:]
			}
			fmt.Println(msg)
			continue
		}

		// Skip Nix internal phase messages (meaningless to devs)
		if strings.Contains(clean, "Running phase:") {
			continue
		}

		// Show building <package> messages (but clean them up)
		if strings.Contains(clean, "building ") && strings.Contains(clean, ".drv") {
			// Nix outputs "building '/nix/store/xxx-package-name.drv'..."
			// Extract package name between last dash-separated segments before .drv
			if start := strings.LastIndex(clean, "/"); start >= 0 {
				storePath := clean[start+1:]
				if end := strings.Index(storePath, ".drv"); end >= 0 {
					pkgName := storePath[:end]
					// Strip the hash prefix (first 33 chars: hash + dash)
					if len(pkgName) > 33 {
						pkgName = pkgName[33:]
					}
					fmt.Printf("  ⚙  Building %s...\n", pkgName)
				}
			}
			continue
		}

		// Show errors
		if strings.Contains(clean, "error:") ||
			strings.Contains(clean, "npm error") ||
			strings.Contains(clean, "npm ERR!") {
			fmt.Fprintf(os.Stderr, "  ✗ %s\n", strings.TrimSpace(clean))
			continue
		}

		// Show warnings
		if strings.Contains(clean, "warning:") && !strings.Contains(clean, "deprecated") {
			fmt.Fprintf(os.Stderr, "  ⚠ %s\n", strings.TrimSpace(clean))
			continue
		}

		// Filter out everything else:
		// - "evaluating derivation ..."
		// - "copying '/Users/...' to the store"
		// - "querying ... on https://cache.nixos.org"
		// - Progress bars "[0/1 built, 0.0 KiB DL]"
		// - Store paths "/nix/store/..."
		// - Empty lines from progress bar clearing
		// → silently dropped
	}
}

// stripAnsi removes ANSI escape sequences from a string.
func stripAnsi(s string) string {
	var out strings.Builder
	out.Grow(len(s))
	i := 0
	for i < len(s) {
		if s[i] == '\x1b' {
			// Skip ESC [ ... final_byte
			i++
			if i < len(s) && s[i] == '[' {
				i++
				for i < len(s) && !((s[i] >= 'A' && s[i] <= 'Z') || (s[i] >= 'a' && s[i] <= 'z')) {
					i++
				}
				if i < len(s) {
					i++ // skip final byte
				}
			}
		} else if s[i] == '\r' || s[i] == '\b' {
			i++ // skip carriage returns and backspaces
		} else {
			out.WriteByte(s[i])
			i++
		}
	}
	return out.String()
}

// RunSilent executes nix and captures output, returning it as a string.
func RunSilent(workspace string, args ...string) (string, error) {
	cmd := exec.Command(NixBin, args...)
	cmd.Dir = workspace
	cmd.Env = nixEnv()
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// nixEnv returns os.Environ() with NixBinPath prepended to PATH.
func nixEnv() []string {
	env := os.Environ()
	for i, e := range env {
		if strings.HasPrefix(e, "PATH=") {
			env[i] = "PATH=" + NixBinPath + ":" + e[len("PATH="):]
			return env
		}
	}
	return append(env, "PATH="+NixBinPath+":"+os.Getenv("PATH"))
}

func appendNixConf(line string) error {
	home, _ := os.UserHomeDir()
	confDir := filepath.Join(home, ".config", "nix")
	if err := os.MkdirAll(confDir, 0755); err != nil {
		return err
	}
	confPath := filepath.Join(confDir, "nix.conf")
	f, err := os.OpenFile(confPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintln(f, line)
	return err
}
