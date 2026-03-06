package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Spark-Rewards/homebrew-spark-cli/internal/aws"
	"github.com/Spark-Rewards/homebrew-spark-cli/internal/git"
	"github.com/Spark-Rewards/homebrew-spark-cli/internal/workspace"
	"github.com/spf13/cobra"
)

var (
	workspaceCreateProfile string
	workspaceCreateRegion  string
	workspaceConfigureProfile string
	workspaceConfigureList    bool
)

var workspaceCmd = &cobra.Command{
	Use:     "workspace",
	Short:   "Manage workspace (ws, info | create | configure --profile, --list | -h)",
	Aliases: []string{"ws", "info"},
	Long: `Show workspace info or run a workspace subcommand.
Use 'workspace' or 'ws' (same command).

With no subcommand, lists the workspace name, repos, and AWS profile.

Examples:
  spark-cli workspace                    # or: spark-cli ws
  spark-cli ws create [path]             # create a new workspace
  spark-cli workspace configure --profile dev   # set default AWS profile`,
	RunE: func(cmd *cobra.Command, args []string) error {
		wsPath, err := workspace.Find()
		if err != nil {
			return err
		}

		ws, err := workspace.Load(wsPath)
		if err != nil {
			return err
		}

		fmt.Printf("%-15s %-30s %-25s %s\n", "WORKSPACE", "LOCATION", "AWS PROFILE", "ENVIRONMENT")
		fmt.Printf("%-15s %-30s %-25s %s\n", "---------", "--------", "------------", "-----------")
		fmt.Printf("%-15s %-30s %-25s %s\n", ws.Name, wsPath, orDefault(ws.AWSProfile, "(not set)"), orDefault(ws.SSMEnvPath, "beta"))
		fmt.Println()

		// List configured AWS profiles; mark the one selected for this workspace
		profiles := aws.GetSSOProfiles()
		if len(profiles) > 0 {
			fmt.Println("AWS profiles (swap with: spark-cli workspace configure --profile <name>):")
			for _, p := range profiles {
				mark := ""
				if p == ws.AWSProfile {
					mark = "  ← current"
				}
				fmt.Printf("  • %s%s\n", p, mark)
			}
			fmt.Println()
		}

		if len(ws.Repos) > 0 {
			fmt.Printf("%-20s %-15s %-10s %s\n", "REPO", "BRANCH", "STATUS", "PATH")
			fmt.Printf("%-20s %-15s %-10s %s\n", "----", "------", "------", "----")

			for name, repo := range ws.Repos {
				repoDir := filepath.Join(wsPath, repo.Path)
				branch := "-"
				status := "missing"

				if _, err := os.Stat(repoDir); err == nil {
					if git.IsRepo(repoDir) {
						b, _ := git.CurrentBranch(repoDir)
						if b != "" {
							branch = b
						}
						if git.IsDirty(repoDir) {
							status = "unstaged-changes"
						} else {
							status = "up-to-date"
						}
					}
				}

				fmt.Printf("%-20s %-15s %-10s %s\n", name, branch, status, repo.Path)
			}
		} else {
			fmt.Println("No repos — run 'spark-cli use <repo>' to add one")
		}

		return nil
	},
}

var workspaceCreateCmd = &cobra.Command{
	Use:   "create [path]",
	Short: "Create a new spark-cli workspace",
	Long: `Creates a new workspace directory with a .spk/workspace.json manifest.
If the directory doesn't exist, it will be created.

Examples:
  spark-cli workspace create .
  spark-cli workspace create ./my-project`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		targetPath := args[0]
		absPath, err := filepath.Abs(targetPath)
		if err != nil {
			return fmt.Errorf("invalid path: %w", err)
		}
		if err := os.MkdirAll(absPath, 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}
		manifestPath := workspace.ManifestPath(absPath)
		if _, err := os.Stat(manifestPath); err == nil {
			return fmt.Errorf("workspace already exists at %s", absPath)
		}
		name := filepath.Base(absPath)
		ws, err := workspace.Create(absPath, name, workspaceCreateProfile, workspaceCreateRegion)
		if err != nil {
			return err
		}
		if err := workspace.GenerateVSCodeWorkspace(absPath); err != nil {
			fmt.Printf("Warning: failed to create VS Code workspace: %v\n", err)
		}
		fmt.Printf("Workspace '%s' created at %s\n", ws.Name, absPath)
		fmt.Printf("  VS Code:     %s\n", workspace.VSCodeWorkspacePath(absPath))
		if ws.AWSProfile != "" {
			fmt.Printf("  AWS Profile: %s\n", ws.AWSProfile)
		}
		if ws.AWSRegion != "" {
			fmt.Printf("  AWS Region:  %s\n", ws.AWSRegion)
		}
		fmt.Println("\nNext steps:")
		fmt.Printf("  cd %s\n", absPath)
		fmt.Println("  spark-cli use <org/repo>")
		return nil
	},
}

var workspaceConfigureCmd = &cobra.Command{
	Use:   "configure",
	Short: "Set or list default AWS profile for this workspace",
	Long: `Set the default AWS profile for this workspace (used by sync), or list available profiles.
Setting a profile runs SSO login if credentials are missing or expired.

Examples:
  spark-cli workspace configure --list            # list profiles; if none, runs aws configure sso
  spark-cli workspace configure sso              # add a new profile (wrapper for aws configure sso)
  spark-cli workspace configure --profile dev    # set default profile to "dev"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if workspaceConfigureList {
			return runWorkspaceConfigureList()
		}
		if workspaceConfigureProfile != "" {
			return runWorkspaceConfigureProfile(workspaceConfigureProfile)
		}
		return cmd.Usage()
	},
}

var workspaceConfigureSSOCmd = &cobra.Command{
	Use:   "sso",
	Short: "Add a new AWS SSO profile (runs aws configure sso)",
	Long: `Runs 'aws configure sso' to add a new profile to ~/.aws/config.
Shows relevant account IDs (beta, prod, central) before starting.

After setup, run: spark-cli workspace configure --profile <name>`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := aws.CheckCLI(); err != nil {
			return err
		}
		aws.PrintSSOAccountReference()
		fmt.Println("Running: aws configure sso")
		fmt.Println()
		return aws.RunConfigureSSO()
	},
}

func runWorkspaceConfigureList() error {
	if err := aws.CheckCLI(); err != nil {
		return err
	}
	profiles := aws.GetSSOProfiles()
	fmt.Println("Available AWS SSO profiles (from ~/.aws/config):")
	if len(profiles) == 0 {
		fmt.Println("  (none)")
		aws.ShowSSOSetupInstructions()
		fmt.Println("Running aws configure sso...")
		return aws.RunConfigureSSO()
	}
	for _, p := range profiles {
		fmt.Printf("  • %s\n", p)
	}
	wsPath, err := workspace.Find()
	if err == nil {
		ws, err := workspace.Load(wsPath)
		if err == nil {
			if ws.AWSProfile != "" {
				fmt.Printf("\nCurrent workspace profile: %s\n", ws.AWSProfile)
			} else {
				fmt.Println("\nCurrent workspace profile: (not set)")
			}
		}
	} else {
		fmt.Println("\n(Not inside a workspace — run from a workspace to set a profile)")
	}
	aws.ShowSSOSetupInstructionsShort()
	return nil
}

func runWorkspaceConfigureProfile(profileName string) error {
	wsPath, err := workspace.Find()
	if err != nil {
		return err
	}
	if err := aws.CheckCLI(); err != nil {
		return err
	}
	ws, err := workspace.Load(wsPath)
	if err != nil {
		return err
	}
	profiles := aws.GetSSOProfiles()
	isSSO := false
	for _, p := range profiles {
		if p == profileName {
			isSSO = true
			break
		}
	}
	if !isSSO {
		fmt.Printf("Note: profile %q not found in ~/.aws/config (you can still set it).\n", profileName)
	}
	ws.AWSProfile = profileName
	if err := workspace.Save(wsPath, ws); err != nil {
		return fmt.Errorf("failed to save workspace: %w", err)
	}
	fmt.Printf("Workspace AWS profile set to: %s\n", profileName)

	// Auto-login for SSO profiles so credentials are valid for sync
	if isSSO {
		if err := aws.GetCallerIdentity(profileName); err != nil {
			fmt.Println("Logging in to AWS SSO...")
			if err := aws.SSOLogin(profileName); err != nil {
				return fmt.Errorf("SSO login failed: %w", err)
			}
			if err := aws.GetCallerIdentity(profileName); err != nil {
				return fmt.Errorf("verification failed after login: %w", err)
			}
			fmt.Println("✓ Login successful")
		}
	}
	fmt.Println("Use 'spark-cli workspace sync' with this profile.")
	return nil
}

func orDefault(val, def string) string {
	if val == "" {
		return def
	}
	return val
}

func init() {
	workspaceCmd.GroupID = "workspace"
	rootCmd.AddCommand(workspaceCmd)
	workspaceCmd.AddCommand(workspaceCreateCmd)
	workspaceCmd.AddCommand(workspaceConfigureCmd)
	workspaceConfigureCmd.AddCommand(workspaceConfigureSSOCmd)

	workspaceCreateCmd.Flags().StringVar(&workspaceCreateProfile, "aws-profile", "", "AWS SSO profile name")
	workspaceCreateCmd.Flags().StringVar(&workspaceCreateRegion, "aws-region", "", "Default AWS region")

	workspaceConfigureCmd.Flags().StringVar(&workspaceConfigureProfile, "profile", "", "Set the AWS profile name for this workspace")
	workspaceConfigureCmd.Flags().BoolVar(&workspaceConfigureList, "list", false, "List available AWS SSO profiles; if none, runs aws configure sso")
}
