package distribution

import (
	"encoding/json"
	"fmt"
	"io"
)

// ProgressMessage represents a structured message for progress reporting
type ProgressMessage struct {
	Type    string `json:"type"`    // "progress", "success", or "error"
	Message string `json:"message"` // Human-readable message
}

// writeProgressMessage writes a JSON-formatted progress message to the writer
func writeProgressMessage(w io.Writer, msg ProgressMessage) error {
	if w == nil {
		return nil
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "%s\n", data)
	return err
}

// writeProgress writes a progress update message
func writeProgress(w io.Writer, complete int64) error {
	if complete == 0 {
		return nil
	}
	return writeProgressMessage(w, ProgressMessage{
		Type:    "progress",
		Message: fmt.Sprintf("Downloaded: %.2f MB", float64(complete)/1024/1024),
	})
}

// writeSuccess writes a success message
func writeSuccess(w io.Writer, message string) error {
	return writeProgressMessage(w, ProgressMessage{
		Type:    "success",
		Message: message,
	})
}

// writeError writes an error message
func writeError(w io.Writer, message string) error {
	return writeProgressMessage(w, ProgressMessage{
		Type:    "error",
		Message: message,
	})
}
