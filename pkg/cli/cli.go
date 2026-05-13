package cli

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"runtime"

	helpers "github.com/felipegenef/gothicframework/pkg/helpers"
	proxy "github.com/felipegenef/gothicframework/pkg/helpers/proxy"
	routes "github.com/felipegenef/gothicframework/pkg/helpers/routes"
)

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
	Wasm            helpers.WasmHelper
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
		Wasm:            helpers.NewWasmHelper(runtime.GOOS, runtime.GOARCH),
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

func (cli *GothicCli) InitializeModule(goModuleName string) error {
	initCmd := exec.Command("go", "mod", "init", goModuleName)
	initCmd.Stdin = os.Stdin
	initCmd.Stderr = os.Stderr
	if err := initCmd.Run(); err != nil {
		return fmt.Errorf("error running go mod init: %w", err)
	}
	tidyCmd := exec.Command("go", "mod", "tidy")
	tidyCmd.Stdin = os.Stdin
	tidyCmd.Stderr = os.Stderr
	if err := tidyCmd.Run(); err != nil {
		return fmt.Errorf("error running go mod tidy: %w", err)
	}
	return nil
}
