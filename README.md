# aws-profile-manager

A small CLI for using AWS profiles from interactive shells and AI agents. It
reads `~/.aws/config`, can fuzzy-search profiles, emits machine-readable profile
data, and runs commands with an explicit AWS environment.

```text
Select an AWS profile  ·  type to filter by name or account

  Profile       Account       Region     Role
▸ dev/example   123456789012  eu-west-1  ReadOnly
  test/example  123456789012  us-east-1  Developer

2 items
↑/k up • ↓/j down • / filter • q quit • ? more
```

## Install

Building from source requires Go ≥ 1.24.2. Credential checks, SSO login, and
AWS commands require the [AWS CLI v2](https://aws.amazon.com/cli/). Listing
profiles and `env --login never` work without it.

Install globally as `aws-profile-manager` via the Go toolchain:

```bash
go install github.com/tiansuyu/aws_profile_manager/cmd/aws-profile-manager@latest
```

This installs the binary to `GOBIN`, or
`$(go env GOPATH)/bin` when `GOBIN` is unset. Add the effective directory to
`PATH`:

```bash
GO_BIN="$(go env GOBIN)"
GO_BIN="${GO_BIN:-$(go env GOPATH)/bin}"
export PATH="$GO_BIN:$PATH"
```

Then verify the active installation:

```bash
command -v aws-profile-manager
aws-profile-manager list
aws-profile-manager --version
```

If an older installation shadows the Go binary, locate every copy with
`which -a aws-profile-manager`, then remove the obsolete copy or put `GO_BIN`
earlier in `PATH`.

Alternatively, download an archive for your OS and architecture from
[GitHub Releases](https://github.com/TiansuYu/aws_profile_manager/releases),
verify it against the release's `checksums.txt`, and extract the binary to a
directory on your `PATH`. Release binaries do not require Go.

## Usage

```bash
aws-profile-manager
aws-profile-manager --version
aws-profile-manager list
aws-profile-manager list --output json
aws-profile-manager env --profile dev/data-platform
aws-profile-manager env --profile dev/data-platform --output json
aws-profile-manager exec --profile dev/data-platform -- aws sts get-caller-identity
```

Running the tool without a command opens the interactive picker. It checks the
selected profile's credentials, starts SSO login when needed, and prints an
`exec` example. Press `/` to enter filter mode, then type or paste a profile
name or account ID. Press Enter to accept the filter, then Enter again to
select the highlighted profile. Outside filter mode, `q`, Esc, or Ctrl-C
cancels the picker.

To apply a profile to the current human shell:

```bash
profile_env=$(aws-profile-manager env --profile dev/data-platform) && eval "$profile_env"
```

The assignment checks the CLI's exit status before evaluating its output.
`env` reports output-write failures with a nonzero exit status.

`env` defaults to `--login never`, so it never unexpectedly opens a browser.
Use `--login auto` to verify credentials first or `--login force` to force a new
SSO session.

## AI agent usage

Agents should prefer `exec`. It scopes credentials to one child process and
does not depend on shell startup files or environment persistence:

```bash
aws-profile-manager exec \
  --profile dev/data-platform \
  --login auto \
  -- aws sts get-caller-identity --output json
```

For profile discovery, use JSON:

```bash
aws-profile-manager list --output json
```

For tools that need environment variables rather than a child command:

```bash
aws-profile-manager env --profile dev/data-platform --output json
```

The JSON object contains `AWS_PROFILE`, `AWS_REGION`, and `AWS_DEFAULT_REGION`;
credential variables that could override the profile are returned as empty
strings. The shell format safely quotes values and unsets those overrides.

### Login policies

- `auto`: reuse valid credentials or run `aws sso login`
- `force`: always run `aws sso login`
- `never`: do not check or modify credentials

`exec` defaults to `auto`; `env` defaults to `never`.

## Behavior

1. Parses `~/.aws/config`, honoring `AWS_CONFIG_FILE`.
2. Resolves the region from the profile, then `AWS_REGION` or
   `AWS_DEFAULT_REGION`.
3. Sets all three AWS environment variables for `exec` and removes inherited
   static/web-identity credential variables that would override the profile.
4. Forwards stdin, stdout, stderr, and the child exit status.

## Project layout

```
cmd/aws-profile-manager/     # thin entrypoint
internal/cli/                # commands and orchestration
internal/awsconfig/          # ~/.aws/config parser
internal/aws/                # aws CLI wrapper (sso login, identity, region)
internal/selector/           # interactive Bubble Tea picker
```

## Development

Run `make` or `make help` to list the available commands.

Install [prek](https://github.com/j178/prek), then enable the hooks in each clone:

```bash
# macOS
brew install prek
# Alternatively, with uv: uv tool install prek
make hooks-install
make check
```

Prek manages the pinned hook dependencies, including Gitleaks; a separate
Gitleaks installation is not required. The first run downloads those tools.
Go and Make must be on your `PATH`.

| Command | Purpose |
| --- | --- |
| `make build` | Build `bin/aws-profile-manager` |
| `make test` | Run unit and compiled-CLI integration tests |
| `make test-race` | Run all tests with the race detector |
| `make fmt` / `make fmt-check` | Format Go code / check formatting |
| `make vet` | Run Go's static checks |
| `make lint` | Run prek checks and scan the working tree for secrets |
| `make lint-go` | Run golangci-lint v2.14, installed separately |
| `make secrets` | Run only the working-tree Gitleaks scan |
| `make hooks-install` | Install the pre-commit hook in this clone |
| `make check` | Run basic lint, secret scanning, and race tests |
| `make run` | Open the interactive picker |
| `make install` / `make uninstall` | Install / remove the CLI |
| `make clean` | Remove local `bin/` and `dist/` artifacts |

The commit hook checks Go formatting, runs `go vet`, checks YAML and merge
conflicts, fixes trailing whitespace and missing final newlines, and scans
staged changes with Gitleaks. Review and re-stage files if a hook fixes them.
`make lint` and CI additionally scan the working tree, so secret checking also
runs when there are no staged changes. Hook versions are pinned in
`.pre-commit-config.yaml`.

Local Makefile builds use `git describe --tags --always --dirty`. Override the
version for release testing when needed:

```bash
VERSION=0.2.0-rc.1 make build
```

`go install …@vX.Y.Z` reports the module version recorded by Go. Local builds
use Go's recorded version when available (which may include Git metadata),
and otherwise report `dev`. Release builds embed their version at build time.

### Testing

Run the checks used by CI:

```bash
make check
make lint-go # requires golangci-lint v2.14 on PATH
```

CI tests the minimum Go version and current stable Go, and runs golangci-lint
v2.14 on stable Go. Release builds use stable Go and run only after the CI
checks pass.

The automated tests need no AWS credentials. They compile the CLI and use a
temporary AWS config and fake `aws` executable to exercise login policies,
login failures, child exit codes, credential isolation, JSON output, shell
quoting, and output-write errors. Picker tests cover navigation, filtering,
selection, late filter results, cancellation, and table layout. The subprocess
integration tests support macOS and Linux, matching the release targets.

For a live smoke test, choose a non-production profile and run:

```bash
make build
bin/aws-profile-manager list --output json
bin/aws-profile-manager exec --profile YOUR_SANDBOX_PROFILE --login never -- \
  aws sts get-caller-identity --output json
```

This checks identity without initiating login. If the session is expired,
repeat with `--login auto` and complete the SSO browser flow. Live AWS and
browser-login checks are manual; they are not part of the automated suite.
Run `bin/aws-profile-manager` to exercise the picker in a real terminal.
Selecting a profile may check credentials and start SSO login.

## Contributing

Issues and pull requests are welcome. Keep changes focused, enable the hooks
with `make hooks-install`, and run `make fmt`, `make check`, and `make lint-go`
before submitting a pull request.

## License

Copyright 2026 Tiansu Yu. Licensed under the [Apache License 2.0](LICENSE).
