package helpers

import (
	"fmt"
	"os"
	"os/exec"
)

type AwsSamHelper struct {
}

func NewAwsSamHelper() AwsSamHelper {
	return AwsSamHelper{}
}

func (helper *AwsSamHelper) Build() error {
	samBuildCMD := exec.Command("sam", "build")
	samBuildCMD.Stdout = os.Stdout
	samBuildCMD.Stdin = os.Stdin
	samBuildCMD.Stderr = os.Stderr

	if err := samBuildCMD.Run(); err != nil {
		return fmt.Errorf("error building AWS SAM app: %w", err)
	}
	return nil
}

func (helper *AwsSamHelper) Deploy(stage string, stackName string, awsProfile string) error {
	samDeployCMD := exec.Command("sam", "deploy", "--stack-name", stackName+"-"+stage, "--parameter-overrides", "Stage="+stage, "--profile", awsProfile)
	samDeployCMD.Stdout = os.Stdout
	samDeployCMD.Stdin = os.Stdin
	samDeployCMD.Stderr = os.Stderr

	if err := samDeployCMD.Run(); err != nil {
		return fmt.Errorf("error deploying app: %w", err)
	}
	return nil
}

func (helper *AwsSamHelper) DeleteStack(stage string, stackName string, awsProfile string) error {
	samDeleteCMD := exec.Command("sam", "delete", "--stack-name", stackName+"-"+stage, "--profile", awsProfile)
	samDeleteCMD.Stdout = os.Stdout
	samDeleteCMD.Stdin = os.Stdin
	samDeleteCMD.Stderr = os.Stderr

	if err := samDeleteCMD.Run(); err != nil {
		return fmt.Errorf("error deleting app: %w", err)
	}
	return nil
}
