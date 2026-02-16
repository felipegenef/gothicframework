package helpers

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"text/template"
)

type InitCmdTemplateInfo struct {
	ProjectName            string
	GoModName              string
	MainServerPackageName  string
	MainServerFunctionName string
	PageName               string
	RouteName              string
	ComponentName          string
}

type RouteTemplateInfo struct {
	PageName      string
	RouteName     string
	ComponentName string
	GoModName     string
}

type EnvValueInfo struct {
	Value        interface{}
	Key          string // Lambda env var name (spaces replaced with underscores)
	SanitizedKey string // Alphanumeric-only key for CloudFormation Mappings lookups
}
type StageTemplateInfo struct {
	Name                  string
	BucketName            string
	LambdaName            string
	CustomDomain          string
	HostedZone            string
	CertificateArn        string
	IsCustomDomainWithArn bool
	IsCustomDomain        bool
	WafArn                string
	Env                   []EnvValueInfo
}

type SamYamlTemplateInfo struct {
	Timeout           int
	MemorySize        int
	UsedTemplateName  string
	ProjectName       string
	StageTemplateInfo StageTemplateInfo
}
type SamTomlTemplateInfo struct {
	StackName string
	AwsRegion string
}

type TemplateHelper struct {
	InitCmdTemplateInfo InitCmdTemplateInfo
	RouteTemplateInfo   RouteTemplateInfo
}

func NewTemplateHelper() TemplateHelper {
	return TemplateHelper{}
}

func (helper *TemplateHelper) UpdateFromTemplate(templateFilePath string, outputFilePath string, templateStruct interface{}) error {
	templateFileData, err := os.ReadFile(templateFilePath)
	if err != nil {
		return err
	}
	data := template.Must(template.New(templateFilePath).Parse(string(templateFileData)))
	// Cria ou abre o arquivo de saída
	outFile, err := os.Create(outputFilePath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	err = data.Execute(outFile, templateStruct)
	if err != nil {
		return fmt.Errorf("error replacing go module name to file %s: %w", outputFilePath, err)
	}

	return nil
}

func (helper *TemplateHelper) CreateFromTemplate(fileTemplate embed.FS, templateFilePath string, outputFilePath string, templateStruct interface{}) error {
	templateBytes, err := fs.ReadFile(fileTemplate, templateFilePath)
	if err != nil {
		return err
	}
	data := template.Must(template.New(templateFilePath).Parse(string(templateBytes)))
	// Cria ou abre o arquivo de saída
	outFile, err := os.Create(outputFilePath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	err = data.Execute(outFile, templateStruct)
	if err != nil {
		return fmt.Errorf("error replacing go module name to file %s: %w", outputFilePath, err)
	}

	return nil
}

func (helper *TemplateHelper) CopyFile(filePath string, destinationPath string) error {
	fileContent, err := os.ReadFile(filePath)

	if err != nil {
		return err
	}

	return os.WriteFile(destinationPath, fileContent, 0644)
}
func (helper *TemplateHelper) DeleteFile(filePath string) error {
	return os.Remove(filePath)

}

func (helper *TemplateHelper) CopyFromFs(fileTemplate embed.FS, templateFilePath string, outputFilePath string) error {
	templateBytes, err := fs.ReadFile(fileTemplate, templateFilePath)
	if err != nil {
		return err
	}
	return os.WriteFile(outputFilePath, templateBytes, 0644)
}
