package cmd

import (
	"os"
	"testing"
	"time"

	gothic_cli "github.com/felipegenef/gothicframework/v2/pkg/cli"
	"github.com/spf13/cobra"
)

func TestNewHotReloadCommandCliWindowsBinary(t *testing.T) {
	cli := gothic_cli.NewCli()
	cli.Runtime = "windows"
	cmd := newHotReloadCommandCli(&cli)
	if cmd.mainBinaryName != "tmp/main.exe" {
		t.Errorf("windows binary = %q, want tmp/main.exe", cmd.mainBinaryName)
	}
}

func TestNewHotReloadCommandCliUnixBinary(t *testing.T) {
	cli := gothic_cli.NewCli()
	cli.Runtime = "linux"
	cmd := newHotReloadCommandCli(&cli)
	if cmd.mainBinaryName != "tmp/main" {
		t.Errorf("unix binary = %q, want tmp/main", cmd.mainBinaryName)
	}
}

func TestHotReloadFailsResolvingTailwindBinary(t *testing.T) {
	chdirTemp(t)
	// Bad tailwind override makes EnsureBinary fail, so HotReload returns before
	// starting any goroutine, watcher, proxy, or browser.
	writeConfig(t, `{"projectName":"demo","goModuleName":"demo","tailwindBinary":"/nonexistent/tw"}`)

	cli := gothic_cli.NewCli()
	cmd := newHotReloadCommandCli(&cli)
	if err := cmd.HotReload(); err == nil {
		t.Fatal("expected HotReload to fail resolving tailwind binary")
	}
}

func TestNewHotReloadCommandRunEFailsEarly(t *testing.T) {
	chdirTemp(t)
	writeConfig(t, `{"projectName":"demo","goModuleName":"demo","tailwindBinary":"/nonexistent/tw"}`)

	runE := newHotReloadCommand(gothic_cli.NewCli())
	if err := runE(&cobra.Command{}, nil); err == nil {
		t.Fatal("expected hot-reload RunE to fail early")
	}
}

func TestOpenBrowserUnsupportedRuntime(t *testing.T) {
	cli := gothic_cli.NewCli()
	cli.Runtime = "plan9"
	cmd := newHotReloadCommandCli(&cli)
	if err := cmd.defaultOpenBrowser("http://127.0.0.1:3000"); err != nil {
		t.Errorf("defaultOpenBrowser on unsupported runtime should be a no-op, got %v", err)
	}
}

func TestScheduleRebuildResetsTimer(t *testing.T) {
	cli := gothic_cli.NewCli()
	cmd := newHotReloadCommandCli(&cli)

	// First call arms the debounce timer; second call must reset (not leak) it.
	// Immediately stop it so the 150ms rebuild() never fires (rebuild shells out).
	cmd.scheduleRebuild()
	if cmd.debounceTimer == nil {
		t.Fatal("expected debounceTimer to be armed")
	}
	cmd.scheduleRebuild()
	cmd.debounceMu.Lock()
	stopped := cmd.debounceTimer.Stop()
	cmd.debounceMu.Unlock()
	if !stopped {
		t.Error("expected to stop the pending debounce timer before it fired")
	}
}

func TestRebuildReturnsEarlyWithoutConfig(t *testing.T) {
	chdirTemp(t)
	// No gothic-config.json: rebuild logs the config error and returns before
	// rendering routes/templ or shelling out to `go build`.
	cli := gothic_cli.NewCli()
	cmd := newHotReloadCommandCli(&cli)
	cmd.rebuild()

	// rebuild() must bail out at the config-read step: no binary is built.
	if _, err := os.Stat(cmd.mainBinaryName); !os.IsNotExist(err) {
		t.Errorf("expected no binary when config is missing, stat err = %v", err)
	}
}

// Crash guard only — proves watchTailwindChanges does not panic when the
// tailwind binary cannot be resolved (it logs and returns). There is no
// observable seam to assert the early return, so behavior beyond "no panic"
// is not covered here.
func TestWatchTailwindChangesStartFailure(t *testing.T) {
	chdirTemp(t)
	// Bad tailwind override -> WatchStart fails resolving the binary; the
	// function logs and returns without spawning a watcher goroutine.
	writeConfig(t, `{"projectName":"demo","goModuleName":"demo","tailwindBinary":"/nonexistent/tw"}`)
	cli := gothic_cli.NewCli()
	if _, err := cli.GetConfig(); err != nil {
		t.Fatalf("config: %v", err)
	}
	cmd := newHotReloadCommandCli(&cli)
	cmd.watchTailwindChanges()
}

// Crash/deadlock guard only — proves watchTailwindChanges starts the watch
// process and its reaper goroutine without panicking when the binary launches
// successfully. The spawned process and reaper have no observable seam, so
// only the no-panic/no-deadlock property is exercised here.
func TestWatchTailwindChangesStartsFakeBinary(t *testing.T) {
	bin := writeFakeTailwind(t, true)
	chdirTemp(t)
	// Fake tailwind exits 0 immediately, so WatchStart's cmd.Start() succeeds,
	// the PID is logged, and the wait-goroutine reaps the short-lived process.
	writeConfig(t, `{"projectName":"demo","goModuleName":"demo","tailwindBinary":"`+bin+`"}`)
	cli := gothic_cli.NewCli()
	if _, err := cli.GetConfig(); err != nil {
		t.Fatalf("config: %v", err)
	}
	cmd := newHotReloadCommandCli(&cli)
	cmd.watchTailwindChanges()
	// Give the reaper goroutine a moment to observe the quick exit. Not
	// strictly required for coverage, but keeps the goroutine from outliving
	// the test in a confusing way.
	time.Sleep(50 * time.Millisecond)
}

func TestRebuildProceedsUntilGoBuild(t *testing.T) {
	chdirTemp(t)
	writeGoMod(t, "demo")
	scaffoldSrc(t)
	writeConfig(t, `{"projectName":"demo","goModuleName":"demo"}`)

	// Config + Router.Render + Templ.Render + buildWasmAll (no pages) all run;
	// then `go build main.go` fails (no main.go) and rebuild logs and returns
	// without starting the app process. Exercises the bulk of rebuild() safely.
	cli := gothic_cli.NewCli()
	cmd := newHotReloadCommandCli(&cli)
	cmd.rebuild()

	if _, err := os.Stat(cmd.mainBinaryName); err == nil {
		t.Error("no binary should be produced when go build fails")
	}
}

func TestBuildWasmAllScanFailsOutsideModule(t *testing.T) {
	chdirTemp(t)
	writeConfig(t, `{"projectName":"demo","goModuleName":"demo"}`)
	// ScanPages fails outside a Go module; buildWasmAll must log and return
	// without panicking (no GenerateAll / tinygo invocation).
	cli := gothic_cli.NewCli()
	cmd := newHotReloadCommandCli(&cli)
	cmd.buildWasmAll()

	// ScanPages fails before GenerateAll, so no wasm output is written.
	if _, err := os.Stat("public/wasm"); !os.IsNotExist(err) {
		t.Errorf("expected no public/wasm output on scan failure, stat err = %v", err)
	}
}

func TestBuildWasmAllNoPages(t *testing.T) {
	chdirTemp(t)
	writeGoMod(t, "demo")
	scaffoldSrc(t)
	writeConfig(t, `{"projectName":"demo","goModuleName":"demo"}`)
	// Valid empty module: ScanPages returns zero pages; buildWasmAll returns
	// early (len(pages)==0) without invoking TinyGo.
	cli := gothic_cli.NewCli()
	cmd := newHotReloadCommandCli(&cli)
	cmd.buildWasmAll()

	// Zero pages -> buildWasmAll returns before GenerateAll, so no output dir.
	if _, err := os.Stat("public/wasm"); !os.IsNotExist(err) {
		t.Errorf("expected GenerateAll not called (no public/wasm), stat err = %v", err)
	}
}
