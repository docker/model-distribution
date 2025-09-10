package bufferfile

import (
	"bytes"
	"io"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestFIFO_BasicReadWrite(t *testing.T) {
	fifo, err := NewFIFO()
	if err != nil {
		t.Fatalf("Failed to create FIFO: %v", err)
	}
	defer fifo.Close()

	// Test basic write
	data := []byte("hello world")
	n, err := fifo.Write(data)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != len(data) {
		t.Fatalf("Expected to write %d bytes, wrote %d", len(data), n)
	}

	// Test basic read
	buf := make([]byte, len(data))
	n, err = fifo.Read(buf)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if n != len(data) {
		t.Fatalf("Expected to read %d bytes, read %d", len(data), n)
	}
	if !bytes.Equal(buf, data) {
		t.Fatalf("Read data doesn't match written data: got %q, want %q", buf, data)
	}
}

func TestFIFO_MultipleWrites(t *testing.T) {
	fifo, err := NewFIFO()
	if err != nil {
		t.Fatalf("Failed to create FIFO: %v", err)
	}
	defer fifo.Close()

	// Write multiple chunks
	chunks := [][]byte{
		[]byte("chunk1"),
		[]byte("chunk2"),
		[]byte("chunk3"),
	}

	for i, chunk := range chunks {
		n, err := fifo.Write(chunk)
		if err != nil {
			t.Fatalf("Write %d failed: %v", i, err)
		}
		if n != len(chunk) {
			t.Fatalf("Write %d: expected %d bytes, wrote %d", i, len(chunk), n)
		}
	}

	// Read all data back
	expected := bytes.Join(chunks, nil)
	buf := make([]byte, len(expected))
	totalRead := 0

	for totalRead < len(expected) {
		n, err := fifo.Read(buf[totalRead:])
		if err != nil {
			t.Fatalf("Read failed: %v", err)
		}
		totalRead += n
	}

	if !bytes.Equal(buf, expected) {
		t.Fatalf("Read data doesn't match expected: got %q, want %q", buf, expected)
	}
}

func TestFIFO_PartialReads(t *testing.T) {
	fifo, err := NewFIFO()
	if err != nil {
		t.Fatalf("Failed to create FIFO: %v", err)
	}
	defer fifo.Close()

	// Write data
	data := []byte("0123456789")
	_, err = fifo.Write(data)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Read in small chunks
	buf := make([]byte, 3) // Smaller than data
	var result []byte

	for len(result) < len(data) {
		n, err := fifo.Read(buf)
		if err != nil {
			t.Fatalf("Read failed: %v", err)
		}
		result = append(result, buf[:n]...)
	}

	if !bytes.Equal(result, data) {
		t.Fatalf("Partial read result doesn't match: got %q, want %q", result, data)
	}
}

func TestFIFO_ConcurrentReadWrite(t *testing.T) {
	fifo, err := NewFIFO()
	if err != nil {
		t.Fatalf("Failed to create FIFO: %v", err)
	}
	defer fifo.Close()

	const numWriters = 3
	const numChunksPerWriter = 100
	const chunkSize = 100

	var wg sync.WaitGroup
	var writeOrder []int
	var writeOrderMu sync.Mutex

	// Start multiple writers
	for writerID := 0; writerID < numWriters; writerID++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < numChunksPerWriter; i++ {
				// Create unique data for this writer and chunk
				data := make([]byte, chunkSize)
				for j := range data {
					data[j] = byte((id*1000 + i) % 256)
				}

				writeOrderMu.Lock()
				writeOrder = append(writeOrder, id*1000+i)
				writeOrderMu.Unlock()

				_, err := fifo.Write(data)
				if err != nil {
					t.Errorf("Writer %d chunk %d failed: %v", id, i, err)
					return
				}
			}
		}(writerID)
	}

	// Read all data
	var readData []byte
	totalExpected := numWriters * numChunksPerWriter * chunkSize
	buf := make([]byte, 1024) // Read buffer

	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		for len(readData) < totalExpected {
			n, err := fifo.Read(buf)
			if err != nil {
				t.Errorf("Read failed: %v", err)
				return
			}
			readData = append(readData, buf[:n]...)
		}
	}()

	// Wait for all writes to complete
	wg.Wait()

	// Wait for all reads to complete
	select {
	case <-readDone:
		// Success
	case <-time.After(5 * time.Second):
		t.Fatal("Read timed out")
	}

	if len(readData) != totalExpected {
		t.Fatalf("Expected to read %d bytes, got %d", totalExpected, len(readData))
	}

	t.Logf("Successfully handled %d concurrent writers writing %d total bytes",
		numWriters, totalExpected)
}

func TestFIFO_ReadBlocksUntilData(t *testing.T) {
	fifo, err := NewFIFO()
	if err != nil {
		t.Fatalf("Failed to create FIFO: %v", err)
	}
	defer fifo.Close()

	buf := make([]byte, 10)
	readDone := make(chan struct{})
	var readErr error

	// Start a reader that should block
	go func() {
		defer close(readDone)
		_, readErr = fifo.Read(buf)
	}()

	// Ensure reader is blocked
	select {
	case <-readDone:
		t.Fatal("Read should have blocked")
	case <-time.After(100 * time.Millisecond):
		// Good, read is blocked
	}

	// Write data to unblock reader
	data := []byte("test")
	_, err = fifo.Write(data)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Now read should complete
	select {
	case <-readDone:
		if readErr != nil {
			t.Fatalf("Read failed: %v", readErr)
		}
	case <-time.After(time.Second):
		t.Fatal("Read did not complete after write")
	}
}

func TestFIFO_CloseInterruptsRead(t *testing.T) {
	fifo, err := NewFIFO()
	if err != nil {
		t.Fatalf("Failed to create FIFO: %v", err)
	}

	buf := make([]byte, 10)
	readDone := make(chan struct{})
	var readN int
	var readErr error

	// Start a reader that should block
	go func() {
		defer close(readDone)
		readN, readErr = fifo.Read(buf)
	}()

	// Ensure reader is blocked
	select {
	case <-readDone:
		t.Fatal("Read should have blocked")
	case <-time.After(100 * time.Millisecond):
		// Good, read is blocked
	}

	// Close FIFO to interrupt read
	err = fifo.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Read should complete with EOF
	select {
	case <-readDone:
		if readErr != io.EOF {
			t.Fatalf("Expected EOF after close, got: %v", readErr)
		}
		if readN != 0 {
			t.Fatalf("Expected 0 bytes read after close, got %d", readN)
		}
	case <-time.After(time.Second):
		t.Fatal("Read did not complete after close")
	}
}

func TestFIFO_CloseWithPendingData(t *testing.T) {
	fifo, err := NewFIFO()
	if err != nil {
		t.Fatalf("Failed to create FIFO: %v", err)
	}

	// Write some data
	data := []byte("pending data")
	_, err = fifo.Write(data)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Close FIFO
	err = fifo.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// After close, reads should return EOF immediately (data is lost)
	buf := make([]byte, len(data))
	n, err := fifo.Read(buf)
	if err != io.EOF {
		t.Fatalf("Expected EOF after close, got: %v", err)
	}
	if n != 0 {
		t.Fatalf("Expected 0 bytes read after close, got %d", n)
	}
}

func TestFIFO_WriteAfterClose(t *testing.T) {
	fifo, err := NewFIFO()
	if err != nil {
		t.Fatalf("Failed to create FIFO: %v", err)
	}

	err = fifo.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Write after close should fail
	_, err = fifo.Write([]byte("test"))
	if err == nil {
		t.Fatal("Expected write after close to fail")
	}
}

func TestFIFO_Stat(t *testing.T) {
	fifo, err := NewFIFO()
	if err != nil {
		t.Fatalf("Failed to create FIFO: %v", err)
	}
	defer fifo.Close()

	// Check initial state
	readPos, writePos, closed := fifo.Stat()
	if readPos != 0 || writePos != 0 || closed {
		t.Fatalf("Initial state wrong: readPos=%d, writePos=%d, closed=%v", readPos, writePos, closed)
	}

	// Write some data
	data := []byte("test data")
	_, err = fifo.Write(data)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	readPos, writePos, closed = fifo.Stat()
	if readPos != 0 || writePos != int64(len(data)) || closed {
		t.Fatalf("After write state wrong: readPos=%d, writePos=%d, closed=%v", readPos, writePos, closed)
	}

	// Read some data
	buf := make([]byte, 4)
	n, err := fifo.Read(buf)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	readPos, writePos, closed = fifo.Stat()
	if readPos != int64(n) || writePos != int64(len(data)) || closed {
		t.Fatalf("After read state wrong: readPos=%d, writePos=%d, closed=%v", readPos, writePos, closed)
	}

	// Close and check
	fifo.Close()
	readPos, writePos, closed = fifo.Stat()
	if !closed {
		t.Fatal("FIFO should be marked as closed")
	}
}

func TestFIFO_StressTest(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	fifo, err := NewFIFO()
	if err != nil {
		t.Fatalf("Failed to create FIFO: %v", err)
	}
	defer fifo.Close()

	const duration = 2 * time.Second
	const maxWriteSize = 1024
	const maxReadSize = 512

	var totalWritten int64
	var totalRead int64
	var wg sync.WaitGroup

	// Start writer goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		start := time.Now()
		for time.Since(start) < duration {
			size := rand.Intn(maxWriteSize) + 1
			data := make([]byte, size)
			rand.Read(data)

			n, err := fifo.Write(data)
			if err != nil {
				t.Errorf("Write failed: %v", err)
				return
			}
			atomic.AddInt64(&totalWritten, int64(n))
		}
	}()

	// Start reader goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, maxReadSize)
		start := time.Now()

		for time.Since(start) < duration+time.Second { // Give extra time to read
			n, err := fifo.Read(buf)
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Errorf("Read failed: %v", err)
				return
			}
			atomic.AddInt64(&totalRead, int64(n))

			// If we've read everything written and writer is done, we're done
			if atomic.LoadInt64(&totalRead) >= atomic.LoadInt64(&totalWritten) && time.Since(start) > duration {
				break
			}
		}
	}()

	wg.Wait()

	finalWritten := atomic.LoadInt64(&totalWritten)
	finalRead := atomic.LoadInt64(&totalRead)
	t.Logf("Stress test completed: wrote %d bytes, read %d bytes", finalWritten, finalRead)

	if finalRead > finalWritten {
		t.Fatalf("Read more than written: read=%d, written=%d", finalRead, finalWritten)
	}
}

func TestFIFO_EmptyOperations(t *testing.T) {
	fifo, err := NewFIFO()
	if err != nil {
		t.Fatalf("Failed to create FIFO: %v", err)
	}
	defer fifo.Close()

	// Test empty write
	n, err := fifo.Write(nil)
	if err != nil {
		t.Fatalf("Empty write failed: %v", err)
	}
	if n != 0 {
		t.Fatalf("Expected 0 bytes written for empty write, got %d", n)
	}

	// Test empty read
	n, err = fifo.Read(nil)
	if err != nil {
		t.Fatalf("Empty read failed: %v", err)
	}
	if n != 0 {
		t.Fatalf("Expected 0 bytes read for empty read, got %d", n)
	}
}

func TestFIFO_MultipleClose(t *testing.T) {
	fifo, err := NewFIFO()
	if err != nil {
		t.Fatalf("Failed to create FIFO: %v", err)
	}

	// First close should succeed
	err = fifo.Close()
	if err != nil {
		t.Fatalf("First close failed: %v", err)
	}

	// Second close should not panic and should not error
	err = fifo.Close()
	if err != nil {
		t.Fatalf("Second close failed: %v", err)
	}
}

// Benchmark tests
func BenchmarkFIFO_Write(b *testing.B) {
	fifo, err := NewFIFO()
	if err != nil {
		b.Fatalf("Failed to create FIFO: %v", err)
	}
	defer fifo.Close()

	data := make([]byte, 1024)
	rand.Read(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := fifo.Write(data)
		if err != nil {
			b.Fatalf("Write failed: %v", err)
		}
	}
}

func BenchmarkFIFO_Read(b *testing.B) {
	fifo, err := NewFIFO()
	if err != nil {
		b.Fatalf("Failed to create FIFO: %v", err)
	}
	defer fifo.Close()

	// Pre-fill with data
	data := make([]byte, 1024)
	rand.Read(data)
	for i := 0; i < b.N; i++ {
		fifo.Write(data)
	}

	buf := make([]byte, 1024)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := fifo.Read(buf)
		if err != nil {
			b.Fatalf("Read failed: %v", err)
		}
	}
}
