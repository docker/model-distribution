package progress

import (
	"bytes"
	"encoding/json"
	"io"
	"testing"
	"time"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	v1types "github.com/google/go-containerregistry/pkg/v1/types"

	"github.com/docker/model-distribution/types"
)

// mockLayer implements v1.Layer for testing
type mockLayer struct {
	size      int64
	diffID    string
	mediaType v1types.MediaType
}

func (m *mockLayer) Digest() (v1.Hash, error) {
	return v1.Hash{}, nil
}

func (m *mockLayer) DiffID() (v1.Hash, error) {
	return v1.NewHash(m.diffID)
}

func (m *mockLayer) Compressed() (io.ReadCloser, error) {
	return nil, nil
}

func (m *mockLayer) Uncompressed() (io.ReadCloser, error) {
	return nil, nil
}

func (m *mockLayer) Size() (int64, error) {
	return m.size, nil
}

func (m *mockLayer) MediaType() (v1types.MediaType, error) {
	return m.mediaType, nil
}

func newMockLayer(size int64) *mockLayer {
	return &mockLayer{
		size:      size,
		diffID:    "sha256:c7790a0a70161f1bfd441cf157313e9efb8fcd1f0831193101def035ead23b32",
		mediaType: types.MediaTypeGGUF,
	}
}

func TestMessages(t *testing.T) {
	t.Run("writeProgress", func(t *testing.T) {
		var buf bytes.Buffer
		update := v1.Update{
			Complete: 1024 * 1024,
		}
		layer := newMockLayer(2016)
		size := layer.size

		err := WriteProgress(&buf, PullMsg(update), uint64(size), uint64(update.Complete), layer.diffID)
		if err != nil {
			t.Fatalf("Failed to write progress message: %v", err)
		}

		var msg Message
		if err := json.Unmarshal(buf.Bytes(), &msg); err != nil {
			t.Fatalf("Failed to parse JSON: %v", err)
		}

		if msg.Type != "progress" {
			t.Errorf("Expected type 'progress', got '%s'", msg.Type)
		}
		if msg.Message != "Downloaded: 1.00 MB" {
			t.Errorf("Expected message 'Downloaded: 1.00 MB', got '%s'", msg.Message)
		}
		if msg.Pulled != uint64(1024*1024) {
			t.Errorf("Expected pulled 1MB, got %d", msg.Pulled)
		}
		if msg.Layer == (Layer{}) {
			t.Errorf("Expected layer to be set")
		}
		if msg.Layer.ID != "sha256:c7790a0a70161f1bfd441cf157313e9efb8fcd1f0831193101def035ead23b32" {
			t.Errorf("Expected layer ID to be %s, got %s", "sha256:c7790a0a70161f1bfd441cf157313e9efb8fcd1f0831193101def035ead23b32", msg.Layer.ID)
		}
		if msg.Layer.Size != uint64(2016) {
			t.Errorf("Expected layer size to be %d, got %d", 2016, msg.Layer.Size)
		}
		if msg.Layer.Current != uint64(1048576) {
			t.Errorf("Expected layer current to be %d, got %d", 1048576, msg.Layer.Current)
		}
	})

	t.Run("writeSuccess", func(t *testing.T) {
		var buf bytes.Buffer
		err := WriteSuccess(&buf, "Model pulled successfully")
		if err != nil {
			t.Fatalf("Failed to write success message: %v", err)
		}

		var msg Message
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

		var msg Message
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

func TestProgressEmissionScenarios(t *testing.T) {
	tests := []struct {
		name          string
		updates       []v1.Update
		delays        []time.Duration
		expectedCount int
		description   string
		layerSize     int64
	}{
		{
			name: "time-based updates",
			updates: []v1.Update{
				{Complete: 100},  // First update always sent
				{Complete: 100},  // Sent after interval
				{Complete: 1000}, // Sent after interval
			},
			delays: []time.Duration{
				UpdateInterval + 100*time.Millisecond,
				UpdateInterval + 100*time.Millisecond,
			},
			expectedCount: 3, // First update + 2 time-based updates
			description:   "should emit updates based on time interval",
			layerSize:     100,
		},
		{
			name: "byte-based updates",
			updates: []v1.Update{
				{Complete: MinBytesForUpdate * 2}, // First update always sent
				{Complete: MinBytesForUpdate * 2},
			},
			delays: []time.Duration{
				10 * time.Millisecond, // Short delay, should trigger based on bytes
			},
			expectedCount: 2, // First update + 1 byte-based update
			description:   "should emit update based on byte threshold",
			layerSize:     MinBytesForUpdate + 1,
		},
		{
			name: "no updates - too frequent",
			updates: []v1.Update{
				{Complete: 100}, // First update always sent
				{Complete: 100}, // Too frequent, no update
				{Complete: 100}, // Too frequent, no update
			},
			delays: []time.Duration{
				10 * time.Millisecond, // Too short
				10 * time.Millisecond, // Too short
			},
			expectedCount: 1, // Only first update
			description:   "should not emit updates if too frequent",
			layerSize:     100,
		},
		{
			name: "no updates - too few bytes",
			updates: []v1.Update{
				{Complete: 50},                      // First update always sent
				{Complete: MinBytesForUpdate},       // Too few bytes
				{Complete: MinBytesForUpdate + 100}, // enough bytes now
			},
			delays: []time.Duration{
				10 * time.Millisecond,
			},
			expectedCount: 2, // First update and last update
			description:   "should emit updates based on time even with few bytes",
			layerSize:     100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			layer := newMockLayer(tt.layerSize)

			reporter := NewProgressReporter(&buf, PullMsg, layer)
			updates := reporter.Updates()

			// Send updates with delays
			for i, update := range tt.updates {
				updates <- update
				if i < len(tt.delays) {
					time.Sleep(tt.delays[i])
				}
			}
			close(updates)

			// Wait for processing to complete
			if err := reporter.Wait(); err != nil {
				t.Fatalf("Reporter.Wait() failed: %v", err)
			}

			// Parse messages
			lines := bytes.Split(buf.Bytes(), []byte("\n"))
			var messages []Message
			for _, line := range lines {
				if len(line) == 0 {
					continue
				}
				var msg Message
				if err := json.Unmarshal(line, &msg); err != nil {
					t.Fatalf("Failed to parse JSON: %v", err)
				}
				messages = append(messages, msg)
			}

			if len(messages) != tt.expectedCount {
				t.Errorf("%s: expected %d messages, got %d", tt.description, tt.expectedCount, len(messages))
			}

			// Verify message format for any messages received
			for i, msg := range messages {
				if msg.Type != "progress" {
					t.Errorf("message %d: expected type 'progress', got '%s'", i, msg.Type)
				}
				if msg.Layer.ID == "" {
					t.Errorf("message %d: expected layer ID to be set", i)
				}
				if msg.Layer.Size != uint64(tt.layerSize) {
					t.Errorf("message %d: expected layer size %d, got %d", i, tt.layerSize, msg.Layer.Size)
				}
			}
		})
	}
}

func TestChannelBufferBackpressure(t *testing.T) {
	// Helper function to create a reporter with specified buffer size
	createReporter := func(bufferSize int) *Reporter {
		var buf bytes.Buffer
		layer := newMockLayer(1000)
		reporter := &Reporter{
			out:      &buf,
			progress: make(chan v1.Update, bufferSize),
			done:     make(chan struct{}),
			format:   PullMsg,
			layer:    layer,
		}
		return reporter
	}

	// Helper function to measure time taken to send updates
	sendUpdates := func(reporter *Reporter, numUpdates int) time.Duration {
		updates := reporter.Updates()
		start := time.Now()

		// Send updates as fast as possible
		for i := 0; i < numUpdates; i++ {
			updates <- v1.Update{Complete: int64(i * 100)}
		}
		close(updates)

		// Wait for processing to complete
		reporter.Wait()
		return time.Since(start)
	}

	t.Run("unbuffered vs buffered channel", func(t *testing.T) {
		const numUpdates = 1000

		// Test with unbuffered channel
		unbufferedReporter := createReporter(0)
		unbufferedTime := sendUpdates(unbufferedReporter, numUpdates)

		// Test with buffered channel
		bufferedReporter := createReporter(1000)
		bufferedTime := sendUpdates(bufferedReporter, numUpdates)

		// Log the results
		t.Logf("Unbuffered channel time: %v", unbufferedTime)
		t.Logf("Buffered channel time: %v", bufferedTime)
		t.Logf("Speedup factor: %.2fx", float64(unbufferedTime)/float64(bufferedTime))

		// Verify that buffered channel is significantly faster
		if bufferedTime >= unbufferedTime {
			t.Errorf("Expected buffered channel to be faster than unbuffered channel")
		}
	})

	t.Run("buffer overflow scenario", func(t *testing.T) {
		const numUpdates = 200 // More updates than buffer size
		const bufferSize = 100

		// Create a reporter with a small buffer
		reporter := createReporter(bufferSize)
		updates := reporter.Updates()

		// Send updates and measure time
		start := time.Now()
		for i := 0; i < numUpdates; i++ {
			updates <- v1.Update{Complete: int64(i * 1000)}
		}
		close(updates)
		reporter.Wait()
		totalTime := time.Since(start)

		// Log the results
		t.Logf("Time to send %d updates with buffer size %d: %v", numUpdates, bufferSize, totalTime)
		t.Logf("Average time per update: %v", totalTime/time.Duration(numUpdates))

		// Verify that all updates were processed
		if err := reporter.Wait(); err != nil {
			t.Errorf("Reporter.Wait() failed: %v", err)
		}
	})
}
