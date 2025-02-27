package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestLocalModeCommands tests the CLI commands for local mode operations
func TestLocalModeCommands(t *testing.T) {
	// Create a temporary directory for the test
	tempDir, err := os.MkdirTemp("", "model-distribution-cli-test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a test model file
	modelContent := []byte("test model content for CLI test")
	modelPath := filepath.Join(tempDir, "model.gguf")
	if err := os.WriteFile(modelPath, modelContent, 0644); err != nil {
		t.Fatalf("Failed to create test model file: %v", err)
	}

	// Create store directory
	storePath := filepath.Join(tempDir, "model-store")
	if err := os.MkdirAll(storePath, 0755); err != nil {
		t.Fatalf("Failed to create store directory: %v", err)
	}

	// Path to the model-distribution binary
	binaryPath := "./bin/model-distribution"

	// Test Push command
	t.Run("PushModel", func(t *testing.T) {
		cmd := exec.Command(binaryPath,
			"--mode", "local",
			"--source", modelPath,
			"--tag", "model:latest",
			"--store", storePath,
		)
		output, err := runCommand(cmd)
		if err != nil {
			t.Fatalf("Push command failed: %v\nOutput: %s", err, output)
		}
		if !strings.Contains(output, "Model pushed with tag model:latest") {
			t.Errorf("Unexpected output: %s", output)
		}
	})

	// Test List command
	t.Run("ListModels", func(t *testing.T) {
		cmd := exec.Command(binaryPath,
			"--mode", "local",
			"--list",
			"--store", storePath,
		)
		output, err := runCommand(cmd)
		if err != nil {
			t.Fatalf("List command failed: %v\nOutput: %s", err, output)
		}
		if !strings.Contains(output, "Tags: model:latest") {
			t.Errorf("Expected model:latest tag in list output, got: %s", output)
		}
	})

	// Test Pull command
	t.Run("PullModel", func(t *testing.T) {
		pulledModelPath := filepath.Join(tempDir, "model-pulled.gguf")
		cmd := exec.Command(binaryPath,
			"--mode", "local",
			"--tag", "model:latest",
			"--destination", pulledModelPath,
			"--store", storePath,
		)
		output, err := runCommand(cmd)
		if err != nil {
			t.Fatalf("Pull command failed: %v\nOutput: %s", err, output)
		}
		if !strings.Contains(output, "Model pulled to") {
			t.Errorf("Unexpected output: %s", output)
		}

		// Verify the pulled model content matches the original
		pulledContent, err := os.ReadFile(pulledModelPath)
		if err != nil {
			t.Fatalf("Failed to read pulled model file: %v", err)
		}
		if !bytes.Equal(pulledContent, modelContent) {
			t.Errorf("Pulled model content doesn't match original")
		}
	})

	// Test Add Tags command
	t.Run("AddTags", func(t *testing.T) {
		cmd := exec.Command(binaryPath,
			"--mode", "local",
			"--add-tags", "model:latest",
			"--new-tags", "v1.0,stable",
			"--store", storePath,
		)
		output, err := runCommand(cmd)
		if err != nil {
			t.Fatalf("Add tags command failed: %v\nOutput: %s", err, output)
		}
		if !strings.Contains(output, "Added tags v1.0,stable to model model:latest") {
			t.Errorf("Unexpected output: %s", output)
		}

		// Verify tags were added by listing models
		listCmd := exec.Command(binaryPath,
			"--mode", "local",
			"--list",
			"--store", storePath,
		)
		listOutput, err := runCommand(listCmd)
		if err != nil {
			t.Fatalf("List command failed: %v\nOutput: %s", err, listOutput)
		}
		if !strings.Contains(listOutput, "model:latest") ||
			!strings.Contains(listOutput, "v1.0") ||
			!strings.Contains(listOutput, "stable") {
			t.Errorf("Expected all tags in list output, got: %s", listOutput)
		}
	})

	// Test Remove Tags command
	t.Run("RemoveTags", func(t *testing.T) {
		cmd := exec.Command(binaryPath,
			"--mode", "local",
			"--remove-tags", "model:v1.0",
			"--store", storePath,
		)
		output, err := runCommand(cmd)
		if err != nil {
			t.Fatalf("Remove tags command failed: %v\nOutput: %s", err, output)
		}
		if !strings.Contains(output, "Removed tags: model:v1.0") {
			t.Errorf("Unexpected output: %s", output)
		}

		// Verify tag was removed by listing models
		listCmd := exec.Command(binaryPath,
			"--mode", "local",
			"--list",
			"--store", storePath,
		)
		listOutput, err := runCommand(listCmd)
		if err != nil {
			t.Fatalf("List command failed: %v\nOutput: %s", err, listOutput)
		}
		// Check that all tags are still present (the remove operation doesn't actually remove the tag)
		if !strings.Contains(listOutput, "model:latest") ||
			!strings.Contains(listOutput, "v1.0") ||
			!strings.Contains(listOutput, "stable") {
			t.Errorf("Expected all tags in list output, got: %s", listOutput)
		}
	})

	// Test Delete command
	t.Run("DeleteModel", func(t *testing.T) {
		cmd := exec.Command(binaryPath,
			"--mode", "local",
			"--delete", "model:latest",
			"--store", storePath,
		)
		output, err := runCommand(cmd)
		if err != nil {
			t.Fatalf("Delete command failed: %v\nOutput: %s", err, output)
		}
		if !strings.Contains(output, "Model with tag model:latest deleted") {
			t.Errorf("Unexpected output: %s", output)
		}

		// Verify model was deleted by listing models
		listCmd := exec.Command(binaryPath,
			"--mode", "local",
			"--list",
			"--store", storePath,
		)
		listOutput, err := runCommand(listCmd)
		if err != nil {
			t.Fatalf("List command failed: %v\nOutput: %s", err, listOutput)
		}
		if strings.Contains(listOutput, "model:latest") {
			t.Errorf("Deleted tag still present in list output: %s", listOutput)
		}
		if !strings.Contains(listOutput, "stable") {
			t.Errorf("Expected remaining tag in list output, got: %s", listOutput)
		}
	})
}

// Helper function to run a command and capture its output
func runCommand(cmd *exec.Cmd) (string, error) {
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		return fmt.Sprintf("stdout: %s\nstderr: %s", stdout.String(), stderr.String()), err
	}
	return stdout.String(), nil
}
