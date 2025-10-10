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
	TailWindFileName       string
	MainBinaryFileName     string
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
	Value interface{}
	Key   string
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
	Waf                   *WafConfig
	Env                   []EnvValueInfo
}

type WafConfig struct {
	DefaultAction string    `json:"defaultAction"`
	WafRules      []WafRule `json:"wafRules"`
}

type WafRule struct {
	Name             string           `json:"name"`
	Priority         int              `json:"priority"`
	Action           Action           `json:"action"`
	Statement        Statement        `json:"statement"`
	VisibilityConfig VisibilityConfig `json:"visibilityConfig"`
}

type Action struct {
	Block map[string]interface{} `json:"block,omitempty"` // empty object
}

type Statement struct {
	IPSetReferenceStatement *IPSetReferenceStatement `json:"ipSetReferenceStatement,omitempty"`
	GeoMatchStatement       *GeoMatchStatement       `json:"geoMatchStatement,omitempty"`
	ByteMatchStatement      *ByteMatchStatement      `json:"byteMatchStatement,omitempty"`
	RateBasedStatement      *RateBasedStatement      `json:"rateBasedStatement,omitempty"`
}

type IPSetReferenceStatement struct {
	Arn string `json:"arn"`
}

type GeoMatchStatement struct {
	CountryCodes []string `json:"countryCodes"`
}

type ByteMatchStatement struct {
	FieldToMatch         FieldToMatch         `json:"fieldToMatch"`
	PositionalConstraint string               `json:"positionalConstraint"`
	SearchString         string               `json:"searchString"`
	TextTransformations  []TextTransformation `json:"textTransformations"`
}

type FieldToMatch struct {
	Headers *HeaderMatch `json:"headers,omitempty"`
	UriPath *struct{}    `json:"uriPath,omitempty"`
}

type HeaderMatch struct {
	Name       string `json:"name"`
	MatchScope string `json:"matchScope"`
}

type TextTransformation struct {
	Priority int    `json:"priority"`
	Type     string `json:"type"`
}

type RateBasedStatement struct {
	Limit              int                 `json:"limit"`
	AggregateKeyType   string              `json:"aggregateKeyType"`
	ScopeDownStatement *ByteMatchStatement `json:"scopeDownStatement,omitempty"`
}

type VisibilityConfig struct {
	SampledRequestsEnabled   bool   `json:"sampledRequestsEnabled"`
	CloudWatchMetricsEnabled bool   `json:"cloudWatchMetricsEnabled"`
	MetricName               string `json:"metricName"`
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
