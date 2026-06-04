package helpers

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
)

// argvContains reports whether flag appears in argv immediately followed by val.
func argvContains(argv []string, flag, val string) bool {
	for i, a := range argv {
		if a == flag && i+1 < len(argv) && argv[i+1] == val {
			return true
		}
	}
	return false
}

// fakeRunner records the commands it is asked to run and replays a scripted
// sequence of responses. Each Run call consumes the next response; if the
// script is exhausted it returns the zero value (nil out, nil err).
type fakeRunner struct {
	mu        sync.Mutex
	calls     [][]string
	responses []fakeResponse
	idx       int
}

type fakeResponse struct {
	out []byte
	err error
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	call := append([]string{name}, args...)
	f.calls = append(f.calls, call)
	if f.idx < len(f.responses) {
		r := f.responses[f.idx]
		f.idx++
		return r.out, r.err
	}
	return nil, nil
}

func TestAddCloudFrontAssets(t *testing.T) {
	tests := []struct {
		name      string
		responses []fakeResponse
		wantErr   bool
	}{
		{
			name:      "happy path no wasm dir",
			responses: []fakeResponse{{out: []byte("removed")}, {out: []byte("copied")}},
			wantErr:   false,
		},
		{
			name:      "rm fails",
			responses: []fakeResponse{{err: errors.New("rm boom")}},
			wantErr:   true,
		},
		{
			name:      "cp fails",
			responses: []fakeResponse{{out: []byte("removed")}, {err: errors.New("cp boom")}},
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Run from a temp dir so public/wasm does not exist.
			withWorkDir(t, t.TempDir())

			fr := &fakeRunner{responses: tt.responses}
			restore := setRunner(fr)
			defer restore()

			h := NewAwsHelper()
			err := h.AddCloudFrontAssets("my-bucket", "us-east-1", "default")
			if tt.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(fr.calls) > 0 && fr.calls[0][0] != "aws" {
				t.Errorf("expected first command to be aws, got %v", fr.calls[0][0])
			}
			// On the happy path, assert the full argv of the `s3 rm` call that
			// clears the public folder before upload. A regression in any of
			// these args (recursive, region, profile) would leave stale assets
			// or target the wrong bucket/account.
			if tt.name == "happy path no wasm dir" {
				if len(fr.calls) == 0 {
					t.Fatalf("expected at least one aws call, got none")
				}
				wantRm := []string{
					"aws", "s3", "rm", "s3://my-bucket/public",
					"--recursive", "--region", "us-east-1", "--profile", "default",
				}
				if !reflect.DeepEqual(fr.calls[0], wantRm) {
					t.Errorf("s3 rm argv mismatch:\n got %v\nwant %v", fr.calls[0], wantRm)
				}
				// calls[1] is the `s3 cp public ... --recursive` upload. If the
				// source folder were changed away from "public", or --recursive
				// dropped (copying only the top-level file, not subdirectories),
				// the deploy would silently ship a broken asset set.
				if len(fr.calls) < 2 {
					t.Fatalf("expected at least two aws calls, got %d", len(fr.calls))
				}
				wantCp := []string{
					"aws", "s3", "cp", "public", "s3://my-bucket/public",
					"--recursive", "--region", "us-east-1", "--profile", "default",
				}
				if !reflect.DeepEqual(fr.calls[1], wantCp) {
					t.Errorf("s3 cp argv mismatch:\n got %v\nwant %v", fr.calls[1], wantCp)
				}
			}
		})
	}
}

func TestAddCloudFrontAssetsWithWasm(t *testing.T) {
	dir := t.TempDir()
	withWorkDir(t, dir)

	// Create public/wasm with a gzip and a brotli wasm file.
	wasmDir := filepath.Join(dir, "public", "wasm")
	if err := os.MkdirAll(wasmDir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"app.wasm.gz", "app.wasm.br"} {
		if err := os.WriteFile(filepath.Join(wasmDir, f), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	fr := &fakeRunner{} // all responses succeed (zero value)
	restore := setRunner(fr)
	defer restore()

	h := NewAwsHelper()
	if err := h.AddCloudFrontAssets("my-bucket", "us-east-1", "default"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Expect: rm + cp + 2 wasm uploads = 4 calls.
	if len(fr.calls) != 4 {
		t.Fatalf("expected 4 aws calls, got %d: %v", len(fr.calls), fr.calls)
	}

	// calls[2] and calls[3] are the per-encoding wasm re-uploads. The
	// correctness-critical headers are content-encoding (gzip/br), the wasm
	// content-type, and the REPLACE metadata directive — without REPLACE the
	// new headers are silently dropped by S3 and browsers fail to instantiate
	// the module. The order of gz vs br depends on the glob slice in aws.go,
	// so assert per-call by its content-encoding value.
	gzSeen, brSeen := false, false
	for _, call := range fr.calls[2:] {
		if !argvContains(call, "--content-type", "application/wasm") {
			t.Errorf("wasm upload missing --content-type application/wasm: %v", call)
		}
		if !argvContains(call, "--metadata-directive", "REPLACE") {
			t.Errorf("wasm upload missing --metadata-directive REPLACE: %v", call)
		}
		switch {
		case argvContains(call, "--content-encoding", "gzip"):
			gzSeen = true
			if !argvContains(call, "--include", "*.wasm.gz") {
				t.Errorf("gzip upload should include *.wasm.gz: %v", call)
			}
		case argvContains(call, "--content-encoding", "br"):
			brSeen = true
			if !argvContains(call, "--include", "*.wasm.br") {
				t.Errorf("br upload should include *.wasm.br: %v", call)
			}
		default:
			t.Errorf("wasm upload missing a recognized --content-encoding: %v", call)
		}
		// Region/profile must still be threaded through.
		if !argvContains(call, "--region", "us-east-1") || !argvContains(call, "--profile", "default") {
			t.Errorf("wasm upload missing region/profile: %v", call)
		}
	}
	if !gzSeen || !brSeen {
		t.Errorf("expected both gzip and br wasm uploads (gz=%v br=%v)", gzSeen, brSeen)
	}
}

func TestAddCloudFrontAssetsWasmUploadError(t *testing.T) {
	dir := t.TempDir()
	withWorkDir(t, dir)

	wasmDir := filepath.Join(dir, "public", "wasm")
	if err := os.MkdirAll(wasmDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wasmDir, "app.wasm.gz"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	fr := &fakeRunner{responses: []fakeResponse{
		{out: []byte("removed")},          // rm
		{out: []byte("copied")},           // cp
		{err: errors.New("wasm upload")},  // gz upload fails
	}}
	restore := setRunner(fr)
	defer restore()

	h := NewAwsHelper()
	if err := h.AddCloudFrontAssets("my-bucket", "us-east-1", "default"); err == nil {
		t.Fatal("expected error from wasm upload, got nil")
	}
}

func TestHasWasmFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.wasm.gz"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	h := NewAwsHelper()
	if !h.hasWasmFiles(dir, "*.wasm.gz") {
		t.Error("expected to find *.wasm.gz")
	}
	if h.hasWasmFiles(dir, "*.wasm.br") {
		t.Error("did not expect to find *.wasm.br")
	}
	if h.hasWasmFiles(filepath.Join(dir, "missing"), "*") {
		t.Error("expected false for missing dir")
	}
}

func TestRemoveCloudFrontAssets(t *testing.T) {
	tests := []struct {
		name    string
		resp    fakeResponse
		wantErr bool
	}{
		{"happy path", fakeResponse{out: []byte("deleted")}, false},
		{"command error", fakeResponse{err: errors.New("rm boom")}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fr := &fakeRunner{responses: []fakeResponse{tt.resp}}
			restore := setRunner(fr)
			defer restore()

			h := NewAwsHelper()
			err := h.RemoveCloudFrontAssets("my-bucket", "us-east-1", "default")
			if tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestCleanCloudFrontCache(t *testing.T) {
	tests := []struct {
		name      string
		responses []fakeResponse
		wantErr   bool
	}{
		{
			name:      "happy path",
			responses: []fakeResponse{{out: []byte("ABC123\n")}, {out: []byte("invalidated")}},
			wantErr:   false,
		},
		{
			name:      "describe-stacks fails",
			responses: []fakeResponse{{err: errors.New("describe boom")}},
			wantErr:   true,
		},
		{
			name:      "empty distribution id",
			responses: []fakeResponse{{out: []byte("   \n")}},
			wantErr:   true,
		},
		{
			name:      "invalidation fails",
			responses: []fakeResponse{{out: []byte("ABC123\n")}, {err: errors.New("invalidate boom")}},
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fr := &fakeRunner{responses: tt.responses}
			restore := setRunner(fr)
			defer restore()

			h := NewAwsHelper()
			err := h.CleanCloudFrontCache("stack", "prod", "us-east-1", "default")
			if tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			// On the happy path, the second call must be create-invalidation
			// targeting the distribution ID parsed (and trimmed) from the first
			// call's output, with --paths /* to invalidate everything.
			if tt.name == "happy path" {
				if len(fr.calls) != 2 {
					t.Fatalf("expected 2 aws calls, got %d: %v", len(fr.calls), fr.calls)
				}
				inv := fr.calls[1]
				if inv[0] != "aws" || inv[1] != "cloudfront" || inv[2] != "create-invalidation" {
					t.Errorf("expected cloudfront create-invalidation, got %v", inv)
				}
				if !argvContains(inv, "--distribution-id", "ABC123") {
					t.Errorf("expected --distribution-id ABC123 (trimmed from %q), got %v", "ABC123\\n", inv)
				}
				if !argvContains(inv, "--paths", "/*") {
					t.Errorf("expected --paths /* in %v", inv)
				}
			}
		})
	}
}

// withWorkDir changes the process working directory to dir for the duration of
// the test, restoring the original on cleanup.
func withWorkDir(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })
}
