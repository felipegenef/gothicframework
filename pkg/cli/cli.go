package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"runtime"

	helpers     "github.com/felipegenef/gothicframework/v2/pkg/helpers"
	proxy       "github.com/felipegenef/gothicframework/v2/pkg/helpers/proxy"
	routes      "github.com/felipegenef/gothicframework/v2/pkg/helpers/routes"
	wasmhelper  "github.com/felipegenef/gothicframework/v2/pkg/helpers/wasm"
)

// commandRunner is the DI seam that lets tests intercept Go-toolchain
// invocations made by InitializeModule without shelling out for real.
type commandRunner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

type execRunner struct{}

func (r execRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	return cmd.CombinedOutput()
}

// cliRunner is overridable in tests to avoid invoking the real Go toolchain.
var cliRunner commandRunner = execRunner{}

type GothicCli struct {
	config  *Config
	appID   *string
	Runtime string

	Templates       helpers.TemplateHelper
	Tailwind        helpers.TailwindHelper
	Templ           helpers.TemplHelper
	Logger          *slog.Logger
	AwsSam          helpers.AwsSamHelper
	AWS             helpers.AwsHelper
	FileBasedRouter routes.FileBasedRouteHelper
	Proxy           proxy.ProxyHelper
	Wasm            wasmhelper.WasmHelper
}

type CliCommands struct {
	Init            *bool
	Build           *string
	Deploy          *bool
	Help            *bool
	ImgOptimization *bool
	HotReload       *bool
	DeployAction    *string
	DeployStage     *string
}

func NewCli() GothicCli {
	cli := GothicCli{
		Runtime:         runtime.GOOS,
		Templates:       helpers.NewTemplateHelper(),
		Tailwind:        helpers.NewTailwindHelper(runtime.GOOS, runtime.GOARCH),
		Templ:           helpers.NewTemplHelper(),
		AwsSam:          helpers.NewAwsSamHelper(),
		AWS:             helpers.NewAwsHelper(),
		Logger:          helpers.NewLogger("error", false, os.Stdout),
		FileBasedRouter: routes.NewFileBasedRouteHelper(),
		Proxy:           proxy.NewProxyHelper(),
		Wasm:            wasmhelper.NewWasmHelper(runtime.GOOS, runtime.GOARCH),
	}

	return cli
}

func (cli *GothicCli) GetAppId() (string, error) {
	if cli.appID != nil {
		return *cli.appID, nil
	}
	content, err := os.ReadFile(".gothicCli/app-id.txt")
	if err != nil {
		return "", fmt.Errorf("error reading file: %v", err)
	}

	// Convert the content to string
	appID := string(content)
	cli.appID = &appID
	return appID, nil
}

func (cli *GothicCli) GetConfig() (Config, error) {
	if cli.config != nil {
		return *cli.config, nil
	}
	var config Config
	file, err := os.Open("gothic-config.json")
	if err != nil {
		return Config{}, fmt.Errorf("error opening gothic-config.json: %w", err)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&config); err != nil {
		return Config{}, fmt.Errorf("error decoding gothic-config.json: %w", err)
	}
	if config.TailwindBinary != "" {
		cli.Tailwind.ConfigOverride = config.TailwindBinary
	}
	if config.WasmTinyGoVersion != "" {
		cli.Wasm.Version = config.WasmTinyGoVersion
	}
	if config.WasmBinary != "" {
		cli.Wasm.ConfigOverride = config.WasmBinary
	}
	cli.config = &config
	return config, nil
}

func (cli *GothicCli) InitializeModule(goModuleName string, frameworkVersion string) error {
	ctx := context.Background()
	if _, err := cliRunner.Run(ctx, "go", "mod", "init", goModuleName); err != nil {
		return fmt.Errorf("error running go mod init: %w", err)
	}
	// Pin the exact gothicframework version before go mod tidy so new projects
	// use the same version as the CLI that scaffolded them.
	if frameworkVersion != "" {
		if _, err := cliRunner.Run(ctx, "go", "get", "github.com/felipegenef/gothicframework/v2@"+frameworkVersion); err != nil {
			return fmt.Errorf("error pinning gothicframework version: %w", err)
		}
	}
	if _, err := cliRunner.Run(ctx, "go", "mod", "tidy"); err != nil {
		return fmt.Errorf("error running go mod tidy: %w", err)
	}
	return nil
}
