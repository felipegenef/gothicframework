package helpers

import (
	"errors"
	"testing"
)

func TestSamBuild(t *testing.T) {
	tests := []struct {
		name    string
		resp    fakeResponse
		wantErr bool
	}{
		{"happy path", fakeResponse{out: []byte("built")}, false},
		{"command error", fakeResponse{err: errors.New("build boom")}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fr := &fakeRunner{responses: []fakeResponse{tt.resp}}
			restore := setRunner(fr)
			defer restore()

			h := NewAwsSamHelper()
			err := h.Build()
			if tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(fr.calls) != 1 || fr.calls[0][0] != "sam" || fr.calls[0][1] != "build" {
				t.Errorf("expected sam build call, got %v", fr.calls)
			}
		})
	}
}

func TestSamDeploy(t *testing.T) {
	tests := []struct {
		name    string
		resp    fakeResponse
		wantErr bool
	}{
		{"happy path", fakeResponse{out: []byte("deployed")}, false},
		{"command error", fakeResponse{err: errors.New("deploy boom")}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fr := &fakeRunner{responses: []fakeResponse{tt.resp}}
			restore := setRunner(fr)
			defer restore()

			h := NewAwsSamHelper()
			err := h.Deploy("prod", "mystack", "default")
			if tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			// Deploy must always issue exactly one shell-out; a regression that
			// makes zero calls must fail here rather than silently skip the
			// argv assertion below.
			if len(fr.calls) != 1 {
				t.Fatalf("expected exactly 1 runner call, got %d: %v", len(fr.calls), fr.calls)
			}
			// Verify the full deploy argv: command, stack-name composition,
			// the Stage parameter override, and the profile.
			args := fr.calls[0]
			if args[0] != "sam" || args[1] != "deploy" {
				t.Errorf("expected `sam deploy`, got %v", args)
			}
			if !argvContains(args, "--stack-name", "mystack-prod") {
				t.Errorf("expected --stack-name mystack-prod in %v", args)
			}
			if !argvContains(args, "--parameter-overrides", "Stage=prod") {
				t.Errorf("expected --parameter-overrides Stage=prod in %v", args)
			}
			if !argvContains(args, "--profile", "default") {
				t.Errorf("expected --profile default in %v", args)
			}
		})
	}
}

func TestSamDeleteStack(t *testing.T) {
	tests := []struct {
		name    string
		resp    fakeResponse
		wantErr bool
	}{
		{"happy path", fakeResponse{out: []byte("deleted")}, false},
		{"command error", fakeResponse{err: errors.New("delete boom")}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fr := &fakeRunner{responses: []fakeResponse{tt.resp}}
			restore := setRunner(fr)
			defer restore()

			h := NewAwsSamHelper()
			err := h.DeleteStack("prod", "mystack", "default")
			if tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			// DeleteStack must issue exactly one shell-out with the correct
			// command, stack-name composition, and profile. Targeting the wrong
			// stack name here would delete the wrong environment's resources.
			if len(fr.calls) != 1 {
				t.Fatalf("expected exactly 1 runner call, got %d: %v", len(fr.calls), fr.calls)
			}
			args := fr.calls[0]
			if args[0] != "sam" || args[1] != "delete" {
				t.Errorf("expected `sam delete`, got %v", args)
			}
			if !argvContains(args, "--stack-name", "mystack-prod") {
				t.Errorf("expected --stack-name mystack-prod in %v", args)
			}
			if !argvContains(args, "--profile", "default") {
				t.Errorf("expected --profile default in %v", args)
			}
		})
	}
}
