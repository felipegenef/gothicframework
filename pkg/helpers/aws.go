package helpers

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
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

	// Delete existing S3 public folder contents so no stale files remain after deploy.
	removeFilesCmd := exec.Command("aws", "s3", "rm", bucketPublicFolderName, "--recursive", "--region", region, "--profile", awsProfile)
	removeFilesCmd.Stdout = os.Stdout
	removeFilesCmd.Stdin = os.Stdin
	removeFilesCmd.Stderr = os.Stderr
	if err := removeFilesCmd.Run(); err != nil {
		fmt.Printf("Error clearing S3 public folder before upload: %v", err)
		return err
	}
	fmt.Println("S3 public folder cleared.")

	addFilesCmd := exec.Command("aws", "s3", "cp", "public", bucketPublicFolderName, "--recursive", "--region", region, "--profile", awsProfile)
	addFilesCmd.Stdout = os.Stdout
	addFilesCmd.Stdin = os.Stdin
	addFilesCmd.Stderr = os.Stderr
	if err := addFilesCmd.Run(); err != nil {
		fmt.Printf("Error adding CloudFront assets: %v", err)
		return err
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
			wasmCmd := exec.Command("aws", "s3", "cp", "public/wasm",
				bucketPublicFolderName+"/wasm",
				"--recursive",
				"--exclude", "*",
				"--include", enc.glob,
				"--content-encoding", enc.encoding,
				"--content-type", "application/wasm",
				"--metadata-directive", "REPLACE",
				"--region", region, "--profile", awsProfile)
			wasmCmd.Stdout = os.Stdout
			wasmCmd.Stdin = os.Stdin
			wasmCmd.Stderr = os.Stderr
			if err := wasmCmd.Run(); err != nil {
				fmt.Printf("Error uploading %s WASM assets: %v\n", enc.encoding, err)
				return err
			}
		}
		fmt.Println("S3 WASM files uploaded with correct headers.")
	}
	return nil
}

func (helper *AwsHelper) RemoveCloudFrontAssets(originBucketName string, region string, awsProfile string) error {

	// Construct the S3 bucket name
	bucketPublicFolderName := "s3://" + originBucketName + "/public"

	removeFilesCmd := exec.Command("aws", "s3", "rm", bucketPublicFolderName, "--recursive", "--region", region, "--profile", awsProfile)
	removeFilesCmd.Stdout = os.Stdout
	removeFilesCmd.Stdin = os.Stdin
	removeFilesCmd.Stderr = os.Stderr

	// Run the command
	err := removeFilesCmd.Run()
	if err != nil {
		fmt.Printf("Error removing CloudFront Assets: %v", err)
		return err
	}
	fmt.Println("S3 Files deleted successfully.")

	return nil
}

func (helper *AwsHelper) CleanCloudFrontCache(stackName string, stage string, region string, awsProfile string) error {

	// Execute the command to get the CloudFront distribution ID
	getDistributionIdCMD := exec.Command("aws", "cloudformation", "describe-stacks", "--stack-name", stackName+"-"+stage, "--query", "Stacks[0].Outputs[?OutputKey=='CloudFrontId'].OutputValue", "--output", "text", "--region", region, "--profile", awsProfile)

	// Capture the output of the command
	var out bytes.Buffer
	getDistributionIdCMD.Stdout = &out
	err := getDistributionIdCMD.Run()
	if err != nil {
		fmt.Printf("Error getting CloudFront Id: %v", err)
		return err
	}

	// The result of the command will be the CloudFront distribution ID
	distributionId := strings.TrimSpace(out.String()) // Remove any extra spaces
	if distributionId == "" {
		fmt.Printf("CloudFront ID not found")
		return fmt.Errorf("CloudFront ID not found")
	}

	// Now, use the distribution ID in the command to create the invalidation
	cleanCachesCmd := exec.Command("aws", "cloudfront", "create-invalidation", "--distribution-id", distributionId, "--paths", "/*", "--region", region, "--profile", awsProfile)

	// Execute the cache cleanup command
	cleanCacheErr := cleanCachesCmd.Run()

	if cleanCacheErr != nil {
		fmt.Printf("Error cleaning up deploy files: %v", cleanCacheErr)
		return cleanCacheErr
	}

	// Print the distribution ID and confirm the cache cleanup
	fmt.Printf("Successfully reset CloudFront cache for distribution: %s\n", distributionId)
	return nil
}
