package helpers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type AwsHelper struct{}

func (helper *AwsHelper) hasWasmFiles(dir, glob string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if matched, _ := filepath.Match(glob, e.Name()); matched {
			return true
		}
	}
	return false
}

func NewAwsHelper() AwsHelper {
	return AwsHelper{}
}

func (helper *AwsHelper) AddCloudFrontAssets(originBucketName string, region string, awsProfile string) error {

	bucketPublicFolderName := "s3://" + originBucketName + "/public"

	ctx := context.Background()

	// Delete existing S3 public folder contents so no stale files remain after deploy.
	if out, err := runner.Run(ctx, "aws", "s3", "rm", bucketPublicFolderName, "--recursive", "--region", region, "--profile", awsProfile); err != nil {
		fmt.Printf("Error clearing S3 public folder before upload: %v", err)
		return err
	} else {
		os.Stdout.Write(out)
	}
	fmt.Println("S3 public folder cleared.")

	if out, err := runner.Run(ctx, "aws", "s3", "cp", "public", bucketPublicFolderName, "--recursive", "--region", region, "--profile", awsProfile); err != nil {
		fmt.Printf("Error adding CloudFront assets: %v", err)
		return err
	} else {
		os.Stdout.Write(out)
	}
	fmt.Println("S3 Files added successfully.")

	// Re-upload public/wasm/ with the correct WASM metadata so browsers receive
	// Content-Type: application/wasm and the right Content-Encoding per file type.
	// The recursive upload above sets Content-Type: application/gzip (wrong).
	if _, err := os.Stat("public/wasm"); err == nil {
		for _, enc := range []struct{ glob, encoding string }{
			{"*.wasm.gz", "gzip"},
			{"*.wasm.br", "br"},
		} {
			if !helper.hasWasmFiles("public/wasm", enc.glob) {
				continue
			}
			if out, err := runner.Run(ctx, "aws", "s3", "cp", "public/wasm",
				bucketPublicFolderName+"/wasm",
				"--recursive",
				"--exclude", "*",
				"--include", enc.glob,
				"--content-encoding", enc.encoding,
				"--content-type", "application/wasm",
				"--metadata-directive", "REPLACE",
				"--region", region, "--profile", awsProfile); err != nil {
				fmt.Printf("Error uploading %s WASM assets: %v\n", enc.encoding, err)
				return err
			} else {
				os.Stdout.Write(out)
			}
		}
		fmt.Println("S3 WASM files uploaded with correct headers.")
	}
	return nil
}

func (helper *AwsHelper) RemoveCloudFrontAssets(originBucketName string, region string, awsProfile string) error {

	// Construct the S3 bucket name
	bucketPublicFolderName := "s3://" + originBucketName + "/public"

	out, err := runner.Run(context.Background(), "aws", "s3", "rm", bucketPublicFolderName, "--recursive", "--region", region, "--profile", awsProfile)
	if err != nil {
		fmt.Printf("Error removing CloudFront Assets: %v", err)
		return err
	}
	os.Stdout.Write(out)
	fmt.Println("S3 Files deleted successfully.")

	return nil
}

func (helper *AwsHelper) CleanCloudFrontCache(stackName string, stage string, region string, awsProfile string) error {

	ctx := context.Background()

	// Execute the command to get the CloudFront distribution ID
	out, err := runner.Run(ctx, "aws", "cloudformation", "describe-stacks", "--stack-name", stackName+"-"+stage, "--query", "Stacks[0].Outputs[?OutputKey=='CloudFrontId'].OutputValue", "--output", "text", "--region", region, "--profile", awsProfile)
	if err != nil {
		fmt.Printf("Error getting CloudFront Id: %v", err)
		return err
	}

	// The result of the command will be the CloudFront distribution ID
	distributionId := strings.TrimSpace(string(out)) // Remove any extra spaces
	if distributionId == "" {
		fmt.Printf("CloudFront ID not found")
		return fmt.Errorf("CloudFront ID not found")
	}

	// Now, use the distribution ID in the command to create the invalidation
	_, cleanCacheErr := runner.Run(ctx, "aws", "cloudfront", "create-invalidation", "--distribution-id", distributionId, "--paths", "/*", "--region", region, "--profile", awsProfile)
	if cleanCacheErr != nil {
		fmt.Printf("Error cleaning up deploy files: %v", cleanCacheErr)
		return cleanCacheErr
	}

	// Print the distribution ID and confirm the cache cleanup
	fmt.Printf("Successfully reset CloudFront cache for distribution: %s\n", distributionId)
	return nil
}
