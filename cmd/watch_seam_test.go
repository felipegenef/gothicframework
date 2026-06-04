package cmd

import (
	"os"
	"testing"
	"time"

	gothic_cli "github.com/felipegenef/gothicframework/v2/pkg/cli"
)

// TestWatchForChangesRunsLoop drives watchForChanges in a temp dir with no
// gothic-config.json, so both rebuild() calls return early (config error) and
// never shell out. With no src/ directory, filepath.Walk("src") errors and the
// second rebuild() branch is exercised. The watcher is set up on ".", and a
// .go file write feeds one pass through the select loop, which routes the
// event to handleWatchEvent -> scheduleRebuild and arms the debounce timer.
//
// The test asserts that the debounce timer was armed within a bounded poll
// window (rather than asserting nothing after fixed sleeps). This proves the
// watcher actually delivered the event and the loop scheduled a rebuild — if
// the watcher silently dropped every event, the timer would stay nil and the
// test would fail. The function blocks on select afterward, so it runs in a
// goroutine the test does not join (the fsnotify watcher is GC-finalized at
// process exit), mirroring the watch/reaper-goroutine test style in this
// package.
func TestWatchForChangesRunsLoop(t *testing.T) {
	chdirTemp(t)
	// No gothic-config.json on purpose: rebuild() logs and returns before any
	// `go build` shell-out. No src/ dir: the Walk error branch runs.

	cli := gothic_cli.NewCli()
	cmd := newHotReloadCommandCli(&cli)

	done := make(chan struct{})
	go func() {
		cmd.watchForChanges()
		close(done)
	}()

	// Give the watcher time to register on "." then write a .go file to emit a
	// relevant event so the select loop schedules a rebuild.
	time.Sleep(50 * time.Millisecond)
	if err := os.WriteFile("trigger.go", []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write trigger file: %v", err)
	}

	// Poll for the debounce timer to be armed — proof the event reached the
	// loop and handleWatchEvent -> scheduleRebuild ran. Bounded so a dropped
	// event fails the test instead of hanging.
	armed := false
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		cmd.debounceMu.Lock()
		armed = cmd.debounceTimer != nil
		cmd.debounceMu.Unlock()
		if armed {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Stop the debounce timer so the 150ms rebuild (which would shell out)
	// never fires after the test returns.
	cmd.debounceMu.Lock()
	if cmd.debounceTimer != nil {
		cmd.debounceTimer.Stop()
	}
	cmd.debounceMu.Unlock()

	if !armed {
		t.Error("expected watchForChanges to arm the debounce timer after a .go file write")
	}
}
