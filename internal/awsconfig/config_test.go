package awsconfig

import (
	"os"
	"path/filepath"
	"testing"
)

const sampleConfig = `[default]
region = us-east-1

[profile dev/example]
region         = eu-west-1
sso_session    = example
sso_account_id = 123456789012
sso_role_name  = ReadOnly

[profile no-region]
sso_session = example

[sso-session example]
sso_start_url = https://example.awsapps.com/start
sso_region    = eu-west-1
`

func writeTempConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	if err := os.WriteFile(path, []byte(sampleConfig), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}

func TestLoadParsesProfilesAndSkipsSSOSession(t *testing.T) {
	profiles, err := Load(writeTempConfig(t))
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	// default + dev/example + no-region, but NOT the sso-session section.
	if got, want := len(profiles), 3; got != want {
		t.Fatalf("profile count = %d, want %d (%+v)", got, want, profiles)
	}

	byName := map[string]Profile{}
	for _, p := range profiles {
		byName[p.Name] = p
	}

	live, ok := byName["dev/example"]
	if !ok {
		t.Fatal("expected dev/example profile")
	}
	if !live.IsSSO {
		t.Error("dev/example should be marked as SSO")
	}
	if live.Region != "eu-west-1" {
		t.Errorf("live region = %q, want eu-west-1", live.Region)
	}
	if live.SSOAccount != "123456789012" {
		t.Errorf("sso account = %q", live.SSOAccount)
	}

	if byName["no-region"].Region != "" {
		t.Error("no-region profile should not report a region")
	}
	if _, exists := byName["example"]; exists {
		t.Error("sso-session section must not be treated as a profile")
	}
}

func TestProfilesSortedByName(t *testing.T) {
	profiles, err := Load(writeTempConfig(t))
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	for i := 1; i < len(profiles); i++ {
		if profiles[i-1].Name > profiles[i].Name {
			t.Fatalf("profiles not sorted: %q before %q", profiles[i-1].Name, profiles[i].Name)
		}
	}
}
