package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tiansuyu/aws_profile_manager/internal/awsconfig"
)

func configureTestProfile(t *testing.T) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config")
	config := `[profile dev/example]
region = eu-west-1
sso_session = example
sso_account_id = 123456789012
sso_role_name = Agent
`
	if err := os.WriteFile(path, []byte(config), 0o600); err != nil {
		t.Fatalf("write AWS config: %v", err)
	}
	t.Setenv("AWS_CONFIG_FILE", path)
}

func TestShellQuote(t *testing.T) {
	tests := map[string]string{
		"dev/example":    "'dev/example'",
		"eu-west-1":      "'eu-west-1'",
		"team's profile": "'team'\\''s profile'",
	}

	for input, want := range tests {
		if got := shellQuote(input); got != want {
			t.Errorf("shellQuote(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestWriteEnvironmentShell(t *testing.T) {
	var output bytes.Buffer
	profile := awsconfig.Profile{Name: "dev/example", Region: "eu-west-1"}

	if err := writeEnvironment(&output, profile, "shell"); err != nil {
		t.Fatalf("writeEnvironment returned error: %v", err)
	}

	for _, want := range []string{
		"unset AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY",
		"export AWS_PROFILE='dev/example'",
		"export AWS_REGION='eu-west-1'",
		"export AWS_DEFAULT_REGION='eu-west-1'",
	} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("shell output missing %q:\n%s", want, output.String())
		}
	}
}

func TestWriteEnvironmentJSON(t *testing.T) {
	var output bytes.Buffer
	profile := awsconfig.Profile{Name: "dev/engineer", Region: "eu-west-1"}

	if err := writeEnvironment(&output, profile, "json"); err != nil {
		t.Fatalf("writeEnvironment returned error: %v", err)
	}

	var got map[string]string
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatalf("decode environment JSON: %v", err)
	}
	if got["AWS_PROFILE"] != profile.Name || got["AWS_DEFAULT_REGION"] != profile.Region {
		t.Errorf("unexpected environment: %#v", got)
	}
}

func TestMergedEnvironmentReplacesAWSValues(t *testing.T) {
	current := []string{
		"PATH=/usr/bin",
		"AWS_PROFILE=old",
		"AWS_REGION=us-east-1",
		"AWS_ACCESS_KEY_ID=stale",
		"AWS_SESSION_TOKEN=stale",
	}
	replacements := map[string]string{
		"AWS_PROFILE":        "new",
		"AWS_REGION":         "eu-west-1",
		"AWS_DEFAULT_REGION": "eu-west-1",
	}

	got := mergedEnvironment(current, replacements)
	for _, want := range []string{
		"PATH=/usr/bin",
		"AWS_PROFILE=new",
		"AWS_REGION=eu-west-1",
		"AWS_DEFAULT_REGION=eu-west-1",
	} {
		if !slices.Contains(got, want) {
			t.Errorf("merged environment missing %q: %#v", want, got)
		}
	}
	if slices.Contains(got, "AWS_PROFILE=old") || slices.Contains(got, "AWS_REGION=us-east-1") {
		t.Errorf("merged environment retained stale values: %#v", got)
	}
	if slices.Contains(got, "AWS_ACCESS_KEY_ID=stale") || slices.Contains(got, "AWS_SESSION_TOKEN=stale") {
		t.Errorf("merged environment retained credential overrides: %#v", got)
	}
}

func TestListCommandEmitsMachineReadableProfiles(t *testing.T) {
	configureTestProfile(t)
	var output bytes.Buffer
	command := newRootCommand()
	command.SetOut(&output)
	command.SetArgs([]string{"list", "--output", "json"})

	if err := command.Execute(); err != nil {
		t.Fatalf("execute list command: %v", err)
	}

	var profiles []profileOutput
	if err := json.Unmarshal(output.Bytes(), &profiles); err != nil {
		t.Fatalf("decode profiles: %v", err)
	}
	if len(profiles) != 1 || profiles[0].Name != "dev/example" || !profiles[0].SSO {
		t.Errorf("unexpected profiles: %#v", profiles)
	}
}

func TestHelpDocumentsAgentWorkflow(t *testing.T) {
	tests := map[string]struct {
		args []string
		want []string
	}{
		"root": {
			args: []string{"--help"},
			want: []string{"AI agents and scripts should use list, env, or exec", "list --output json"},
		},
		"list": {
			args: []string{"list", "--help"},
			want: []string{"machine-readable objects", "jq -r '.[].name'"},
		},
		"env": {
			args: []string{"env", "--help"},
			want: []string{`default login policy is "never"`, "json for agents"},
		},
		"exec": {
			args: []string{"exec", "--help"},
			want: []string{"recommended interface for AI agents", "Place -- before COMMAND"},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var output bytes.Buffer
			command := newRootCommand()
			command.SetOut(&output)
			command.SetArgs(test.args)

			if err := command.Execute(); err != nil {
				t.Fatalf("execute help: %v", err)
			}
			for _, want := range test.want {
				if !strings.Contains(output.String(), want) {
					t.Errorf("help output missing %q:\n%s", want, output.String())
				}
			}
		})
	}
}

func TestResolveVersion(t *testing.T) {
	for _, tc := range []struct {
		name, override, module, want string
	}{
		{"build override", "v2.0.0", "v1.0.0", "v2.0.0"},
		{"installed module", "", "v1.2.3", "v1.2.3"},
		{"development build", "", "(devel)", "dev"},
		{"missing metadata", "", "", "dev"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveVersion(tc.override, tc.module); got != tc.want {
				t.Errorf("version = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestVersionFlag(t *testing.T) {
	var output bytes.Buffer
	command := newRootCommand()
	command.SetOut(&output)
	command.SetArgs([]string{"--version"})

	if err := command.Execute(); err != nil {
		t.Fatalf("execute version flag: %v", err)
	}
	if got, want := output.String(), "aws-profile-manager dev\n"; got != want {
		t.Errorf("version output = %q, want %q", got, want)
	}
}

func TestExecCommandSetsScopedEnvironment(t *testing.T) {
	configureTestProfile(t)
	var output bytes.Buffer
	command := newRootCommand()
	command.SetOut(&output)
	command.SetArgs([]string{
		"exec", "--profile", "dev/example", "--login", "never", "--",
		"env",
	})

	if err := command.Execute(); err != nil {
		t.Fatalf("execute child command: %v", err)
	}
	for _, want := range []string{
		"AWS_PROFILE=dev/example",
		"AWS_REGION=eu-west-1",
		"AWS_DEFAULT_REGION=eu-west-1",
	} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("child environment missing %q", want)
		}
	}
}

func TestExitCodePreservesChildStatus(t *testing.T) {
	command := exec.Command(os.Args[0], "-test.run=TestExitCodeHelper")
	command.Env = append(os.Environ(), "TEST_EXIT_CODE_HELPER=1")
	err := command.Run()
	if got, want := ExitCode(err), 7; got != want {
		t.Errorf("ExitCode() = %d, want %d", got, want)
	}
}

func TestExitCodeHelper(t *testing.T) {
	if os.Getenv("TEST_EXIT_CODE_HELPER") == "1" {
		os.Exit(7)
	}
}
