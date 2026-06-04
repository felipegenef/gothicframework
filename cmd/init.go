/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"regexp"
	"strings"

	gothci_cli "github.com/felipegenef/gothicframework/v2/pkg/cli"
	cli_data "github.com/felipegenef/gothicframework/v2/pkg/data"
	helpers "github.com/felipegenef/gothicframework/v2/pkg/helpers"
	"github.com/spf13/cobra"
	"github.com/teris-io/shortid"
	"golang.org/x/sync/errgroup"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize the project structure and configuration files for a Gothic app.",
	Long: `Sets up the initial folder structure and essential files required to start building a Gothic app.

This includes:
  - Auto-download of the Tailwind CSS standalone binary (cached in ~/.cache/gothic-cli/bin/)
  - A gothic-config.json file
  - A basic example app to help you get started
  - A link to the official documentation for further guidance`,
	RunE: newInitCommand(gothci_cli.NewCli()),
}

func init() {
	rootCmd.AddCommand(initCmd)
}

type InitCommand struct {
	gothicCliData cli_data.GothicCliData
	cli           *gothci_cli.GothicCli

	// gitRunner is an injectable seam for tests; the default runs the real
	// `git` command exactly as the previous inline code did.
	gitRunner func(args ...string) error
}

func NewInitCommandCli(cli *gothci_cli.GothicCli, gothicCliData cli_data.GothicCliData) InitCommand {
	command := InitCommand{
		cli:           cli,
		gothicCliData: gothicCliData,
	}
	command.gitRunner = defaultGitRunner
	return command
}

// defaultGitRunner runs `git <args...>`, mirroring the original
// exec.Command("git", "init").Run() behavior (errors are ignored by the
// caller, just as before).
func defaultGitRunner(args ...string) error {
	return exec.Command("git", args...).Run()
}

func newInitCommand(cli gothci_cli.GothicCli) RunEFunc {
	return func(cmd *cobra.Command, args []string) error {
		command := NewInitCommandCli(&cli, cli_data.DefaultCLIData)

		return command.CreateNewGothicApp(cli_data.DefaultCLIData)
	}
}

func (command *InitCommand) CreateNewGothicApp(data cli_data.GothicCliData) error {

	projectName, err := command.promptForProjectName()
	if err != nil {
		return err
	}
	data.ProjectName = projectName
	gomodName, err := command.promptForGoModName()
	if err != nil {
		return err
	}
	data.GoModName = gomodName
	command.gothicCliData = data

	if err := command.initializeProject(); err != nil {
		return err
	}
	// Pre-cache the Tailwind binary during init
	if _, err := command.cli.Tailwind.EnsureBinary(); err != nil {
		return fmt.Errorf("error downloading tailwind binary: %w", err)
	}
	if err := command.cli.InitializeModule(command.gothicCliData.GoModName, CURRENT_VERSION); err != nil {
		return err
	}
	if err := command.cli.Templ.Render(); err != nil {
		return err
	}

	if err := command.cli.FileBasedRouter.Render(gomodName); err != nil {
		return err
	}

	gitRunner := command.gitRunner
	if gitRunner == nil {
		gitRunner = defaultGitRunner
	}
	gitRunner("init")
	fmt.Println("Project initialized successfully!")
	return nil
}

// Function to create directories and files
func (command *InitCommand) initializeProject() error {

	command.cli.Templates.InitCmdTemplateInfo = helpers.InitCmdTemplateInfo{
		ProjectName:            command.gothicCliData.ProjectName,
		GoModName:              command.gothicCliData.GoModName,
		MainServerPackageName:  "package main",
		MainServerFunctionName: "main()",
	}

	if err := command.createInitialDirs(); err != nil {
		return err
	}
	// Create dot files (embed api wont let dots on files)
	if err := command.createHiddenFiles(); err != nil {
		return err
	}

	// Create initial file structure
	if err := command.createInitialFileStructure(); err != nil {
		return err
	}
	// create all custom template files
	if err := command.createTemplateBasedFiles(); err != nil {
		return err
	}
	return nil
}

func (command *InitCommand) createInitialDirs() error {
	for _, dir := range command.gothicCliData.InitialDirs {
		if err := os.MkdirAll(dir, os.ModePerm); err != nil {
			return fmt.Errorf("error generating initial Directories: %v", err)
		}
	}
	return nil
}

func (command *InitCommand) createHiddenFiles() error {

	upperId, err := shortid.Generate()
	if err != nil {
		return fmt.Errorf("error generating app ID: %v", err)
	}
	// Replace all special characters with -
	re := regexp.MustCompile(`[^\w\s]|_`)
	lowerId := strings.ToLower(upperId)
	id := re.ReplaceAllString(lowerId, "-")

	g := new(errgroup.Group)

	g.Go(func() error {
		return os.WriteFile(".gothicCli/app-id.txt", []byte(id), 0644)
	})

	g.Go(func() error {
		return os.WriteFile(".env", []byte(command.gothicCliData.Env), 0644)
	})

	g.Go(func() error {
		return os.WriteFile(".gitignore", []byte(command.gothicCliData.GitIgnore), 0644)
	})

	return g.Wait()

}

func (command *InitCommand) createInitialFileStructure() error {
	mainServerData, err := fs.ReadFile(command.gothicCliData.ServerFolder, "server/server.go")
	if err != nil {
		return fmt.Errorf("error reading embedded server template: %w", err)
	}
	if err := os.WriteFile("main.go", mainServerData, 0644); err != nil {
		return fmt.Errorf("error creating file %s: %w", "main.go", err)
	}
	command.cli.Templates.UpdateFromTemplate("main.go", "main.go", command.cli.Templates.InitCmdTemplateInfo)

	g := new(errgroup.Group)

	for filename, fileContent := range command.gothicCliData.InitialFiles {
		g.Go(func() error {
			if err := command.cli.Templates.CreateFromTemplate(fileContent, filename, filename, command.cli.Templates.InitCmdTemplateInfo); err != nil {
				return fmt.Errorf("error creating file %s: %w", filename, err)
			}
			return nil
		})
	}

	for filename, fileContent := range command.gothicCliData.TemplateFiles {
		g.Go(func() error {
			if err := command.cli.Templates.CopyFromFs(fileContent, filename, filename); err != nil {
				return fmt.Errorf("error copying file %s: %w", filename, err)
			}
			return nil
		})
	}

	for filename, fileContent := range command.gothicCliData.PublicFolderAssets {
		g.Go(func() error {
			data, err := fs.ReadFile(fileContent, filename)
			if err != nil {
				return fmt.Errorf("error reading embedded asset %s: %w", filename, err)
			}

			if err := os.WriteFile(filename, data, 0644); err != nil {
				return fmt.Errorf("error creating file %s: %w", filename, err)
			}
			return nil
		})
	}

	return g.Wait()
}

func (command *InitCommand) createTemplateBasedFiles() error {
	g := new(errgroup.Group)

	// Pages
	for templateFilePath, pageName := range command.gothicCliData.CustomTemplateBasedPages {
		g.Go(func() error {
			if err := command.cli.Templates.CreateFromTemplate(command.gothicCliData.SrcFolder, templateFilePath, templateFilePath, helpers.RouteTemplateInfo{PageName: pageName, GoModName: command.gothicCliData.GoModName}); err != nil {
				return fmt.Errorf("error creating page file %s: %w", templateFilePath, err)
			}
			return nil
		})
	}

	// Components
	for templateFilePath, componentName := range command.gothicCliData.CustomTemplateBasedComponents {
		g.Go(func() error {
			if err := command.cli.Templates.CreateFromTemplate(command.gothicCliData.SrcFolder, templateFilePath, templateFilePath, helpers.RouteTemplateInfo{ComponentName: componentName, GoModName: command.gothicCliData.GoModName}); err != nil {
				return fmt.Errorf("error creating component file %s: %w", templateFilePath, err)
			}
			return nil
		})
	}

	// API Routes
	for templateFilePath, routeName := range command.gothicCliData.CustomTemplateBasedRoutes {
		g.Go(func() error {
			if err := command.cli.Templates.CreateFromTemplate(command.gothicCliData.SrcFolder, templateFilePath, templateFilePath, helpers.RouteTemplateInfo{RouteName: routeName, GoModName: command.gothicCliData.GoModName}); err != nil {
				return fmt.Errorf("error creating api route file %s: %w", templateFilePath, err)
			}
			return nil
		})
	}

	return g.Wait()
}

func (command *InitCommand) promptForProjectName() (string, error) {
	var name string
	fmt.Print("Enter your unique stack name in kebab case (e.g., your-unique-stack-name): ")
	fmt.Scanln(&name)

	// Validate kebab case
	if matched, _ := regexp.MatchString(`^[a-z0-9]+(-[a-z0-9]+)*$`, name); !matched {
		return "", fmt.Errorf("invalid name format. Please use kebab case (lowercase letters and numbers only, with dashes)")
	}
	if name == "" {
		return "", fmt.Errorf("project name cannot be empty")
	}
	return name, nil
}

func (command *InitCommand) promptForGoModName() (string, error) {
	var name string
	fmt.Print("Enter your go module name: ")
	fmt.Scanln(&name)
	if name == "" {
		return "", fmt.Errorf("go module name cannot be empty")
	}
	return name, nil
}
