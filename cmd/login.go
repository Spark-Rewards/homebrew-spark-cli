package cmd

import (
	"fmt"

	"github.com/Spark-Rewards/homebrew-spk/internal/aws"
	"github.com/Spark-Rewards/homebrew-spk/internal/workspace"
	"github.com/spf13/cobra"
)

var loginProfile string

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Login to AWS SSO",
	Long: `Logs in to AWS SSO. If no profile is specified, you'll be prompted to
select from available SSO profiles.

If no SSO profiles are configured, instructions will be shown for setup.

Note: 'spk sync' automatically handles login when refreshing environment,
so you typically don't need to run this separately.

Examples:
  spk login                  # select from available profiles
  spk login --profile dev    # login with specific profile`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := aws.CheckCLI(); err != nil {
			return err
		}

		profile := loginProfile

		if profile == "" {
			wsPath, err := workspace.Find()
			if err == nil {
				ws, err := workspace.Load(wsPath)
				if err == nil && ws.AWSProfile != "" {
					profile = ws.AWSProfile
					fmt.Printf("Using workspace profile: %s\n", profile)
				}
			}
		}

		if !aws.IsSSOConfigured(profile) {
			aws.ShowSSOSetupInstructions()
			return fmt.Errorf("no SSO profiles configured — run 'aws configure sso' first")
		}

		if profile == "" {
			selected, err := aws.PromptProfileSelection()
			if err != nil {
				return err
			}
			profile = selected
			fmt.Printf("\nUsing profile: %s\n", profile)
		}

		fmt.Println("Logging in to AWS SSO...")
		if err := aws.SSOLogin(profile); err != nil {
			return fmt.Errorf("login failed: %w", err)
		}

		fmt.Println("Verifying credentials...")
		if err := aws.GetCallerIdentity(profile); err != nil {
			return fmt.Errorf("verification failed: %w", err)
		}

		fmt.Println("\n✓ Login successful")

		wsPath, err := workspace.Find()
		if err == nil {
			ws, err := workspace.Load(wsPath)
			if err == nil && ws.AWSProfile == "" {
				ws.AWSProfile = profile
				workspace.Save(wsPath, ws)
				fmt.Printf("Saved profile '%s' to workspace\n", profile)
			}
		}

		return nil
	},
}

func init() {
	loginCmd.Flags().StringVar(&loginProfile, "profile", "", "AWS profile to use")
	rootCmd.AddCommand(loginCmd)
}
