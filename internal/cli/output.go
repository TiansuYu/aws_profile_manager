package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	awscli "github.com/tiansuyu/aws_profile_manager/internal/aws"
	"github.com/tiansuyu/aws_profile_manager/internal/awsconfig"
)

type profileOutput struct {
	Name    string `json:"name"`
	Account string `json:"account,omitempty"`
	Region  string `json:"region,omitempty"`
	Role    string `json:"role,omitempty"`
	SSO     bool   `json:"sso"`
}

func writeProfiles(w io.Writer, profiles []awsconfig.Profile, output string) error {
	switch output {
	case "json":
		result := make([]profileOutput, 0, len(profiles))
		for _, profile := range profiles {
			result = append(result, profileOutput{
				Name:    profile.Name,
				Account: profile.SSOAccount,
				Region:  profile.Region,
				Role:    profile.SSORole,
				SSO:     profile.IsSSO,
			})
		}
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)
	case "text":
		table := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
		_, _ = fmt.Fprintln(table, "PROFILE\tACCOUNT\tREGION\tROLE")
		for _, profile := range profiles {
			_, _ = fmt.Fprintf(table, "%s\t%s\t%s\t%s\n",
				profile.Name, orDash(profile.SSOAccount), orDash(profile.Region), orDash(profile.SSORole))
		}
		return table.Flush()
	default:
		return fmt.Errorf("invalid output format %q: expected text or json", output)
	}
}

func writeEnvironment(w io.Writer, profile awsconfig.Profile, output string) error {
	environment := profileEnvironment(profile)
	switch output {
	case "json":
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(environment)
	case "shell":
		if _, err := fmt.Fprintf(w, "unset %s\n", strings.Join(awsCredentialOverrideKeys, " ")); err != nil {
			return err
		}
		for _, key := range awsEnvironmentKeys {
			if value := environment[key]; value != "" {
				if _, err := fmt.Fprintf(w, "export %s=%s\n", key, shellQuote(value)); err != nil {
					return err
				}
			}
		}
		return nil
	default:
		return fmt.Errorf("invalid output format %q: expected shell or json", output)
	}
}

func profileEnvironment(profile awsconfig.Profile) map[string]string {
	region := awscli.ResolveRegion(profile.Region)
	environment := map[string]string{
		"AWS_PROFILE":        profile.Name,
		"AWS_REGION":         region,
		"AWS_DEFAULT_REGION": region,
	}
	for _, key := range awsCredentialOverrideKeys {
		environment[key] = ""
	}
	return environment
}

func orDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
