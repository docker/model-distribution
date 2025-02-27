package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/docker/model-distribution/pkg/distribution"
	"github.com/docker/model-distribution/pkg/types"
)

// Define an interface for the client methods we use in the commands
type ClientInterface interface {
	PullModel(ctx context.Context, reference string) (string, error)
	PushModel(ctx context.Context, source, reference string) error
	ListModels() ([]*types.Model, error)
	GetModel(reference string) (*types.Model, error)
	GetModelPath(reference string) (string, error)
}

// Ensure distribution.Client implements ClientInterface
var _ ClientInterface = (*distribution.Client)(nil)

// TestMainHelp tests the help command
func TestMainHelp(t *testing.T) {
	if os.Getenv("GO_RUN_INTEGRATION_TESTS") != "1" {
		t.Skip("Skipping integration test; set GO_RUN_INTEGRATION_TESTS=1 to run")
	}

	cmd := exec.Command("go", "run", "main.go", "--help")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to run help command: %v\nOutput: %s", err, output)
	}

	// Check that the output contains the usage information
	if !strings.Contains(string(output), "Usage:") {
		t.Errorf("Help output does not contain usage information")
	}

	// Check that the output contains the commands
	commands := []string{"pull", "push", "list", "get", "get-path"}
	for _, cmd := range commands {
		if !strings.Contains(string(output), cmd) {
			t.Errorf("Help output does not contain command: %s", cmd)
		}
	}
}

// TestMainVersion tests the version command
func TestMainVersion(t *testing.T) {
	if os.Getenv("GO_RUN_INTEGRATION_TESTS") != "1" {
		t.Skip("Skipping integration test; set GO_RUN_INTEGRATION_TESTS=1 to run")
	}

	cmd := exec.Command("go", "run", "main.go", "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to run version command: %v\nOutput: %s", err, output)
	}

	// Check that the output contains the version information
	if !strings.Contains(string(output), "version") {
		t.Errorf("Version output does not contain version information")
	}
}

// Helper function to wrap command functions to accept our interface
func wrapCmdPull(client ClientInterface, args []string) int {
	return cmdPull(client.(*distribution.Client), args)
}

func wrapCmdPush(client ClientInterface, args []string) int {
	return cmdPush(client.(*distribution.Client), args)
}

func wrapCmdList(client ClientInterface, args []string) int {
	return cmdList(client.(*distribution.Client), args)
}

func wrapCmdGet(client ClientInterface, args []string) int {
	return cmdGet(client.(*distribution.Client), args)
}

func wrapCmdGetPath(client ClientInterface, args []string) int {
	return cmdGetPath(client.(*distribution.Client), args)
}

// TestMainPull tests the pull command
func TestMainPull(t *testing.T) {
	// Create a temporary directory for the test
	tempDir, err := os.MkdirTemp("", "model-distribution-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a model store directory
	storeDir := filepath.Join(tempDir, "model-store")
	if err := os.MkdirAll(storeDir, 0755); err != nil {
		t.Fatalf("Failed to create model store directory: %v", err)
	}

	// Create a client for testing
	client, err := distribution.NewClient(storeDir)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Test the pull command with invalid arguments
	exitCode := cmdPull(client, []string{})
	if exitCode != 1 {
		t.Errorf("Pull command with invalid arguments should fail")
	}
}

// TestMainPush tests the push command
func TestMainPush(t *testing.T) {
	// Create a temporary directory for the test
	tempDir, err := os.MkdirTemp("", "model-distribution-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a client for testing
	client, err := distribution.NewClient(tempDir)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Test the push command with invalid arguments
	exitCode := cmdPush(client, []string{})
	if exitCode != 1 {
		t.Errorf("Push command with invalid arguments should fail")
	}
}

// TestMainList tests the list command
func TestMainList(t *testing.T) {
	// Create a temporary directory for the test
	tempDir, err := os.MkdirTemp("", "model-distribution-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a client for testing
	client, err := distribution.NewClient(tempDir)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Test the list command
	exitCode := cmdList(client, []string{})
	if exitCode != 0 {
		t.Errorf("List command failed with exit code: %d", exitCode)
	}
}

// TestMainGet tests the get command
func TestMainGet(t *testing.T) {
	// Create a temporary directory for the test
	tempDir, err := os.MkdirTemp("", "model-distribution-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a client for testing
	client, err := distribution.NewClient(tempDir)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Test the get command with invalid arguments
	exitCode := cmdGet(client, []string{})
	if exitCode != 1 {
		t.Errorf("Get command with invalid arguments should fail")
	}
}

// TestMainGetPath tests the get-path command
func TestMainGetPath(t *testing.T) {
	// Create a temporary directory for the test
	tempDir, err := os.MkdirTemp("", "model-distribution-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a client for testing
	client, err := distribution.NewClient(tempDir)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Test the get-path command with invalid arguments
	exitCode := cmdGetPath(client, []string{})
	if exitCode != 1 {
		t.Errorf("Get-path command with invalid arguments should fail")
	}
}
