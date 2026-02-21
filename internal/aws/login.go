package aws

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// SSOAccount holds a known AWS account for SSO setup reference
type SSOAccount struct {
	Name    string
	Account string
}

// KnownSSOAccounts are Spark Rewards AWS accounts (for setup reference)
var KnownSSOAccounts = []SSOAccount{
	{Name: "beta", Account: "050451385382"},
	{Name: "prod", Account: "396608803858"},
	{Name: "central", Account: "417975668372"},
}

// SSOLogin runs `aws sso login` with the given profile
func SSOLogin(profile string) error {
	args := []string{"sso", "login"}
	if profile != "" {
		args = append(args, "--profile", profile)
	}

	cmd := exec.Command("aws", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// CheckCLI verifies that the AWS CLI is installed
func CheckCLI() error {
	_, err := exec.LookPath("aws")
	if err != nil {
		return fmt.Errorf("AWS CLI not found — install it with: brew install awscli")
	}
	return nil
}

// GetCallerIdentity runs `aws sts get-caller-identity` to verify credentials
func GetCallerIdentity(profile string) error {
	args := []string{"sts", "get-caller-identity"}
	if profile != "" {
		args = append(args, "--profile", profile)
	}

	cmd := exec.Command("aws", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// GetCallerIdentityQuiet verifies credentials without printing output
func GetCallerIdentityQuiet(profile string) error {
	args := []string{"sts", "get-caller-identity"}
	if profile != "" {
		args = append(args, "--profile", profile)
	}

	cmd := exec.Command("aws", args...)
	return cmd.Run()
}

// GetSSOProfiles returns a list of SSO-configured profiles from ~/.aws/config
func GetSSOProfiles() []string {
	configPath := filepath.Join(os.Getenv("HOME"), ".aws", "config")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil
	}

	var profiles []string
	re := regexp.MustCompile(`\[profile ([^\]]+)\]`)
	matches := re.FindAllStringSubmatch(string(data), -1)

	content := string(data)
	for _, match := range matches {
		profileName := match[1]
		profileHeader := fmt.Sprintf("[profile %s]", profileName)
		idx := strings.Index(content, profileHeader)
		if idx == -1 {
			continue
		}

		section := content[idx:]
		nextSection := strings.Index(section[1:], "[")
		if nextSection != -1 {
			section = section[:nextSection+1]
		}

		if strings.Contains(section, "sso_start_url") || strings.Contains(section, "sso_session") {
			profiles = append(profiles, profileName)
		}
	}

	return profiles
}

// IsSSOConfigured checks if a profile has SSO configuration
func IsSSOConfigured(profile string) bool {
	if profile == "" {
		profiles := GetSSOProfiles()
		return len(profiles) > 0
	}

	configPath := filepath.Join(os.Getenv("HOME"), ".aws", "config")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return false
	}

	profileHeader := fmt.Sprintf("[profile %s]", profile)
	content := string(data)
	idx := strings.Index(content, profileHeader)
	if idx == -1 {
		return false
	}

	section := content[idx:]
	nextSection := strings.Index(section[1:], "[")
	if nextSection != -1 {
		section = section[:nextSection+1]
	}

	return strings.Contains(section, "sso_start_url") || strings.Contains(section, "sso_session")
}

// PromptProfileSelection shows available profiles and lets user select one
func PromptProfileSelection() (string, error) {
	profiles := GetSSOProfiles()

	if len(profiles) == 0 {
		return "", fmt.Errorf("no SSO profiles found")
	}

	reader := bufio.NewReader(os.Stdin)

	fmt.Println("\nAvailable SSO profiles:")
	for i, p := range profiles {
		fmt.Printf("  %d. %s\n", i+1, p)
	}
	fmt.Println()

	fmt.Print("Enter the number of the profile to use: ")
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	idx, err := strconv.Atoi(input)
	if err != nil || idx < 1 || idx > len(profiles) {
		return "", fmt.Errorf("invalid selection — enter a number between 1 and %d", len(profiles))
	}

	return profiles[idx-1], nil
}

// PrintSSOAccountReference prints the known AWS account IDs (for identifying accounts in the wizard list)
func PrintSSOAccountReference() {
	fmt.Println("  Account reference (you'll pick from a list in the wizard; use these to identify which is which):")
	fmt.Println()
	for _, a := range KnownSSOAccounts {
		fmt.Printf("    %-8s %s\n", a.Name+":", a.Account)
	}
	fmt.Println()
}

// RunConfigureSSO runs `aws configure sso` interactively (wrapper for first-time or new profile setup)
func RunConfigureSSO() error {
	cmd := exec.Command("aws", "configure", "sso")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// ShowSSOSetupInstructions prints instructions matching the real aws configure sso wizard flow
func ShowSSOSetupInstructions() {
	fmt.Println()
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("  AWS SSO Setup (aws configure sso)")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()
	fmt.Println("  The wizard will prompt in this order:")
	fmt.Println()
	fmt.Println("    1. SSO session name (Recommended):  e.g. sparkrewards")
	fmt.Println("    2. SSO start URL [None]:           e.g. https://d-9067d5d83d.awsapps.com/start")
	fmt.Println("    3. SSO region [None]:              e.g. us-east-1")
	fmt.Println("    4. SSO registration scopes [None]: sso:account:access  (or press Enter)")
	fmt.Println("    5. Browser opens to sign in")
	fmt.Println("    6. Select AWS account from list    (see account IDs below to identify beta/prod/central)")
	fmt.Println("    7. Select IAM role from list")
	fmt.Println("    8. Default client Region [None]:   e.g. us-east-1")
	fmt.Println("    9. CLI default output format [None]: json")
	fmt.Println("   10. CLI profile name [...]:          e.g. beta/prod/central")
	fmt.Println()
	PrintSSOAccountReference()
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()
}

// ShowSSOSetupInstructionsNoRun prints the setup blurb and account reference only (no exec)
func ShowSSOSetupInstructionsNoRun() {
	fmt.Println()
	fmt.Println("  No SSO profiles found in ~/.aws/config")
	fmt.Println()
	PrintSSOAccountReference()
	fmt.Println("  Run 'spark-cli workspace configure sso' to set up a profile (runs aws configure sso),")
	fmt.Println("  or run 'aws configure sso' yourself. Then: spark-cli workspace configure --profile <name>")
	fmt.Println()
}

// ShowSSOSetupInstructionsShort prints account reference + short blurb for adding a profile
func ShowSSOSetupInstructionsShort() {
	fmt.Println()
	fmt.Println("  To add another profile: spark-cli workspace configure sso")
	PrintSSOAccountReference()
	fmt.Println("  Then: spark-cli workspace configure --profile <profile-name>")
	fmt.Println()
}
