package cmd

import (
	"testing"

	gothic_cli "github.com/felipegenef/gothicframework/pkg/cli"
	"github.com/fsnotify/fsnotify"
)

func newTestHotReloadCommand() HotReloadCommand {
	cli := gothic_cli.NewCli()
	return newHotReloadCommandCli(&cli)
}

func TestShouldHandle(t *testing.T) {
	command := newTestHotReloadCommand()

	tests := []struct {
		name     string
		path     string
		op       fsnotify.Op
		expected bool
	}{
		{
			name:     "go file write",
			path:     "src/pages/index.go",
			op:       fsnotify.Write,
			expected: true,
		},
		{
			name:     "templ file write",
			path:     "src/pages/index.templ",
			op:       fsnotify.Write,
			expected: true,
		},
		{
			name:     "generated templ file write is ignored",
			path:     "src/pages/index_templ.go",
			op:       fsnotify.Write,
			expected: false,
		},
		{
			name:     "generated templ file delete is handled",
			path:     "src/pages/index_templ.go",
			op:       fsnotify.Remove,
			expected: true,
		},
		{
			name:     "css file is ignored",
			path:     "src/css/app.css",
			op:       fsnotify.Write,
			expected: false,
		},
		{
			name:     "html file is handled",
			path:     "src/pages/index.html",
			op:       fsnotify.Write,
			expected: true,
		},
		{
			name:     "excluded dir assets",
			path:     "src/assets/image.go",
			op:       fsnotify.Write,
			expected: false,
		},
		{
			name:     "excluded dir tmp",
			path:     "src/tmp/main.go",
			op:       fsnotify.Write,
			expected: false,
		},
		{
			name:     "excluded dir public",
			path:     "src/public/styles.go",
			op:       fsnotify.Write,
			expected: false,
		},
		{
			name:     "tpl file",
			path:     "src/pages/layout.tpl",
			op:       fsnotify.Write,
			expected: true,
		},
		{
			name:     "tmpl file",
			path:     "src/pages/layout.tmpl",
			op:       fsnotify.Write,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := command.shouldHandle(tt.path, tt.op)
			if got != tt.expected {
				t.Errorf("shouldHandle(%q, %v) = %v, want %v", tt.path, tt.op, got, tt.expected)
			}
		})
	}
}

func TestIsExcludedDir(t *testing.T) {
	command := newTestHotReloadCommand()

	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{
			name:     "assets dir",
			path:     "src/assets/image.png",
			expected: true,
		},
		{
			name:     "tmp dir",
			path:     "src/tmp/main",
			expected: true,
		},
		{
			name:     "vendor dir",
			path:     "src/vendor/module/file.go",
			expected: true,
		},
		{
			name:     "public dir",
			path:     "src/public/styles.css",
			expected: true,
		},
		{
			name:     "routes dir",
			path:     "src/routes/autoGenRoutes.go",
			expected: true,
		},
		{
			name:     "pages dir not excluded",
			path:     "src/pages/index.go",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := command.isExcludedDir(tt.path)
			if got != tt.expected {
				t.Errorf("isExcludedDir(%q) = %v, want %v", tt.path, got, tt.expected)
			}
		})
	}
}
