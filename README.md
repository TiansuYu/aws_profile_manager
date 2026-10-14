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

Requires the [AWS CLI v2](https://aws.amazon.com/cli/). Building from source
requires Go ≥ 1.24.2.

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
`exec` example.

To apply a profile to the current human shell:

```bash
eval "$(aws-profile-manager env --profile dev/data-platform)"
```

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

```bash
make build
make test
make vet
```

Local Makefile builds use `git describe --tags --always --dirty`. Override the
version for release testing when needed:

```bash
VERSION=0.2.0-rc.1 make build
```

`go install …@vX.Y.Z` reports the module version recorded by Go. Local builds
use Go's recorded version when available (which may include Git metadata),
and otherwise report `dev`. Release builds embed their version at build time.

## Contributing

Issues and pull requests are welcome. Keep changes focused, format Go code with
`gofmt`, and run `go vet ./...`, `go test -race ./...`, and `golangci-lint run`
before submitting a pull request.

## License

Copyright 2026 Tiansu Yu. Licensed under the [Apache License 2.0](LICENSE).
