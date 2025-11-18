package cli

type Config struct {
	ProjectName    string `json:"projectName"`
	GoModName      string `json:"goModuleName"`
	OptimizeImages struct {
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
