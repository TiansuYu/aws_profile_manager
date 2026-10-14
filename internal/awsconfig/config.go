// Package awsconfig parses AWS CLI config files into a list of usable profiles.
package awsconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/ini.v1"
)

// Profile represents a single named profile from the AWS config file.
type Profile struct {
	Name       string // profile name without the "profile " prefix
	Region     string // region configured for the profile, if any
	SSOAccount string // sso_account_id, if any
	SSORole    string // sso_role_name, if any
	IsSSO      bool   // true when the profile is backed by SSO
}

// DefaultConfigPath resolves the AWS config path, honoring AWS_CONFIG_FILE.
func DefaultConfigPath() (string, error) {
	if env := os.Getenv("AWS_CONFIG_FILE"); env != "" {
		return env, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".aws", "config"), nil
}

// Load reads and parses all profiles from the given config path.
func Load(path string) ([]Profile, error) {
	cfg, err := ini.Load(path)
	if err != nil {
		return nil, fmt.Errorf("load aws config %q: %w", path, err)
	}

	var profiles []Profile
	for _, section := range cfg.Sections() {
		name, ok := profileName(section.Name())
		if !ok {
			continue
		}
		profiles = append(profiles, newProfile(name, section))
	}

	sort.Slice(profiles, func(i, j int) bool {
		return profiles[i].Name < profiles[j].Name
	})
	return profiles, nil
}

// profileName extracts the profile name from an ini section header, returning
// false for non-profile sections (e.g. "sso-session", "default-only" noise).
func profileName(section string) (string, bool) {
	switch {
	case section == "default":
		return "default", true
	case strings.HasPrefix(section, "profile "):
		return strings.TrimSpace(strings.TrimPrefix(section, "profile ")), true
	default:
		return "", false
	}
}

func newProfile(name string, section *ini.Section) Profile {
	get := func(key string) string {
		return strings.TrimSpace(section.Key(key).String())
	}
	p := Profile{
		Name:       name,
		Region:     get("region"),
		SSOAccount: get("sso_account_id"),
		SSORole:    get("sso_role_name"),
	}
	// Modern SSO uses sso_session; legacy SSO embeds sso_start_url directly.
	p.IsSSO = get("sso_session") != "" || get("sso_start_url") != ""
	return p
}
