package cli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestCLIEndToEnd exercises the compiled entry point with an isolated AWS
// config and a stand-in aws executable. It never contacts AWS.
func TestCLIEndToEnd(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake aws executable uses /bin/sh")
	}

	bin := filepath.Join(t.TempDir(), "aws-profile-manager")
	buildContext, cancelBuild := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancelBuild()
	build := exec.CommandContext(buildContext, "go", "build", "-o", bin, "./cmd/aws-profile-manager")
	build.Dir = filepath.Join("..", "..")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, output)
	}

	newFixture := func(t *testing.T) (string, string, string) {
		t.Helper()
		dir := t.TempDir()
		config := filepath.Join(dir, "config")
		contents := `[profile dev/example]
region = eu-west-1
sso_session = example
sso_account_id = 123456789012
sso_role_name = Agent

[profile static/example]
region = us-east-1

[profile team's example]
region = eu-central-1
`
		if err := os.WriteFile(config, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
		fakeBin := filepath.Join(dir, "bin")
		if err := os.Mkdir(fakeBin, 0o700); err != nil {
			t.Fatal(err)
		}
		fakeAWS := `#!/bin/sh
printf '%s\n' "$*" >> "$FAKE_AWS_LOG"
if [ "$1 $2" = "sts get-caller-identity" ]; then
    exit "${FAKE_STS_EXIT:-0}"
fi
if [ "$1 $2" = "sso login" ]; then
    printf 'fake login stdout\n'
    exit "${FAKE_LOGIN_EXIT:-0}"
fi
exit 91
`
		if err := os.WriteFile(filepath.Join(fakeBin, "aws"), []byte(fakeAWS), 0o700); err != nil {
			t.Fatal(err)
		}
		return config, fakeBin, filepath.Join(dir, "aws.log")
	}

	run := func(t *testing.T, overrides []string, args ...string) (string, string, int, string) {
		t.Helper()
		config, fakeBin, logPath := newFixture(t)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, bin, args...)
		cmd.Env = append(os.Environ(),
			"AWS_CONFIG_FILE="+config,
			"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
			"FAKE_AWS_LOG="+logPath,
			"FAKE_STS_EXIT=0", "FAKE_LOGIN_EXIT=0",
			"AWS_REGION=", "AWS_DEFAULT_REGION=",
		)
		cmd.Env = append(cmd.Env, overrides...)
		var stdout, stderr strings.Builder
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		status := 0
		if err != nil {
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) {
				t.Fatalf("start CLI: %v", err)
			}
			status = exitErr.ExitCode()
		}
		log, readErr := os.ReadFile(logPath)
		if readErr != nil && !os.IsNotExist(readErr) {
			t.Fatalf("read fake aws log: %v", readErr)
		}
		return stdout.String(), stderr.String(), status, string(log)
	}

	t.Run("list emits sorted JSON profiles", func(t *testing.T) {
		stdout, stderr, status, calls := run(t, nil, "list", "--output", "json")
		var profiles []profileOutput
		if err := json.Unmarshal([]byte(stdout), &profiles); err != nil || status != 0 || calls != "" {
			t.Fatalf("status=%d JSON error=%v stdout=%q stderr=%q calls=%q", status, err, stdout, stderr, calls)
		}
		if len(profiles) != 3 || profiles[0].Name != "dev/example" || !profiles[0].SSO ||
			profiles[1].Name != "static/example" || profiles[1].SSO || profiles[2].Name != "team's example" {
			t.Errorf("unexpected profile list: %#v", profiles)
		}
	})

	t.Run("login policies", func(t *testing.T) {
		for _, tc := range []struct {
			name, profile, policy, stsExit, wantCalls string
		}{
			{"env defaults to never", "dev/example", "", "1", ""},
			{"auto reuses valid SSO", "dev/example", "auto", "0", "sts get-caller-identity --profile dev/example --output text\n"},
			{"auto renews expired SSO", "dev/example", "auto", "1", "sts get-caller-identity --profile dev/example --output text\nsso login --profile dev/example\n"},
			{"force renews valid SSO", "dev/example", "force", "0", "sso login --profile dev/example\n"},
			{"auto accepts static profile", "static/example", "auto", "1", "sts get-caller-identity --profile static/example --output text\n"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				args := []string{"env", "--profile", tc.profile, "--output", "json"}
				if tc.policy != "" {
					args = append(args, "--login", tc.policy)
				}
				stdout, stderr, status, calls := run(t, []string{"FAKE_STS_EXIT=" + tc.stsExit}, args...)
				if status != 0 || calls != tc.wantCalls {
					t.Errorf("status=%d calls=%q; want status=0 calls=%q; stderr=%q", status, calls, tc.wantCalls, stderr)
				}
				var env map[string]string
				if err := json.Unmarshal([]byte(stdout), &env); err != nil {
					t.Fatalf("stdout is not clean JSON: %v; stdout=%q stderr=%q", err, stdout, stderr)
				}
				if env["AWS_PROFILE"] != tc.profile {
					t.Errorf("AWS_PROFILE=%q; want %q", env["AWS_PROFILE"], tc.profile)
				}
			})
		}
	})

	t.Run("force rejects non-SSO profile", func(t *testing.T) {
		stdout, stderr, status, calls := run(t, nil,
			"env", "--profile", "static/example", "--login", "force")
		if status == 0 || stdout != "" || calls != "" || !strings.Contains(stderr, "not configured for SSO") {
			t.Errorf("status=%d stdout=%q stderr=%q calls=%q", status, stdout, stderr, calls)
		}
	})

	t.Run("login failure blocks output and command", func(t *testing.T) {
		stdout, stderr, status, calls := run(t, []string{"FAKE_STS_EXIT=1", "FAKE_LOGIN_EXIT=9"},
			"exec", "--profile", "dev/example", "--", "/bin/sh", "-c", "printf 'child ran'")
		if status == 0 || stdout != "" || !strings.Contains(stderr, "aws sso login failed") ||
			!strings.Contains(calls, "sso login --profile dev/example") {
			t.Errorf("status=%d stdout=%q stderr=%q calls=%q", status, stdout, stderr, calls)
		}
	})

	t.Run("exec clears inherited credentials and preserves child exit", func(t *testing.T) {
		stdout, stderr, status, calls := run(t, []string{
			"AWS_ACCESS_KEY_ID=stale", "AWS_SECRET_ACCESS_KEY=stale", "AWS_SESSION_TOKEN=stale",
			"AWS_SECURITY_TOKEN=stale", "AWS_ROLE_ARN=stale", "AWS_WEB_IDENTITY_TOKEN_FILE=stale",
		}, "exec", "--profile", "dev/example", "--login", "never", "--", "/bin/sh", "-c",
			`printf '%s|%s|%s|%s|%s|%s|%s|%s' "$AWS_PROFILE" "$AWS_REGION" "${AWS_ACCESS_KEY_ID-unset}" "${AWS_SECRET_ACCESS_KEY-unset}" "${AWS_SESSION_TOKEN-unset}" "${AWS_SECURITY_TOKEN-unset}" "${AWS_ROLE_ARN-unset}" "${AWS_WEB_IDENTITY_TOKEN_FILE-unset}"; exit 7`)
		if status != 7 || stdout != "dev/example|eu-west-1|unset|unset|unset|unset|unset|unset" || calls != "" ||
			!strings.Contains(stderr, "exit status 7") {
			t.Errorf("status=%d stdout=%q stderr=%q calls=%q", status, stdout, stderr, calls)
		}
	})

	t.Run("login stdout stays off machine-readable stdout", func(t *testing.T) {
		stdout, stderr, status, calls := run(t, nil,
			"env", "--profile", "dev/example", "--login", "force", "--output", "json")
		var env map[string]string
		if err := json.Unmarshal([]byte(stdout), &env); err != nil || status != 0 ||
			!strings.Contains(stderr, "fake login stdout") || calls != "sso login --profile dev/example\n" {
			t.Errorf("status=%d JSON error=%v stdout=%q stderr=%q calls=%q", status, err, stdout, stderr, calls)
		}
	})

	t.Run("shell output applies quote-containing profile and clears credentials", func(t *testing.T) {
		script, stderr, status, calls := run(t, nil,
			"env", "--profile", "team's example")
		if status != 0 || calls != "" {
			t.Fatalf("env status=%d stderr=%q calls=%q", status, stderr, calls)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "/bin/sh", "-c", script+`printf '%s|%s|%s' "$AWS_PROFILE" "$AWS_REGION" "${AWS_ACCESS_KEY_ID-unset}"`)
		cmd.Env = append(os.Environ(), "AWS_ACCESS_KEY_ID=stale")
		output, err := cmd.CombinedOutput()
		if err != nil || string(output) != "team's example|eu-central-1|unset" {
			t.Errorf("eval output=%q err=%v; shell script=%q", output, err, script)
		}
	})

	t.Run("shell output write failure exits nonzero", func(t *testing.T) {
		config, fakeBin, logPath := newFixture(t)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, bin, "env", "--profile", "dev/example")
		cmd.Env = append(os.Environ(),
			"AWS_CONFIG_FILE="+config,
			"PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"),
			"FAKE_AWS_LOG="+logPath,
		)
		readOnly, err := os.Open(os.DevNull)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := readOnly.Close(); err != nil {
				t.Errorf("close read-only stdout: %v", err)
			}
		})
		cmd.Stdout = readOnly
		var stderr strings.Builder
		cmd.Stderr = &stderr
		err = cmd.Run()
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() == 0 || !strings.Contains(stderr.String(), "error:") {
			t.Errorf("want nonzero CLI exit on stdout write error; err=%v stderr=%q", err, stderr.String())
		}
	})

	t.Run("invalid arguments fail before AWS", func(t *testing.T) {
		for _, tc := range []struct {
			name    string
			args    []string
			message string
		}{
			{"missing profile", []string{"env"}, "required flag"},
			{"unknown profile", []string{"env", "--profile", "absent"}, "not found"},
			{"invalid policy", []string{"env", "--profile", "dev/example", "--login", "sometimes"}, "invalid login policy"},
			{"missing command", []string{"exec", "--profile", "dev/example"}, "requires at least 1 arg"},
			{"invalid output", []string{"list", "--output", "yaml"}, "invalid output format"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				stdout, stderr, status, calls := run(t, nil, tc.args...)
				if status == 0 || stdout != "" || calls != "" || !strings.Contains(stderr, tc.message) {
					t.Errorf("status=%d stdout=%q stderr=%q calls=%q; want error containing %q", status, stdout, stderr, calls, tc.message)
				}
			})
		}
	})
}
