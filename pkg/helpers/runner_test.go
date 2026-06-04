package helpers

import (
	"context"
	"runtime"
	"strings"
	"testing"
)

// TestExecRunnerRun exercises the real execRunner against a harmless,
// universally available command so the production seam is covered without
// touching AWS/SAM or the network.
func TestExecRunnerRun(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping echo-based test on windows")
	}
	r := execRunner{}
	out, err := r.Run(context.Background(), "echo", "hello-runner")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(out), "hello-runner") {
		t.Errorf("got %q, want substring %q", string(out), "hello-runner")
	}
}

func TestExecRunnerRunError(t *testing.T) {
	r := execRunner{}
	_, err := r.Run(context.Background(), "this-binary-does-not-exist-xyz")
	if err == nil {
		t.Fatal("expected error for missing binary")
	}
}

func TestSetRunnerRestore(t *testing.T) {
	fr := &fakeRunner{}
	restore := setRunner(fr)
	if runner != commandRunner(fr) {
		t.Fatal("setRunner did not install fake")
	}
	restore()
	if _, ok := runner.(execRunner); !ok {
		t.Fatal("restore did not reinstall execRunner")
	}
}
