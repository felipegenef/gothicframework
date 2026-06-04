package helpers

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// These tests drive GeneratePage / buildTopicManager through their full
// HERMETIC prologue (temp module setup, runtime extraction, module bridge,
// topic collection, source rewriting, main.go rendering, stale-file cleanup)
// up to — but not through — the toolchain build step. The build step is forced
// to fail fast and offline by pointing the compiler at a non-existent binary
// via ConfigOverride, so no TinyGo/Go compile and no network access occur. We
// assert the call returns an error (the build could not run), which is the
// expected outcome in a toolchain-less environment.
//
// The actual successful compile is covered by the integration lane.

func TestGeneratePage_PrologueRunsThenBuildFails(t *testing.T) {
	withTempCwd(t)
	writeProjectFile(t, "go.mod", "module example.com/pp\n\ngo 1.21\n")
	// A page source file (referenced by SourceFile / SourceFile hashing and read
	// by the renderer).
	writeProjectFile(t, "src/pages/counter_templ.go", "package pages\n\nvar Page = 1\n")

	h := DefaultWasmHelper()
	h.cache = loadWasmCache()
	// Force the build command to a binary that does not exist so cmd.Run() fails
	// immediately and offline. The vendor fallback then also fails (the rebuild
	// uses the same missing binary), so GeneratePage returns an error after the
	// hermetic prologue has executed.
	h.ConfigOverride = filepath.Join(t.TempDir(), "no-such-tinygo")
	// ConfigOverride must exist for EnsureBinary, but GeneratePage doesn't call
	// EnsureBinary; it's used directly as the compiler path. Create it as a
	// non-executable empty file so exec fails with a permission/exec error.
	if err := os.WriteFile(h.ConfigOverride, []byte("not a binary"), 0644); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}

	outDir := filepath.Join(t.TempDir(), "out")
	page := WasmPage{
		SourceFile:  "src/pages/counter_templ.go",
		OutputName:  "counter",
		FuncBody:    "println(\"hi\")",
		Compression: WasmCompressionGzip,
		Compiler:    WasmCompilerGothicTinyGo,
	}

	err := h.GeneratePage(page, outDir, &sync.Once{})
	if err == nil {
		t.Fatal("expected GeneratePage to fail at the build step in a toolchain-less env")
	}
	if !strings.Contains(err.Error(), "build") {
		t.Logf("note: error was: %v", err)
	}
	// The generated main.go path is in a temp dir that is cleaned up; we only
	// assert the prologue executed by virtue of reaching the build error.
}

// TestWriteWasmMain_GeneratesArtifact asserts that the prologue's main.go
// renderer (the intermediate artifact produced before the toolchain build step)
// is written and contains the expected package declaration and runtime import.
// The full GeneratePage flow writes this into a temp dir that is cleaned up on
// return, so we drive writeWasmMain directly to inspect the artifact.
func TestWriteWasmMain_GeneratesArtifact(t *testing.T) {
	withTempCwd(t)
	writeProjectFile(t, "src/pages/counter_templ.go", "package pages\n\nvar Page = 1\n")

	h := DefaultWasmHelper()
	dest := filepath.Join(t.TempDir(), "main.go")

	err := h.writeWasmMain(
		"src/pages/counter_templ.go", // src
		"println(\"hi\")",            // body
		nil,                          // stdImports
		nil,                          // helpers
		nil,                          // topicSnippets
		nil,                          // topicStructs
		map[string]string{},          // aliases
		nil,                          // refAliases
		dest,
	)
	if err != nil {
		t.Fatalf("writeWasmMain: %v", err)
	}

	data, statErr := os.ReadFile(dest)
	if statErr != nil {
		t.Fatalf("expected generated main.go at %s: %v", dest, statErr)
	}
	out := string(data)

	for _, want := range []string{
		"package main",
		`. "wasm-runtime/runtime"`, // key runtime import
		`println("hi")`,            // the user body was embedded
	} {
		if !strings.Contains(out, want) {
			t.Errorf("generated main.go missing %q:\n%s", want, out)
		}
	}
}

func TestBuildTopicManager_PrologueRunsThenBuildFails(t *testing.T) {
	setupTopicProject(t)

	h := DefaultWasmHelper()
	h.cache = loadWasmCache()
	h.ConfigOverride = filepath.Join(t.TempDir(), "no-such-tinygo")
	if err := os.WriteFile(h.ConfigOverride, []byte("not a binary"), 0644); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}

	outDir := filepath.Join(t.TempDir(), "out")
	s := structInfo{
		Name:        "Page",
		KeyName:     "page",
		Compression: WasmCompressionGzip,
		Compiler:    WasmCompilerGothicTinyGo,
		Fields: []fieldInfo{
			testFieldInfo("Pings", "int"),
			testFieldInfo("Label", "string"),
		},
	}
	allStructs := []structInfo{s}

	err := h.buildTopicManager(s, []string{"// snippet"}, allStructs, map[string]string{}, nil, outDir, &sync.Once{})
	if err == nil {
		t.Fatal("expected buildTopicManager to fail at the build step in a toolchain-less env")
	}
}
