package distribution

import (
	"github.com/docker/model-distribution/builder"
	"github.com/docker/model-distribution/tarball"
	"io"
	"os"
	"testing"
)

func TestImportModel(t *testing.T) {
	// Create temp directory for store
	tempDir, err := os.MkdirTemp("", "model-distribution-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create client
	client, err := NewClient(WithStoreRootPath(tempDir))
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	pr, pw := io.Pipe()
	target, err := tarball.NewTarget(pw)
	if err != nil {
		t.Fatalf("Failed to create target: %v", err)
	}
	done := make(chan error)
	go func() {
		done <- client.ImportModel(t.Context(), "some/model", pr, nil)
	}()
	// Create model archive
	bldr, err := builder.FromGGUF(testGGUFFile)
	if err != nil {
		t.Fatalf("Failed to create builder: %v", err)
	}
	err = bldr.Build(t.Context(), target, nil)
	if err != nil {
		t.Fatalf("Failed to build model: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Failed to import model: %v", err)
		}
	case <-t.Context().Done():
	}

	if _, err := client.GetModel("some/model"); err != nil {
		t.Fatalf("Failed to get model: %v", err)
	}
}
