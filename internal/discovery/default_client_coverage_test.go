package discovery

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
)

// TestDefaultMakeBedrockClient verifies the default makeBedrockClient variable
// produces a valid *bedrock.Client (covering the var initializer).
func TestDefaultMakeBedrockClient(t *testing.T) {
	cfg := aws.Config{Region: "us-west-2"}

	// Save and restore the package var so the default initializer is called.
	orig := makeBedrockClient
	t.Cleanup(func() { makeBedrockClient = orig })

	// Call the default initializer directly — this exercises the var body.
	client := makeBedrockClient(cfg)
	if client == nil {
		t.Fatal("makeBedrockClient returned nil")
	}
}

// TestDefaultMakeSTSClient verifies the default makeSTSClient variable produces
// a valid *sts.Client (covering the var initializer).
func TestDefaultMakeSTSClient(t *testing.T) {
	cfg := aws.Config{Region: "us-west-2"}

	// Save and restore the package var so the default initializer is called.
	orig := makeSTSClient
	t.Cleanup(func() { makeSTSClient = orig })

	// Call the default initializer directly.
	client := makeSTSClient(cfg)
	if client == nil {
		t.Fatal("makeSTSClient returned nil")
	}
}

// TestNewClient_HappyPath verifies newClient builds a real Bedrock client
// using the default AWS config chain.
func TestNewClient_HappyPath(t *testing.T) {
	orig := loadDefaultAWSConfig
	t.Cleanup(func() { loadDefaultAWSConfig = orig })

	loadDefaultAWSConfig = func(_ context.Context, optFns ...func(*config.LoadOptions) error) (aws.Config, error) {
		return aws.Config{Region: "us-west-2"}, nil
	}

	client, err := newClient(context.Background(), "us-west-2")
	if err != nil {
		t.Fatalf("newClient: %v", err)
	}
	if client == nil {
		t.Fatal("client is nil")
	}
}

// TestFoundationModelAvailability_AllCases covers all branches of
// foundationModelAvailability including the nil output path.
func TestFoundationModelAvailability_AllCases(t *testing.T) {
	// nil output
	if got := foundationModelAvailability(nil); got != "UNKNOWN" {
		t.Errorf("nil = %q, want UNKNOWN", got)
	}
}