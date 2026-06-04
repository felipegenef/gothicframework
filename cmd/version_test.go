package cmd

import (
	"bytes"
	"io"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// captureStdout redirects os.Stdout for the duration of fn and returns what was
// written. versionCmd prints via fmt.Printf (real stdout), not cmd.OutOrStdout,
// so the Cobra output buffer alone won't capture it.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	fn()

	_ = w.Close()
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("copy: %v", err)
	}
	return buf.String()
}

func TestVersionCommandPrintsCurrentVersion(t *testing.T) {
	root := &cobra.Command{Use: "gothic"}
	// Reuse the real versionCmd; reset its parent linkage by adding to a fresh root.
	root.AddCommand(versionCmd)
	root.SetArgs([]string{"version"})
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})

	out := captureStdout(t, func() {
		if err := root.Execute(); err != nil {
			t.Fatalf("execute version: %v", err)
		}
	})

	if !strings.Contains(out, CURRENT_VERSION) {
		t.Errorf("version output %q does not contain %q", out, CURRENT_VERSION)
	}
	if !strings.Contains(out, "Gothic Framework") {
		t.Errorf("version output %q missing product name", out)
	}
}

// TestCurrentVersionFormat encodes the real format contract: CURRENT_VERSION
// must be a semantic version of the shape vMAJOR.MINOR.PATCH. This matters
// because migrate-v2 seeds go.mod requires with this value, which must be a
// version the Go module registry can resolve.
func TestCurrentVersionFormat(t *testing.T) {
	semver := regexp.MustCompile(`^v\d+\.\d+\.\d+$`)
	if !semver.MatchString(CURRENT_VERSION) {
		t.Errorf("CURRENT_VERSION %q must match vMAJOR.MINOR.PATCH", CURRENT_VERSION)
	}
}
