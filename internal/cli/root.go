// Package cli defines the CLI surface for aws-profile-manager.
package cli

import (
	"fmt"
	"os"
	"runtime/debug"

	"github.com/spf13/cobra"

	awscli "github.com/tiansuyu/aws_profile_manager/internal/aws"
	"github.com/tiansuyu/aws_profile_manager/internal/awsconfig"
	"github.com/tiansuyu/aws_profile_manager/internal/selector"
)

// Version can be set at build time with -ldflags -X.
var Version string

func version() string {
	moduleVersion := ""
	if info, ok := debug.ReadBuildInfo(); ok {
		moduleVersion = info.Main.Version
	}
	return resolveVersion(Version, moduleVersion)
}

func resolveVersion(override, moduleVersion string) string {
	if override != "" {
		return override
	}
	if moduleVersion != "" && moduleVersion != "(devel)" {
		return moduleVersion
	}
	return "dev"
}

type options struct {
	profile string
	login   string
	output  string
}

// Execute is the entry point invoked by main.
func Execute() error {
	return newRootCommand().Execute()
}

func newRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:     "aws-profile-manager",
		Short:   "Select AWS profiles for humans and automation",
		Version: version(),
		Long: `Select AWS profiles for humans and automation.

Running without a subcommand opens an interactive profile picker and may start
an AWS SSO browser login. AI agents and scripts should use list, env, or exec;
those commands never open the picker and support deterministic output.`,
		Example: `  # Discover profiles as structured data
  aws-profile-manager list --output json

  # Run one command with a selected profile
  aws-profile-manager exec --profile dev/data-platform -- aws sts get-caller-identity`,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			profiles, err := loadProfiles()
			if err != nil {
				return err
			}
			chosen, err := selectProfile(profiles)
			if err != nil {
				return err
			}
			if err := ensureLogin(chosen, "auto"); err != nil {
				return err
			}
			printGuidance(chosen)
			return nil
		},
	}
	root.SetVersionTemplate("aws-profile-manager {{.Version}}\n")
	root.CompletionOptions.DisableDefaultCmd = true
	root.AddCommand(newListCommand(), newEnvCommand(), newExecCommand())
	return root
}

func newListCommand() *cobra.Command {
	opts := &options{}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List configured profiles",
		Long: `List profiles parsed from the AWS config file.

The command honors AWS_CONFIG_FILE. Use --output json for stable,
machine-readable objects with name, account, region, role, and sso fields.`,
		Example: `  aws-profile-manager list
  aws-profile-manager list --output json
  aws-profile-manager list --output json | jq -r '.[].name'`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			profiles, err := loadProfiles()
			if err != nil {
				return err
			}
			return writeProfiles(cmd.OutOrStdout(), profiles, opts.output)
		},
	}
	cmd.Flags().StringVarP(&opts.output, "output", "o", "text", "output format: text for humans, json for agents")
	return cmd
}

func newEnvCommand() *cobra.Command {
	opts := &options{}
	cmd := &cobra.Command{
		Use:   "env",
		Short: "Print environment variables for a profile",
		Long: `Print the environment required to use one AWS profile.

Shell output emits safe unset/export statements for eval. JSON output maps
AWS_PROFILE, AWS_REGION, and AWS_DEFAULT_REGION to their values and maps
credential variables that would override the profile to empty strings.

The default login policy is "never", so this command does not open a browser.`,
		Example: `  # Apply the profile to the current shell
  eval "$(aws-profile-manager env --profile dev/data-platform)"

  # Read the environment from an agent
  aws-profile-manager env --profile dev/data-platform --output json

  # Validate credentials before emitting the environment
  aws-profile-manager env --profile dev/data-platform --login auto`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			profile, err := namedProfile(opts.profile)
			if err != nil {
				return err
			}
			if err := ensureLogin(profile, opts.login); err != nil {
				return err
			}
			return writeEnvironment(cmd.OutOrStdout(), profile, opts.output)
		},
	}
	addProfileFlag(cmd, opts)
	addLoginFlag(cmd, opts, "never")
	cmd.Flags().StringVarP(&opts.output, "output", "o", "shell", "output format: shell for eval, json for agents")
	return cmd
}

func newExecCommand() *cobra.Command {
	opts := &options{}
	cmd := &cobra.Command{
		Use:   "exec --profile NAME -- COMMAND [ARG...]",
		Short: "Run a command with a profile's AWS environment",
		Long: `Run a child command with an explicit AWS profile and region.

This is the recommended interface for AI agents. It does not depend on shell
startup files or environment persistence. Inherited static and web-identity
credential variables are removed so they cannot override the selected profile.
stdin, stdout, stderr, and the child exit status are preserved.

Place -- before COMMAND when the child command has flags.`,
		Example: `  aws-profile-manager exec --profile dev/data-platform -- \
    aws sts get-caller-identity --output json

  # Fail immediately instead of starting SSO login
  aws-profile-manager exec --profile dev/data-platform --login never -- \
    aws s3 ls`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			profile, err := namedProfile(opts.profile)
			if err != nil {
				return err
			}
			if err := ensureLogin(profile, opts.login); err != nil {
				return err
			}
			return runCommand(cmd, profile, args)
		},
	}
	addProfileFlag(cmd, opts)
	addLoginFlag(cmd, opts, "auto")
	return cmd
}

func addProfileFlag(cmd *cobra.Command, opts *options) {
	cmd.Flags().StringVarP(&opts.profile, "profile", "p", "", "AWS profile name (required)")
	_ = cmd.MarkFlagRequired("profile")
}

func addLoginFlag(cmd *cobra.Command, opts *options, defaultValue string) {
	cmd.Flags().StringVar(
		&opts.login,
		"login",
		defaultValue,
		"credential policy: auto reuses or starts SSO, force starts SSO, never skips checks",
	)
}

func loadProfiles() ([]awsconfig.Profile, error) {
	path, err := awsconfig.DefaultConfigPath()
	if err != nil {
		return nil, err
	}
	profiles, err := awsconfig.Load(path)
	if err != nil {
		return nil, err
	}
	if len(profiles) == 0 {
		return nil, fmt.Errorf("no profiles found in %s", path)
	}
	return profiles, nil
}

func namedProfile(name string) (awsconfig.Profile, error) {
	profiles, err := loadProfiles()
	if err != nil {
		return awsconfig.Profile{}, err
	}
	for _, profile := range profiles {
		if profile.Name == name {
			return profile, nil
		}
	}
	return awsconfig.Profile{}, fmt.Errorf("profile %q not found in config", name)
}

func selectProfile(profiles []awsconfig.Profile) (awsconfig.Profile, error) {
	chosen, err := selector.Select(profiles)
	if err != nil {
		return awsconfig.Profile{}, err
	}
	if chosen == nil {
		return awsconfig.Profile{}, fmt.Errorf("no profile selected")
	}
	return *chosen, nil
}

func ensureLogin(profile awsconfig.Profile, policy string) error {
	switch policy {
	case "never":
		return nil
	case "auto":
		if awscli.IsLoggedIn(profile.Name) {
			fmt.Fprintf(os.Stderr, "profile %s already has valid credentials\n", profile.Name)
			return nil
		}
		if !profile.IsSSO {
			return nil
		}
	case "force":
	default:
		return fmt.Errorf("invalid login policy %q: expected auto, force, or never", policy)
	}
	if !profile.IsSSO {
		return fmt.Errorf("profile %q is not configured for SSO; use --login never", profile.Name)
	}
	fmt.Fprintf(os.Stderr, "running aws sso login for %s\n", profile.Name)
	return awscli.Login(profile.Name)
}

func printGuidance(profile awsconfig.Profile) {
	region := awscli.ResolveRegion(profile.Region)
	fmt.Fprintf(os.Stderr, "\nactive profile: %s\n", profile.Name)
	if region == "" {
		fmt.Fprintln(os.Stderr, "no region is configured")
		return
	}
	fmt.Fprintf(os.Stderr, "region: %s\n", region)
	fmt.Fprintf(os.Stderr, "run commands with: aws-profile-manager exec -p %s -- COMMAND\n", shellQuote(profile.Name))
}
