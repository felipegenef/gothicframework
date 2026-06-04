package helpers

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestNewLogger(t *testing.T) {
	tests := []struct {
		name      string
		logLevel  string
		verbose   bool
		logFn     func(l *slog.Logger)
		wantSub   string
		wantEmpty bool // output should be empty (filtered out by level)
	}{
		{
			name:     "info level logs info",
			logLevel: "info",
			logFn:    func(l *slog.Logger) { l.Info("hello-info") },
			wantSub:  "hello-info",
		},
		{
			name:      "info level filters debug",
			logLevel:  "info",
			logFn:     func(l *slog.Logger) { l.Debug("hidden-debug") },
			wantEmpty: true,
		},
		{
			name:     "debug level logs debug",
			logLevel: "debug",
			logFn:    func(l *slog.Logger) { l.Debug("show-debug") },
			wantSub:  "show-debug",
		},
		{
			name:     "verbose forces debug",
			logLevel: "info",
			verbose:  true,
			logFn:    func(l *slog.Logger) { l.Debug("verbose-debug") },
			wantSub:  "verbose-debug",
		},
		{
			name:      "warn level filters info",
			logLevel:  "warn",
			logFn:     func(l *slog.Logger) { l.Info("hidden-info") },
			wantEmpty: true,
		},
		{
			name:     "warn level logs warn",
			logLevel: "warn",
			logFn:    func(l *slog.Logger) { l.Warn("show-warn") },
			wantSub:  "show-warn",
		},
		{
			name:     "error level logs error",
			logLevel: "error",
			logFn:    func(l *slog.Logger) { l.Error("show-error") },
			wantSub:  "show-error",
		},
		{
			name:      "error level filters warn",
			logLevel:  "error",
			logFn:     func(l *slog.Logger) { l.Warn("hidden-warn") },
			wantEmpty: true,
		},
		{
			name:     "unknown level defaults to info",
			logLevel: "bogus",
			logFn:    func(l *slog.Logger) { l.Info("default-info") },
			wantSub:  "default-info",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			l := NewLogger(tt.logLevel, tt.verbose, &buf)
			if l == nil {
				t.Fatal("NewLogger returned nil")
			}
			tt.logFn(l)
			out := buf.String()
			if tt.wantEmpty {
				if strings.TrimSpace(out) != "" {
					t.Errorf("expected empty output, got %q", out)
				}
				return
			}
			if !strings.Contains(out, tt.wantSub) {
				t.Errorf("expected output to contain %q, got %q", tt.wantSub, out)
			}
		})
	}
}
