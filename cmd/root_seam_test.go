package cmd

import (
	"strings"
	"testing"
)

// TestExecuteVersion runs Execute() against the real rootCmd with the `version`
// subcommand, which prints and returns nil so Execute takes its success path
// (no os.Exit). Args are restored on cleanup so other tests are unaffected.
//
// Beyond guarding against an unexpected os.Exit, this captures stdout and
// asserts the version string is actually emitted, so the test verifies real
// behavior end-to-end through Execute() rather than merely that it didn't crash.
func TestExecuteVersion(t *testing.T) {
	rootCmd.SetArgs([]string{"version"})
	t.Cleanup(func() { rootCmd.SetArgs(nil) })

	// version's RunE only prints the current version and returns nil, so
	// Execute() must not call os.Exit. captureStdout (version_test.go) lets us
	// also assert the printed output.
	out := captureStdout(t, func() {
		Execute()
	})

	if !strings.Contains(out, CURRENT_VERSION) {
		t.Errorf("Execute(version) output %q does not contain %q", out, CURRENT_VERSION)
	}
	if !strings.Contains(out, "Gothic Framework") {
		t.Errorf("Execute(version) output %q missing product name", out)
	}
}
