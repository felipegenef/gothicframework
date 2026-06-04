package helpers

import (
	"os/exec"
	"strings"
	"sync"
	"testing"
)

// buildCommandForCompiler only *constructs* an *exec.Cmd; it does not run the
// toolchain (that happens in runWithVendorFallback via cmd.Run(), which is
// integration-gated). These tests inspect the constructed command without
// executing it, so they remain hermetic.

func TestBuildCommandForCompiler_GothicTinyGo(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("GOTHIC_CLI_CACHE_DIR", tmp)
	h := linuxAmd64Helper()

	cmd, err := h.buildCommandForCompiler(WasmCompilerGothicTinyGo, "./gen/", "/out/app.wasm", tmp, &sync.Once{})
	if err != nil {
		t.Fatalf("buildCommandForCompiler(GothicTinyGo): %v", err)
	}
	if cmd.Dir != tmp {
		t.Errorf("cmd.Dir: got %q, want %q", cmd.Dir, tmp)
	}
	joined := strings.Join(cmd.Args, " ")
	if !strings.Contains(joined, "-target") || !strings.Contains(joined, "wasm") {
		t.Errorf("expected tinygo wasm target args, got %q", joined)
	}
	if !strings.Contains(strings.Join(cmd.Env, " "), "GOWORK=off") {
		t.Error("expected GOWORK=off in env")
	}
}

func TestBuildCommandForCompiler_GothicTinyGo_ConfigOverride(t *testing.T) {
	h := linuxAmd64Helper()
	h.ConfigOverride = "/custom/tinygo"
	cmd, err := h.buildCommandForCompiler(WasmCompilerGothicTinyGo, "./gen/", "/out/app.wasm", t.TempDir(), nil)
	if err != nil {
		t.Fatalf("buildCommandForCompiler override: %v", err)
	}
	if cmd.Path != "/custom/tinygo" && !strings.HasSuffix(cmd.Args[0], "tinygo") {
		t.Errorf("expected override tinygo path, got %q (args[0]=%q)", cmd.Path, cmd.Args[0])
	}
}

func TestBuildCommandForCompiler_Golang(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}
	h := linuxAmd64Helper()
	cmd, err := h.buildCommandForCompiler(WasmCompilerGolang, "./gen/", "/out/app.wasm", t.TempDir(), nil)
	if err != nil {
		t.Fatalf("buildCommandForCompiler(Golang): %v", err)
	}
	env := strings.Join(cmd.Env, " ")
	if !strings.Contains(env, "GOOS=js") || !strings.Contains(env, "GOARCH=wasm") {
		t.Errorf("expected js/wasm env, got %q", env)
	}
}

func TestBuildCommandForCompiler_LocalTinyGo_NotFound(t *testing.T) {
	// Isolate PATH so `tinygo` cannot be found → error branch.
	t.Setenv("PATH", t.TempDir())
	if _, err := exec.LookPath("tinygo"); err == nil {
		t.Skip("tinygo still resolvable; cannot isolate")
	}
	h := linuxAmd64Helper()
	if _, err := h.buildCommandForCompiler(WasmCompilerLocalTinyGo, "./gen/", "/out/app.wasm", t.TempDir(), nil); err == nil {
		t.Error("expected error when tinygo not in PATH for LocalTinyGo")
	}
}
