package progress

import (
	"bytes"
	"encoding/json"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
)

func TestProgressMessages(t *testing.T) {
	t.Run("writeProgress", func(t *testing.T) {
		var buf bytes.Buffer
		update := v1.Update{
			Total:    2 * 1024 * 1024,
			Complete: 1024 * 1024,
		}
		err := writeProgress(&buf, PullMsg(update), uint64(update.Total), uint64(update.Complete))
		if err != nil {
			t.Fatalf("Failed to write progress message: %v", err)
		}

		var msg ProgressMessage
		if err := json.Unmarshal(buf.Bytes(), &msg); err != nil {
			t.Fatalf("Failed to parse JSON: %v", err)
		}

		if msg.Type != "progress" {
			t.Errorf("Expected type 'progress', got '%s'", msg.Type)
		}
		if msg.Message != "Downloaded: 1.00 MB" {
			t.Errorf("Expected message 'Downloaded: 1.00 MB', got '%s'", msg.Message)
		}
		if msg.Total != uint64(2*1024*1024) {
			t.Errorf("Expected total 2MB, got %d", msg.Total)
		}
		if msg.Pulled != uint64(1024*1024) {
			t.Errorf("Expected pulled 1MB, got %d", msg.Pulled)
		}
	})

	t.Run("writeSuccess", func(t *testing.T) {
		var buf bytes.Buffer
		err := WriteSuccess(&buf, "Model pulled successfully")
		if err != nil {
			t.Fatalf("Failed to write success message: %v", err)
		}

		var msg ProgressMessage
		if err := json.Unmarshal(buf.Bytes(), &msg); err != nil {
			t.Fatalf("Failed to parse JSON: %v", err)
		}

		if msg.Type != "success" {
			t.Errorf("Expected type 'success', got '%s'", msg.Type)
		}
		if msg.Message != "Model pulled successfully" {
			t.Errorf("Expected message 'Model pulled successfully', got '%s'", msg.Message)
		}
	})

	t.Run("writeError", func(t *testing.T) {
		var buf bytes.Buffer
		err := WriteError(&buf, "Error: something went wrong")
		if err != nil {
			t.Fatalf("Failed to write error message: %v", err)
		}

		var msg ProgressMessage
		if err := json.Unmarshal(buf.Bytes(), &msg); err != nil {
			t.Fatalf("Failed to parse JSON: %v", err)
		}

		if msg.Type != "error" {
			t.Errorf("Expected type 'error', got '%s'", msg.Type)
		}
		if msg.Message != "Error: something went wrong" {
			t.Errorf("Expected message 'Error: something went wrong', got '%s'", msg.Message)
		}
	})
}
