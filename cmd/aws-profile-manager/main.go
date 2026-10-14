// Command aws-profile-manager selects AWS profiles for interactive and
// automated workflows.
package main

import (
	"fmt"
	"os"

	"github.com/tiansuyu/aws_profile_manager/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(cli.ExitCode(err))
	}
}
