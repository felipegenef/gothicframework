package helpers

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// wasm_binary.go — cacheDir fallback to os.UserCacheDir
// ---------------------------------------------------------------------------

func TestCacheDir_FallbackToUserCacheDir(t *testing.T) {
	// Unset the override so cacheDir falls back to os.UserCacheDir().
	t.Setenv("GOTHIC_CLI_CACHE_DIR", "")
	h := linuxAmd64Helper()
	dir, err := h.cacheDir()
	if err != nil {
		t.Fatalf("cacheDir fallback: %v", err)
	}
	if !strings.HasSuffix(dir, filepath.Join("gothic-cli", "tinygo")) {
		t.Errorf("cacheDir: got %q", dir)
	}
}

// ---------------------------------------------------------------------------
// wasm_archive.go — symlink and hardlink tar entries
// ---------------------------------------------------------------------------

func makeTarGzWithLinks(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "links.tar.gz")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)

	// regular file
	body := "real content"
	tw.WriteHeader(&tar.Header{Name: "bin/real", Mode: 0644, Size: int64(len(body)), Typeflag: tar.TypeReg})
	tw.Write([]byte(body))
	// relative symlink
	tw.WriteHeader(&tar.Header{Name: "bin/link", Typeflag: tar.TypeSymlink, Linkname: "real"})
	// hard link to the regular file
	tw.WriteHeader(&tar.Header{Name: "bin/hard", Typeflag: tar.TypeLink, Linkname: "bin/real"})

	tw.Close()
	gz.Close()
	return path
}

func TestExtractTarGz_SymlinkAndHardlink(t *testing.T) {
	src := makeTarGzWithLinks(t)
	dest := t.TempDir()
	h := linuxAmd64Helper()
	if err := h.extractArchive(src, dest); err != nil {
		t.Fatalf("extractArchive with links: %v", err)
	}
	// symlink should resolve to the real file content.
	got, err := os.ReadFile(filepath.Join(dest, "bin", "link"))
	if err != nil {
		t.Fatalf("read symlink target: %v", err)
	}
	if string(got) != "real content" {
		t.Errorf("symlink content: got %q", got)
	}
	// hard link should also contain the content.
	got2, err := os.ReadFile(filepath.Join(dest, "bin", "hard"))
	if err != nil {
		t.Fatalf("read hardlink: %v", err)
	}
	if string(got2) != "real content" {
		t.Errorf("hardlink content: got %q", got2)
	}
}

func makeTarGzAbsSymlink(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "abs.tar.gz")
	f, _ := os.Create(path)
	defer f.Close()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	tw.WriteHeader(&tar.Header{Name: "bad", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"})
	tw.Close()
	gz.Close()
	return path
}

func TestExtractTarGz_AbsoluteSymlinkRejected(t *testing.T) {
	src := makeTarGzAbsSymlink(t)
	h := linuxAmd64Helper()
	if err := h.extractArchive(src, t.TempDir()); err == nil {
		t.Fatal("expected absolute symlink to be rejected")
	}
}

func TestWriteFileFromReader_BadDest(t *testing.T) {
	h := linuxAmd64Helper()
	// Destination directory does not exist → OpenFile error.
	bad := filepath.Join(t.TempDir(), "nope", "out.txt")
	if err := h.writeFileFromReader(bad, strings.NewReader("x"), 0644); err == nil {
		t.Error("expected error writing to nonexistent dir")
	}
}

func TestExtractArchive_ShortFile(t *testing.T) {
	// A file shorter than the 4-byte magic read → read error.
	p := filepath.Join(t.TempDir(), "tiny")
	if err := os.WriteFile(p, []byte("Z"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	h := linuxAmd64Helper()
	if err := h.extractArchive(p, t.TempDir()); err == nil {
		t.Error("expected error reading magic bytes from short file")
	}
}

func TestExtractArchive_CorruptGzip(t *testing.T) {
	// Starts with the gzip magic bytes but is not a valid gzip stream.
	p := filepath.Join(t.TempDir(), "bad.gz")
	if err := os.WriteFile(p, []byte{0x1f, 0x8b, 0x00, 0x00, 0x00}, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	h := linuxAmd64Helper()
	if err := h.extractArchive(p, t.TempDir()); err == nil {
		t.Error("expected error for corrupt gzip stream")
	}
}

func TestExtractZip_Corrupt(t *testing.T) {
	// Starts with the zip magic "PK" but is not a valid zip archive.
	p := filepath.Join(t.TempDir(), "bad.zip")
	if err := os.WriteFile(p, []byte("PK\x03\x04 garbage"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	h := linuxAmd64Helper()
	if err := h.extractArchive(p, t.TempDir()); err == nil {
		t.Error("expected error for corrupt zip")
	}
}

// ---------------------------------------------------------------------------
// module_bridge.go — WriteBridgeGoMod default go version + go.sum copy
// ---------------------------------------------------------------------------

func TestWriteBridgeGoMod_DefaultsAndGoSum(t *testing.T) {
	userRoot := t.TempDir()
	// user go.mod without an explicit go version → bridge uses default "1.21".
	if err := os.WriteFile(filepath.Join(userRoot, "go.mod"), []byte("module example.com/u\n"), 0644); err != nil {
		t.Fatalf("write user go.mod: %v", err)
	}
	// user go.sum present → it should be copied into the temp dir.
	if err := os.WriteFile(filepath.Join(userRoot, "go.sum"), []byte("example.com/x v1 h1:abc\n"), 0644); err != nil {
		t.Fatalf("write user go.sum: %v", err)
	}

	tempDir := t.TempDir()
	if err := WriteBridgeGoMod(tempDir, "example.com/u", userRoot, ""); err != nil {
		t.Fatalf("WriteBridgeGoMod: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(tempDir, "go.mod"))
	if err != nil {
		t.Fatalf("read bridge go.mod: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, "go 1.21") {
		t.Errorf("expected default go 1.21, got:\n%s", s)
	}
	if !strings.Contains(s, "replace example.com/u") {
		t.Errorf("expected replace directive, got:\n%s", s)
	}
	if _, err := os.Stat(filepath.Join(tempDir, "go.sum")); err != nil {
		t.Errorf("expected go.sum to be copied: %v", err)
	}
}

func TestCopyFile_Errors(t *testing.T) {
	// Missing source → open error.
	if err := copyFile(filepath.Join(t.TempDir(), "nope"), filepath.Join(t.TempDir(), "dst")); err == nil {
		t.Error("expected error for missing source")
	}
	// Valid source but destination directory does not exist → open error.
	src := filepath.Join(t.TempDir(), "src")
	if err := os.WriteFile(src, []byte("x"), 0644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	if err := copyFile(src, filepath.Join(t.TempDir(), "nodir", "dst")); err == nil {
		t.Error("expected error for missing destination dir")
	}
}

func TestCopyFile_Success(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	if err := os.WriteFile(src, []byte("payload"), 0644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile: %v", err)
	}
	got, _ := os.ReadFile(dst)
	if string(got) != "payload" {
		t.Errorf("copyFile content: got %q", got)
	}
}

func TestWriteBridgeGoMod_GoSumCopyError(t *testing.T) {
	userRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(userRoot, "go.mod"), []byte("module example.com/u\ngo 1.21\n"), 0644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	// Make go.sum a directory so copyFile's source open (os.Open on a dir is OK,
	// but io.Copy from a dir fails) surfaces a copy error path.
	if err := os.Mkdir(filepath.Join(userRoot, "go.sum"), 0755); err != nil {
		t.Fatalf("mkdir go.sum: %v", err)
	}
	if err := WriteBridgeGoMod(t.TempDir(), "example.com/u", userRoot, "1.21"); err == nil {
		t.Error("expected error copying directory-as-go.sum")
	}
}

func TestReadUserModulePath_Errors(t *testing.T) {
	// Missing go.mod.
	if _, _, err := ReadUserModulePath(t.TempDir()); err == nil {
		t.Error("expected error for missing go.mod")
	}
	// go.mod without a module directive.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("go 1.21\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, _, err := ReadUserModulePath(dir); err == nil {
		t.Error("expected error for go.mod without module directive")
	}
}

func TestReadUserModulePath_Success(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/ok\n\ngo 1.22\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	mod, ver, err := ReadUserModulePath(dir)
	if err != nil {
		t.Fatalf("ReadUserModulePath: %v", err)
	}
	if mod != "example.com/ok" || ver != "1.22" {
		t.Errorf("got mod=%q ver=%q", mod, ver)
	}
}

// ---------------------------------------------------------------------------
// wasm_topic.go — isCreateTopicCall IndexListExpr + topicStructNameFromArg edge
// ---------------------------------------------------------------------------

func TestIsCreateTopicCall_IndexListExpr(t *testing.T) {
	// CreateTopic[K, V](...) parses Fun as *ast.IndexListExpr.
	ce := mustCallExpr(t, "CreateTopic[Page, int](Page{}, C{})")
	if !isCreateTopicCall(ce.Fun) {
		t.Error("expected IndexListExpr CreateTopic to match")
	}
	// A non-CreateTopic generic call must not match.
	ce2 := mustCallExpr(t, "Other[K, V](x)")
	if isCreateTopicCall(ce2.Fun) {
		t.Error("non-CreateTopic generic should not match")
	}
}

func TestTopicStructNameFromArg_NilCompositeType(t *testing.T) {
	// A composite literal with no explicit type (e.g. inside a typed context)
	// yields "".
	ce := mustCallExpr(t, "CreateTopic([]int{1}, C{})")
	if got := topicStructNameFromArg(ce.Args[0]); got != "" {
		t.Errorf("expected empty name for non-struct composite, got %q", got)
	}
}
