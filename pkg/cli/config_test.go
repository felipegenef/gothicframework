package cli

import (
	"strings"
	"testing"
)

func TestValidateBucketName(t *testing.T) {
	tests := []struct {
		name    string
		bucket  string
		wantErr bool
	}{
		{
			name:    "valid simple bucket",
			bucket:  "my-project-dev-abc123",
			wantErr: false,
		},
		{
			name:    "valid with dots",
			bucket:  "my.project.dev",
			wantErr: false,
		},
		{
			name:    "too short",
			bucket:  "ab",
			wantErr: true,
		},
		{
			name:    "starts with hyphen",
			bucket:  "-my-bucket",
			wantErr: true,
		},
		{
			name:    "contains uppercase",
			bucket:  "My-Bucket",
			wantErr: true,
		},
		{
			name:    "contains spaces",
			bucket:  "my bucket",
			wantErr: true,
		},
		{
			name:    "empty string",
			bucket:  "",
			wantErr: true,
		},
		{
			name:    "valid minimum length",
			bucket:  "abc",
			wantErr: false,
		},
		{
			name:    "ends with hyphen",
			bucket:  "my-bucket-",
			wantErr: true,
		},
		{
			name:    "ends with dot",
			bucket:  "my.bucket.",
			wantErr: true,
		},
		{
			name:    "valid max length 63",
			bucket:  "a" + strings.Repeat("b", 61) + "c",
			wantErr: false,
		},
		{
			name:    "too long 64",
			bucket:  "a" + strings.Repeat("b", 62) + "c",
			wantErr: true,
		},
		{
			name:    "single char too short",
			bucket:  "a",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateBucketName(tt.bucket)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateBucketName(%q) error = %v, wantErr %v", tt.bucket, err, tt.wantErr)
			}
		})
	}
}

func TestValidateStageName(t *testing.T) {
	tests := []struct {
		name    string
		stage   string
		wantErr bool
	}{
		{
			name:    "valid simple stage",
			stage:   "dev",
			wantErr: false,
		},
		{
			name:    "valid with numbers",
			stage:   "stage2",
			wantErr: false,
		},
		{
			name:    "valid uppercase",
			stage:   "Production",
			wantErr: false,
		},
		{
			name:    "invalid with hyphens",
			stage:   "my-stage",
			wantErr: true,
		},
		{
			name:    "invalid with spaces",
			stage:   "my stage",
			wantErr: true,
		},
		{
			name:    "empty string",
			stage:   "",
			wantErr: true,
		},
		{
			name:    "invalid with special chars",
			stage:   "dev;rm -rf",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateStageName(tt.stage)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateStageName(%q) error = %v, wantErr %v", tt.stage, err, tt.wantErr)
			}
		})
	}
}
