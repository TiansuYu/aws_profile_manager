// Package aws wraps the AWS CLI for SSO login and identity checks.
package aws

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ErrCLINotFound is returned when the aws executable is not on PATH.
var ErrCLINotFound = errors.New("aws CLI not found on PATH; install it from https://aws.amazon.com/cli/")

// ensureCLI verifies the aws binary is available.
func ensureCLI() error {
	if _, err := exec.LookPath("aws"); err != nil {
		return ErrCLINotFound
	}
	return nil
}

// IsLoggedIn reports whether the profile already has valid credentials by
// calling sts get-caller-identity with a short timeout.
func IsLoggedIn(profile string) bool {
	if err := ensureCLI(); err != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "aws", "sts", "get-caller-identity",
		"--profile", profile, "--output", "text")
	return cmd.Run() == nil
}

// Login runs `aws sso login` for the given profile, streaming output so the
// user can complete the browser flow. stdout/stderr are wired to stderr to keep
// the program's stdout reserved for shell-eval output.
func Login(profile string) error {
	if err := ensureCLI(); err != nil {
		return err
	}
	cmd := exec.Command("aws", "sso", "login", "--profile", profile)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("aws sso login failed for profile %q: %w", profile, err)
	}
	return nil
}

// ResolveRegion returns the effective region for a profile, preferring the
// profile's configured region and falling back to the AWS_REGION /
// AWS_DEFAULT_REGION environment variables.
func ResolveRegion(profileRegion string) string {
	if r := strings.TrimSpace(profileRegion); r != "" {
		return r
	}
	for _, key := range []string{"AWS_REGION", "AWS_DEFAULT_REGION"} {
		if r := strings.TrimSpace(os.Getenv(key)); r != "" {
			return r
		}
	}
	return ""
}
