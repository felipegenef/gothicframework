package helpers

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestRewriteTopicCalls_Spellings(t *testing.T) {
	h := DefaultWasmHelper()
	structs := []structInfo{{Name: "Page", KeyName: "page"}}

	const wantValue = "Page{Pings: 1}"
	const wantCall = "PageTopic(Page{Pings: 1})"

	cases := []struct {
		name string
		src  string
		// dropped is a key-argument token that must NOT appear in the output for
		// the two-arg spellings (the leading Key/Name ident is removed). Empty for
		// single-arg spellings where there is no key to drop.
		dropped string
	}{
		{"UseTopic key+value", `UseTopic(PageKey, Page{Pings: 1})`, "PageKey"},
		{"UseTopic name+value", `UseTopic(Page, Page{Pings: 1})`, "Page,"},
		{"UseTopic single", `UseTopic(Page{Pings: 1})`, ""},
		{"UseName", `UsePage(Page{Pings: 1})`, ""},
		{"UseNameTopic", `UsePageTopic(Page{Pings: 1})`, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, err := h.rewriteTopicCalls(c.src, structs)
			if err != nil {
				t.Fatalf("rewriteTopicCalls: %v", err)
			}
			// Value argument preserved.
			if !strings.Contains(out, wantValue) {
				t.Errorf("value arg %q missing from output: %q", wantValue, out)
			}
			// Rewritten call has the exact canonical form.
			if !strings.Contains(out, wantCall) {
				t.Errorf("expected canonical call %q in output, got: %q", wantCall, out)
			}
			// Key argument dropped (for two-arg spellings).
			if c.dropped != "" && strings.Contains(out, c.dropped) {
				t.Errorf("expected key arg %q to be dropped, but found it in: %q", c.dropped, out)
			}
			// The legacy call spelling must be fully replaced (no Use* leftover).
			if strings.Contains(out, "UseTopic(") || strings.Contains(out, "UsePage") {
				t.Errorf("legacy Use* spelling still present in output: %q", out)
			}
		})
	}
}

func TestRewriteTopicCalls_NoTopicsPassthrough(t *testing.T) {
	h := DefaultWasmHelper()
	src := `UseTopic(Page{})`
	out, err := h.rewriteTopicCalls(src, nil) // no structs with KeyName
	if err != nil {
		t.Fatalf("rewriteTopicCalls: %v", err)
	}
	if out != src {
		t.Errorf("expected passthrough, got %q", out)
	}
}

func TestRewriteTopicCalls_ParseError(t *testing.T) {
	h := DefaultWasmHelper()
	structs := []structInfo{{Name: "Page", KeyName: "page"}}
	// Unbalanced brace makes the wrapped func body unparseable.
	if _, err := h.rewriteTopicCalls("UseTopic(Page{)", structs); err == nil {
		t.Error("expected parse error for malformed source")
	}
}

func TestRewriteAutoKeys_ParseError(t *testing.T) {
	h := DefaultWasmHelper()
	// Neither top-level nor func-body parse succeeds.
	if _, err := h.rewriteAutoKeys("@@@ not go ###"); err == nil {
		t.Error("expected parse error from rewriteAutoKeys")
	}
}

func TestRewriteAutoKeys_NoMatchPassthrough(t *testing.T) {
	h := DefaultWasmHelper()
	src := `var x = 1`
	out, err := h.rewriteAutoKeys(src)
	if err != nil {
		t.Fatalf("rewriteAutoKeys: %v", err)
	}
	if out != src {
		t.Errorf("expected passthrough, got %q", out)
	}
}

// ---------------------------------------------------------------------------
// wasm_build.go — CopyGoWasmExec (uses the real `go` toolchain, available in
// the default test environment) and GenerateAll's empty-pages early return.
// ---------------------------------------------------------------------------

func TestCopyGoWasmExec(t *testing.T) {
	dir := t.TempDir()
	h := DefaultWasmHelper()
	err := h.CopyGoWasmExec(dir)
	if err != nil {
		// Some minimal CI images ship Go without the wasm_exec.js asset; treat
		// that as a skip rather than a hard failure since it is environmental.
		if strings.Contains(err.Error(), "could not locate wasm_exec.js") {
			t.Skipf("wasm_exec.js not present in this GOROOT: %v", err)
		}
		t.Fatalf("CopyGoWasmExec: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "wasm_exec_go.js")); statErr != nil {
		t.Errorf("expected wasm_exec_go.js: %v", statErr)
	}
}

func TestGenerateAll_EmptyPagesEarlyReturn(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("GOTHIC_CLI_CACHE_DIR", tmp)
	withTempCwd(t) // no src/topics, fresh project root

	h := linuxAmd64Helper()
	// Pre-stage tinygo + binaryen so EnsureBinary returns ready without a
	// download, then GenerateAll returns early because there are no pages.
	touch(t, h.TinyGoBinary())
	touch(t, h.BinaryenBinary())

	if err := h.GenerateAll(nil, filepath.Join(tmp, "out")); err != nil {
		t.Fatalf("GenerateAll empty: %v", err)
	}
}

func TestGenerateTopicManagers_MkdirEmptyTopics(t *testing.T) {
	withTempCwd(t)
	if err := os.MkdirAll("src/topics", 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// A topics dir with no topic structs → hasTopicStructs false → no-op.
	writeProjectFile(t, "src/topics/empty.go", "package topics\n\nvar x = 1\n")
	h := DefaultWasmHelper()
	if err := h.GenerateTopicManagers(t.TempDir(), &sync.Once{}); err != nil {
		t.Fatalf("GenerateTopicManagers: %v", err)
	}
}
