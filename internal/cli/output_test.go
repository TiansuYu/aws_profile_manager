package cli

import (
	"errors"
	"fmt"
	"testing"

	"github.com/tiansuyu/aws_profile_manager/internal/awsconfig"
)

type failingOutputWriter struct {
	failAt int
	calls  int
	err    error
}

func (w *failingOutputWriter) Write(p []byte) (int, error) {
	w.calls++
	if w.calls == w.failAt {
		return len(p) / 2, w.err
	}
	return len(p), nil
}

func TestShellEnvironmentStopsOnWriteError(t *testing.T) {
	// The shell script writes one unset line and three exports. Each write
	// can fail independently, including after some bytes have been accepted.
	for failAt := 1; failAt <= 4; failAt++ {
		t.Run(fmt.Sprintf("write_%d", failAt), func(t *testing.T) {
			writeErr := errors.New("output unavailable")
			writer := &failingOutputWriter{failAt: failAt, err: writeErr}
			profile := awsconfig.Profile{Name: "dev/example", Region: "eu-west-1"}

			err := writeEnvironment(writer, profile, "shell")
			if !errors.Is(err, writeErr) {
				t.Fatalf("writeEnvironment error = %v, want %v", err, writeErr)
			}
			if writer.calls != failAt {
				t.Errorf("made %d writes; must stop at failed write %d", writer.calls, failAt)
			}
		})
	}
}

func TestEnvCommandPropagatesOutputError(t *testing.T) {
	configureTestProfile(t)
	writeErr := errors.New("output unavailable")
	command := newRootCommand()
	command.SetOut(&failingOutputWriter{failAt: 2, err: writeErr})
	command.SetArgs([]string{"env", "--profile", "dev/example", "--login", "never"})
	if err := command.Execute(); !errors.Is(err, writeErr) {
		t.Fatalf("command error = %v, want %v", err, writeErr)
	}
}
