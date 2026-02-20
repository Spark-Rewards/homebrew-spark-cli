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

// ShowSSOSetupInstructions prints instructions for setting up SSO
func ShowSSOSetupInstructions() {
	fmt.Println()
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("  AWS SSO Setup Required")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()
	fmt.Println("  No SSO profiles found in ~/.aws/config")
	fmt.Println()
	fmt.Println("  To set up AWS SSO, run:")
	fmt.Println()
	fmt.Println("    aws configure sso")
	fmt.Println()
	fmt.Println("  You'll need the following information from your AWS admin:")
	fmt.Println()
	fmt.Println("    • SSO Start URL    (e.g., https://mycompany.awsapps.com/start)")
	fmt.Println("    • SSO Region       (e.g., us-east-1)")
	fmt.Println("    • AWS Account ID   (12-digit number)")
	fmt.Println("    • IAM Role Name    (e.g., AdministratorAccess, DeveloperAccess)")
	fmt.Println()
	fmt.Println("  Example session:")
	fmt.Println()
	fmt.Println("    $ aws configure sso")
	fmt.Println("    SSO session name (Recommended): sparkrewards")
	fmt.Println("    SSO start URL [None]: https://sparkrewards.awsapps.com/start")
	fmt.Println("    SSO region [None]: us-east-1")
	fmt.Println("    (Browser opens for authentication)")
	fmt.Println("    ...")
	fmt.Println("    CLI default client Region [None]: us-east-1")
	fmt.Println("    CLI default output format [None]: json")
	fmt.Println("    CLI profile name [...]: sparkrewards-dev")
	fmt.Println()
	fmt.Println("  After setup, run 'spk login' again.")
	fmt.Println()
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()
}
