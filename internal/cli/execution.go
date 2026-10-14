package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tiansuyu/aws_profile_manager/internal/awsconfig"
)

var awsEnvironmentKeys = []string{"AWS_PROFILE", "AWS_REGION", "AWS_DEFAULT_REGION"}

var awsCredentialOverrideKeys = []string{
	"AWS_ACCESS_KEY_ID",
	"AWS_SECRET_ACCESS_KEY",
	"AWS_SESSION_TOKEN",
	"AWS_SECURITY_TOKEN",
	"AWS_ROLE_ARN",
	"AWS_WEB_IDENTITY_TOKEN_FILE",
}

// ExitCode returns the wrapped command's exit status, or 1 for other errors.
func ExitCode(err error) int {
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		return exitError.ExitCode()
	}
	return 1
}

func runCommand(parent *cobra.Command, profile awsconfig.Profile, args []string) error {
	command := exec.CommandContext(parent.Context(), args[0], args[1:]...)
	command.Stdin = parent.InOrStdin()
	command.Stdout = parent.OutOrStdout()
	command.Stderr = parent.ErrOrStderr()
	command.Env = mergedEnvironment(os.Environ(), profileEnvironment(profile))
	if err := command.Run(); err != nil {
		return fmt.Errorf("run %q: %w", args[0], err)
	}
	return nil
}

func mergedEnvironment(current []string, replacements map[string]string) []string {
	environment := make([]string, 0, len(current)+len(replacements))
	for _, entry := range current {
		key, _, _ := strings.Cut(entry, "=")
		if _, replaced := replacements[key]; replaced {
			continue
		}
		if slices.Contains(awsCredentialOverrideKeys, key) {
			continue
		}
		environment = append(environment, entry)
	}
	for _, key := range awsEnvironmentKeys {
		if value := replacements[key]; value != "" {
			environment = append(environment, key+"="+value)
		}
	}
	return environment
}
