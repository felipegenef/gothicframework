package helpers

import (
	"context"
	"fmt"
	"os"
)

type AwsSamHelper struct {
}

func NewAwsSamHelper() AwsSamHelper {
	return AwsSamHelper{}
}

func (helper *AwsSamHelper) Build() error {
	out, err := runner.Run(context.Background(), "sam", "build")
	if err != nil {
		return fmt.Errorf("error building AWS SAM app: %w", err)
	}
	os.Stdout.Write(out)
	return nil
}

func (helper *AwsSamHelper) Deploy(stage string, stackName string, awsProfile string) error {
	out, err := runner.Run(context.Background(), "sam", "deploy", "--stack-name", stackName+"-"+stage, "--parameter-overrides", "Stage="+stage, "--profile", awsProfile)
	if err != nil {
		return fmt.Errorf("error deploying app: %w", err)
	}
	os.Stdout.Write(out)
	return nil
}

func (helper *AwsSamHelper) DeleteStack(stage string, stackName string, awsProfile string) error {
	out, err := runner.Run(context.Background(), "sam", "delete", "--stack-name", stackName+"-"+stage, "--profile", awsProfile)
	if err != nil {
		return fmt.Errorf("error deleting app: %w", err)
	}
	os.Stdout.Write(out)
	return nil
}
