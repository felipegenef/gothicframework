package cmd

import (
	"errors"
	"net/url"
	"os"
	"sync/atomic"
	"testing"
	"time"

	gothic_cli "github.com/felipegenef/gothicframework/v2/pkg/cli"
	"github.com/fsnotify/fsnotify"
)

// TestHotReloadFullPathWithSeams drives HotReload all the way through the banner
// and browser-open by injecting the sleeper, browser, and proxy seams so no real
// 4s wait, OS browser launch, or port bind happens. The proxy seam returns nil,
// so HotReload returns nil.
func TestHotReloadFullPathWithSeams(t *testing.T) {
	bin := writeFakeTailwind(t, true)
	chdirTemp(t)
	writeGoMod(t, "demo")
	scaffoldSrc(t)
	// Use a fake tailwind so EnsureBinary succeeds. Wasm.EnsureBinary still needs
	// TinyGo, so point wasmBinary at the fake tailwind script (it only needs to be
	// an existing executable for the override path).
	writeConfig(t, `{"projectName":"demo","goModuleName":"demo","tailwindBinary":"`+bin+`","wasmBinary":"`+bin+`"}`)

	cli := gothic_cli.NewCli()
	cmd := newHotReloadCommandCli(&cli)

	var browserURL atomic.Value
	cmd.openBrowserFn = func(u string) error { browserURL.Store(u); return nil }
	cmd.sleeper = func(time.Duration) {} // no real sleep
	cmd.proxyRunner = func(target *url.URL) error {
		// Confirm the target URL was parsed from the default port.
		if target.Host != "localhost:8080" {
			t.Errorf("proxy target host = %q, want localhost:8080", target.Host)
		}
		return nil
	}

	if err := cmd.HotReload(); err != nil {
		t.Fatalf("HotReload returned error: %v", err)
	}
	if got := browserURL.Load(); got != "http://127.0.0.1:3000" {
		t.Errorf("openBrowser url = %v, want http://127.0.0.1:3000", got)
	}
}

// TestHotReloadProxyError ensures a proxy failure is wrapped and returned.
func TestHotReloadProxyError(t *testing.T) {
	bin := writeFakeTailwind(t, true)
	chdirTemp(t)
	writeGoMod(t, "demo")
	scaffoldSrc(t)
	writeConfig(t, `{"projectName":"demo","goModuleName":"demo","tailwindBinary":"`+bin+`","wasmBinary":"`+bin+`"}`)

	cli := gothic_cli.NewCli()
	cmd := newHotReloadCommandCli(&cli)
	cmd.openBrowserFn = func(string) error { return nil }
	cmd.sleeper = func(time.Duration) {}
	cmd.proxyRunner = func(*url.URL) error { return errors.New("boom") }

	err := cmd.HotReload()
	if err == nil {
		t.Fatal("expected HotReload to return the proxy error")
	}
}

// TestHotReloadHonorsHTTPListenAddr exercises the non-default port branch.
func TestHotReloadHonorsHTTPListenAddr(t *testing.T) {
	bin := writeFakeTailwind(t, true)
	chdirTemp(t)
	writeGoMod(t, "demo")
	scaffoldSrc(t)
	writeConfig(t, `{"projectName":"demo","goModuleName":"demo","tailwindBinary":"`+bin+`","wasmBinary":"`+bin+`"}`)

	t.Setenv("HTTP_LISTEN_ADDR", ":9999")

	cli := gothic_cli.NewCli()
	cmd := newHotReloadCommandCli(&cli)
	cmd.openBrowserFn = func(string) error { return nil }
	cmd.sleeper = func(time.Duration) {}
	var gotHost string
	cmd.proxyRunner = func(target *url.URL) error { gotHost = target.Host; return nil }

	if err := cmd.HotReload(); err != nil {
		t.Fatalf("HotReload error: %v", err)
	}
	if gotHost != "localhost:9999" {
		t.Errorf("proxy target host = %q, want localhost:9999", gotHost)
	}
}

// TestHandleWatchEventSchedulesRebuild verifies a relevant file change arms the
// debounce timer via handleWatchEvent, without a running watcher.
func TestHandleWatchEventSchedulesRebuild(t *testing.T) {
	cli := gothic_cli.NewCli()
	cmd := newHotReloadCommandCli(&cli)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatalf("new watcher: %v", err)
	}
	defer watcher.Close()

	cmd.handleWatchEvent(watcher, fsnotify.Event{Name: "src/pages/index.go", Op: fsnotify.Write})

	cmd.debounceMu.Lock()
	armed := cmd.debounceTimer != nil
	if armed {
		// Stop so the 150ms rebuild (which shells out) never fires.
		cmd.debounceTimer.Stop()
	}
	cmd.debounceMu.Unlock()
	if !armed {
		t.Error("expected handleWatchEvent to arm the debounce timer for a relevant change")
	}
}

// TestHandleWatchEventIgnoresIrrelevant confirms an ignored path does not arm a
// rebuild timer.
func TestHandleWatchEventIgnoresIrrelevant(t *testing.T) {
	cli := gothic_cli.NewCli()
	cmd := newHotReloadCommandCli(&cli)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatalf("new watcher: %v", err)
	}
	defer watcher.Close()

	cmd.handleWatchEvent(watcher, fsnotify.Event{Name: "src/css/app.css", Op: fsnotify.Write})

	cmd.debounceMu.Lock()
	armed := cmd.debounceTimer != nil
	cmd.debounceMu.Unlock()
	if armed {
		t.Error("expected handleWatchEvent to ignore a css change")
	}
}

// TestHandleWatchEventAddsNewDirectory verifies a Create event on a new,
// non-excluded directory is added to the watcher.
func TestHandleWatchEventAddsNewDirectory(t *testing.T) {
	dir := chdirTemp(t)
	cli := gothic_cli.NewCli()
	cmd := newHotReloadCommandCli(&cli)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatalf("new watcher: %v", err)
	}
	defer watcher.Close()

	newDir := "src/newpkg"
	if err := os.MkdirAll(newDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	cmd.handleWatchEvent(watcher, fsnotify.Event{Name: newDir, Op: fsnotify.Create})

	found := false
	for _, w := range watcher.WatchList() {
		if w == newDir || w == dir+"/"+newDir {
			found = true
		}
	}
	if !found {
		t.Errorf("expected new directory %q added to watcher, list=%v", newDir, watcher.WatchList())
	}
}

// TestHandleWatchEventExcludedDirNotAdded confirms an excluded directory Create
// event is not added to the watcher.
func TestHandleWatchEventExcludedDirNotAdded(t *testing.T) {
	chdirTemp(t)
	cli := gothic_cli.NewCli()
	cmd := newHotReloadCommandCli(&cli)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatalf("new watcher: %v", err)
	}
	defer watcher.Close()

	excluded := "src/public/sub"
	if err := os.MkdirAll(excluded, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	cmd.handleWatchEvent(watcher, fsnotify.Event{Name: excluded, Op: fsnotify.Create})

	for _, w := range watcher.WatchList() {
		if w == excluded {
			t.Errorf("excluded dir %q should not be watched", excluded)
		}
	}
}
