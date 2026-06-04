package helpers

import (
	"context"
	"os"
	"os/exec"
)

// commandRunner abstracts running an external command so AWS/SAM shell-out
// logic can be exercised in tests without a cloud account. The returned bytes
// are the command's standard output; callers that only care about success/error
// can ignore them.
type commandRunner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// execRunner is the production implementation backed by os/exec. It wires the
// child process's stderr/stdin to the parent so interactive AWS/SAM prompts and
// progress output continue to behave exactly as before. Stdout is captured and
// returned to the caller.
type execRunner struct{}

func (r execRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	// Forward stderr/stdin so interactive prompts and progress output from
	// aws/sam reach the user, matching the pre-seam behavior. Stdout is captured
	// via cmd.Output() and returned to the caller.
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Output()
}

// runner is the package-level default. Tests replace it via setRunner.
var runner commandRunner = execRunner{}

// setRunner swaps the package-level runner and returns a function that restores
// the previous one. Intended for tests only.
func setRunner(r commandRunner) func() {
	prev := runner
	runner = r
	return func() { runner = prev }
}
