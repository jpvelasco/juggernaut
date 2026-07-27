// Package discovery — tests for loadAWSConfig.
package discovery

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
)

func TestLoadAWSConfig_HappyPath(t *testing.T) {
	original := loadDefaultAWSConfig
	t.Cleanup(func() { loadDefaultAWSConfig = original })

	const wantRegion = "us-east-1"
	loadDefaultAWSConfig = func(_ context.Context, optFns ...func(*config.LoadOptions) error) (aws.Config, error) {
		options := config.LoadOptions{}
		for _, fn := range optFns {
			if err := fn(&options); err != nil {
				return aws.Config{}, err
			}
		}
		return aws.Config{Region: options.Region}, nil
	}

	got, err := loadAWSConfig(context.Background(), wantRegion)
	if err != nil {
		t.Fatalf("loadAWSConfig() error: %v", err)
	}
	if got.Region != wantRegion {
		t.Errorf("expected region %q, got %q", wantRegion, got.Region)
	}
}

func TestLoadAWSConfig_ErrorPath(t *testing.T) {
	original := loadDefaultAWSConfig
	t.Cleanup(func() { loadDefaultAWSConfig = original })

	loadDefaultAWSConfig = func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return aws.Config{}, errors.New("credentials unavailable")
	}

	_, err := loadAWSConfig(context.Background(), "us-west-2")
	if err == nil {
		t.Fatal("expected error when loadDefaultAWSConfig fails")
	}
	if !strings.Contains(err.Error(), "loading AWS config") {
		t.Errorf("expected wrapped error message, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "credentials unavailable") {
		t.Errorf("expected original error message in chain, got %q", err.Error())
	}
}

func TestLoadAWSConfig_ZeroConfigOnError(t *testing.T) {
	original := loadDefaultAWSConfig
	t.Cleanup(func() { loadDefaultAWSConfig = original })

	loadDefaultAWSConfig = func(context.Context, ...func(*config.LoadOptions) error) (aws.Config, error) {
		return aws.Config{}, errors.New("no config")
	}

	got, err := loadAWSConfig(context.Background(), "eu-west-1")
	if err == nil {
		t.Fatal("expected error")
	}
	if got.Region != "" {
		t.Errorf("expected empty region in zero config, got %q", got.Region)
	}
}
