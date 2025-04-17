package distribution

import (
	"bytes"
	"encoding/json"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
)

func TestProgressMessages(t *testing.T) {
	t.Run("writeProgress", func(t *testing.T) {
		var buf bytes.Buffer
		err := writeProgress(&buf, pullMsg(v1.Update{
			Complete: 1024 * 1024,
		}))
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
	})

	t.Run("writeSuccess", func(t *testing.T) {
		var buf bytes.Buffer
		err := writeSuccess(&buf, "Model pulled successfully")
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
		err := writeError(&buf, "Error: something went wrong")
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
