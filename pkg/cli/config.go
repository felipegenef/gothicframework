package cli

import (
	"fmt"
	"regexp"
)

var validS3BucketName = regexp.MustCompile(`^[a-z0-9][a-z0-9.\-]{1,61}[a-z0-9]$`)
var validStageName = regexp.MustCompile(`^[a-zA-Z0-9]+$`)

func ValidateBucketName(name string) error {
	if !validS3BucketName.MatchString(name) {
		return fmt.Errorf("invalid S3 bucket name %q: must be 3-63 characters, lowercase alphanumeric, dots, or hyphens", name)
	}
	return nil
}

func ValidateStageName(name string) error {
	if !validStageName.MatchString(name) {
		return fmt.Errorf("invalid stage name %q: must be alphanumeric only", name)
	}
	return nil
}

type Config struct {
	ProjectName       string `json:"projectName"`
	GoModName         string `json:"goModuleName"`
	TailwindBinary    string `json:"tailwindBinary,omitempty"`
	WasmBinary        string `json:"wasmBinary,omitempty"`
	WasmTinyGoVersion string `json:"wasmTinyGoVersion,omitempty"`
	OptimizeImages    struct {
		LowResolutionRate int `json:"lowResolutionRate"`
	} `json:"optimizeImages"`
	Deploy *DeployConfig `json:"deploy"`
}

type DeployConfig struct {
	ServerMemory  int                     `json:"serverMemory"`
	ServerTimeout int                     `json:"serverTimeout"`
	Region        string                  `json:"region"`
	Profile       string                  `json:"profile"`
	Stages        map[string]EnvVariables `json:"stages"`
	CustomDomain  bool                    `json:"customDomain"`
}
type EnvVariables struct {
	BucketName     string
	LambdaName     string
	HostedZoneId   *string                `json:"hostedZoneId"`
	CustomDomain   *string                `json:"customDomain"`
	CertificateArn *string                `json:"certificateArn"`
	WafArn         *string                `json:"wafArn"`
	ENV            map[string]interface{} `json:"env,omitempty"`
}
