/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	gothic_cli "github.com/felipegenef/gothicframework/pkg/cli"
	"github.com/fsnotify/fsnotify"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

var hotReloadCmd = &cobra.Command{
	Use:   "hot-reload",
	Short: "Run your Gothic app locally in hot-reload mode.",
	Long: `This command uses Templ and Tailwind to enable real-time reloading for local development.

It allows you to develop and debug your Gothic app more efficiently, with changes instantly reflected in the browser as you save your files.`,
	RunE: newHotReloadCommand(gothic_cli.NewCli()),
}

func init() {
	rootCmd.AddCommand(hotReloadCmd)
}

type HotReloadCommand struct {
	cli               *gothic_cli.GothicCli
	mainBinaryName    string
	runCmd            *exec.Cmd
	runCancel         context.CancelFunc
	mutex             sync.Mutex
	excludedDirs      []string
	watchedExtensions []string
	excludeRegex      regexp.Regexp
}

func newHotReloadCommandCli(cli *gothic_cli.GothicCli) HotReloadCommand {
	mainBinary := "tmp/main"
	if cli.Runtime == "windows" {
		mainBinary = "tmp/main.exe"
	}
	return HotReloadCommand{
		cli:               cli,
		mainBinaryName:    mainBinary,
		excludedDirs:      []string{"assets", "tmp", "vendor", "public", "routes"},
		watchedExtensions: []string{".go", ".tpl", ".tmpl", ".templ", ".html"},
		excludeRegex:      *regexp.MustCompile(`.*_templ\.go$`),
	}
}

func newHotReloadCommand(cli gothic_cli.GothicCli) RunEFunc {
	return func(cmd *cobra.Command, args []string) error {
		command := newHotReloadCommandCli(&cli)

		return command.HotReload()
	}
}

func (command *HotReloadCommand) HotReload() error {
	godotenv.Load()
	// Load config to pick up binary overrides if present
	command.cli.GetConfig()
	// Ensure tailwind binary is available before starting watch
	if _, err := command.cli.Tailwind.EnsureBinary(); err != nil {
		return fmt.Errorf("error resolving tailwind binary: %w", err)
	}
	// Ensure TinyGo is installed before any goroutines start — avoids a
	// race between the download and the first rebuild() call.
	if err := command.cli.Wasm.EnsureBinary(); err != nil {
		return fmt.Errorf("error resolving tinygo binary: %w", err)
	}
	port := os.Getenv("HTTP_LISTEN_ADDR")
	if port == "" {
		port = ":8080"
	}
	targetURL, err := url.Parse("http://localhost" + port)
	if err != nil {
		return fmt.Errorf("invalid target URL: %w", err)
	}
	go command.watchTailwindChanges()
	// Wait for tailwind process to render css for the first time
	time.Sleep(4 * time.Second)
	go command.watchForChanges()

	proxyErrCh := make(chan error, 1)
	go func() {
		proxyErrCh <- command.cli.Proxy.RunProxy("localhost", 3000, targetURL)
	}()

	banner := `
 ██████╗  ██████╗ ████████╗██╗  ██╗██╗ ██████╗     █████╗ ██████╗ ██████╗ 
██╔════╝ ██╔═══██╗╚══██╔══╝██║  ██║██║██╔════╝    ██╔══██╗██╔══██╗██╔══██╗
██║  ███╗██║   ██║   ██║   ███████║██║██║         ███████║██████╔╝██████╔╝
██║   ██║██║   ██║   ██║   ██╔══██║██║██║         ██╔══██║██╔═══╝ ██╔═══╝ 
╚██████╔╝╚██████╔╝   ██║   ██║  ██║██║╚██████╗    ██║  ██║██║     ██║     
 ╚═════╝  ╚═════╝    ╚═╝   ╚═╝  ╚═╝╚═╝ ╚═════╝    ╚═╝  ╚═╝╚═╝     ╚═╝     

🚀 Gothic App is up and running!
🌐 Listening on: http://127.0.0.1:3000
🔥  Mode: HOT RELOAD ENABLED
`
	fmt.Println(banner)
	command.openBrowser("http://127.0.0.1:3000")

	if err := <-proxyErrCh; err != nil {
		return fmt.Errorf("proxy server error: %w", err)
	}
	return nil
}

func (command *HotReloadCommand) isExcludedDir(path string) bool {
	for _, d := range command.excludedDirs {
		if strings.Contains(path, string(os.PathSeparator)+d+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}

func (command *HotReloadCommand) watchForChanges() {
	command.rebuild()
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		fmt.Printf("error creating watcher: %v", err)
	}
	defer watcher.Close()
	// Watch the project root directory for changes to main.go and other root-level files
	if err := watcher.Add("."); err != nil {
		fmt.Printf("error watching project root: %v", err)
	}
	err = filepath.Walk("src", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() && command.isExcludedDir(path) {
			return filepath.SkipDir
		}
		if info.IsDir() {
			return watcher.Add(path)
		}
		return nil
	})
	if err != nil {
		fmt.Printf("error walking through directories: %v", err)
		command.rebuild()
	}

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if command.shouldHandle(event.Name, event.Op) {
				command.rebuild()
			}
			// Dynamically watch new directories
			if event.Op&fsnotify.Create == fsnotify.Create {
				info, err := os.Stat(event.Name)
				if err == nil && info.IsDir() && !command.isExcludedDir(event.Name) {
					err := watcher.Add(event.Name)
					if err == nil {
						log.Printf("New directory added to watcher: %s", event.Name)
					} else {
						log.Printf("Failed to add new directory to watcher: %s, error: %v", event.Name, err)
					}
				}
			}
		case err, ok := <-watcher.Errors:
			command.rebuild()
			if !ok {
				fmt.Printf("error reloading app: %v", err)
			}
			log.Println("Watcher error:", err)
		}
	}
}

func (command *HotReloadCommand) shouldHandle(path string, op fsnotify.Op) bool {
	if command.isExcludedDir(path) {
		return false
	}

	filename := filepath.Base(path)
	if command.excludeRegex.MatchString(filename) {
		// Ignore templ-generated files unless they are deleted
		if op&(fsnotify.Remove) == 0 {
			return false
		}
	}

	ext := filepath.Ext(path)
	for _, e := range command.watchedExtensions {
		if e == ext {
			return true
		}
	}
	return false
}

func (command *HotReloadCommand) watchTailwindChanges() {
	log.Println("Starting Tailwind in watch mode...")

	tailWindCmd, err := command.cli.Tailwind.WatchStart()
	if err != nil {
		fmt.Printf("Failed to start Tailwind watch process: %v", err)
		return
	}

	log.Printf("Tailwind is watching with PID %d", tailWindCmd.Process.Pid)

	// Optionally wait for the process to exit and log its exit
	go func() {
		err := tailWindCmd.Wait()
		if err != nil {
			fmt.Printf("Tailwind process exited with error: %v", err)
		} else {
			log.Println("Tailwind process exited normally.")
		}
	}()
}

func (command *HotReloadCommand) rebuild() {
	command.mutex.Lock()
	defer command.mutex.Unlock()

	log.Println("Build routes...")
	config, err := command.cli.GetConfig()
	if err != nil {
		fmt.Printf("error reading config: %v", err)
		return
	}
	if err := command.cli.FileBasedRouter.Render(config.GoModName); err != nil {
		fmt.Printf("error building routes: %v", err)
		return
	}

	log.Println("Build templ...")
	if err := command.cli.Templ.Render(); err != nil {
		fmt.Printf("error building templ: %v", err)
		return
	}

	// WASM must finish before restarting the app — browser reloads immediately after.
	command.buildWasmAll()

	log.Println("Build app...")
	buildCmd := exec.Command("go", "build", "-o", command.mainBinaryName, "main.go")
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr
	if err := buildCmd.Run(); err != nil {
		fmt.Printf("error building app: %v", err)
		return
	}

	if command.runCancel != nil {
		log.Println("Stopping previous go run process...")
		command.runCancel()
		command.runCancel = nil
	}
	log.Println("Running app...")
	ctx, cancel := context.WithCancel(context.Background())
	command.runCancel = cancel

	runCmd := exec.CommandContext(ctx, command.mainBinaryName)
	runCmd.Env = append(os.Environ(), "GOTHIC_MODE=dev")
	runCmd.Stdout = os.Stdout
	runCmd.Stderr = os.Stderr
	command.runCmd = runCmd
	command.cli.Proxy.Sse.Send("message", "reload")
	go func() {
		if err := runCmd.Run(); err != nil {
			if ctx.Err() == nil {
				fmt.Printf("error running app: %v", err)
			}
		}
	}()

}

func (command *HotReloadCommand) buildWasmAll() {
	pages, err := command.cli.Wasm.ScanPages("src/pages", "src/components")
	if err != nil {
		log.Printf("wasm: scan failed: %v", err)
		return
	}
	if len(pages) == 0 {
		return
	}
	log.Printf("wasm: building %d page(s)...", len(pages))
	if err := command.cli.Wasm.GenerateAll(pages, "public/wasm"); err != nil {
		log.Printf("wasm: build failed (continuing with stale binaries): %v", err)
	}
}

func (command *HotReloadCommand) openBrowser(url string) error {
	var cmd *exec.Cmd

	switch command.cli.Runtime {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	default:
		return nil
	}

	return cmd.Start()
}
