// Package bufferfile provides a FIFO implementation backed by a temporary file
// that supports concurrent reads and writes.
package bufferfile

import (
	"fmt"
	"io"
	"os"
	"sync"
)

// FIFO is an io.ReadWriteCloser implementation that supports concurrent
// reads and writes to a temporary file. Reads begin from the start of the file
// and writes always append to the end. The type maintains separate read and write
// positions internally.
type FIFO struct {
	file        *os.File
	mu          sync.Mutex
	cond        *sync.Cond // Condition variable for signaling data availability
	readPos     int64      // Current read position in the file
	writePos    int64      // Current write position in the file (always at end)
	closed      bool
	writeClosed bool  // True when no more writes will happen
	writeErr    error // Last write error (persistent)
}

// NewFIFO creates a new FIFO backed by a temporary file.
// The caller is responsible for calling Close() to clean up the temporary file.
func NewFIFO() (*FIFO, error) {
	file, err := os.CreateTemp("", "fifo-*.tmp")
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary file: %w", err)
	}

	fifo := &FIFO{
		file:     file,
		readPos:  0,
		writePos: 0,
		closed:   false,
	}
	fifo.cond = sync.NewCond(&fifo.mu)

	return fifo, nil
}

// Write implements io.Writer. Writes always append to the end of the file.
// Write is safe for concurrent use with Read.
func (f *FIFO) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	if f.closed || f.writeClosed {
		return 0, fmt.Errorf("write to closed FIFO")
	}

	// Return persistent write error if we have one
	if f.writeErr != nil {
		return 0, f.writeErr
	}

	// Seek to current write position (end of file)
	_, err := f.file.Seek(f.writePos, io.SeekStart)
	if err != nil {
		f.writeErr = fmt.Errorf("seek to write position failed: %w", err)
		return 0, f.writeErr
	}

	// Write the data
	n, err := f.file.Write(p)
	if n > 0 {
		f.writePos += int64(n)
		// Signal waiting readers that data is available
		f.cond.Broadcast()
	}

	if err != nil {
		f.writeErr = fmt.Errorf("write failed: %w", err)
		return n, f.writeErr
	}

	return n, nil
}

// Read implements io.Reader. Reads from the current read position in the file.
// Read blocks until data is available or the FIFO is closed.
// Read is safe for concurrent use with Write.
func (f *FIFO) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	for {
		if f.closed {
			// If closed, we can't read from file anymore since it's closed
			// Return EOF immediately
			return 0, io.EOF
		}

		// Check if there's data available to read
		availableBytes := f.writePos - f.readPos
		if availableBytes > 0 {
			return f.readFromFile(p)
		}

		// No data available - check if write side is closed
		if f.writeClosed {
			// No more data will be written and no data available - return EOF
			return 0, io.EOF
		}

		// No data available and not closed - wait for signal
		f.cond.Wait()
	}
}

// readFromFile performs the actual file read operation.
// Must be called with mutex held.
func (f *FIFO) readFromFile(p []byte) (int, error) {
	availableBytes := f.writePos - f.readPos
	toRead := int64(len(p))
	if toRead > availableBytes {
		toRead = availableBytes
	}

	// Seek to current read position
	_, err := f.file.Seek(f.readPos, io.SeekStart)
	if err != nil {
		return 0, fmt.Errorf("seek to read position failed: %w", err)
	}

	// Read the data
	n, err := f.file.Read(p[:toRead])
	if n > 0 {
		f.readPos += int64(n)
	}

	if err != nil && err != io.EOF {
		return n, fmt.Errorf("read failed: %w", err)
	}

	return n, nil
}

// Close closes the FIFO and removes the temporary file.
// Any blocked Read or Write operations will be interrupted.
// Close is safe to call multiple times.
func (f *FIFO) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.closed {
		return nil
	}

	f.closed = true

	// Wake up all waiting readers
	f.cond.Broadcast()

	var err error
	if f.file != nil {
		// Get the file name before closing for cleanup
		fileName := f.file.Name()

		// Close the file (this will interrupt any blocked I/O operations)
		if closeErr := f.file.Close(); closeErr != nil {
			err = fmt.Errorf("failed to close file: %w", closeErr)
		}

		// Remove the temporary file
		if removeErr := os.Remove(fileName); removeErr != nil {
			if err != nil {
				err = fmt.Errorf("%w; also failed to remove temp file: %v", err, removeErr)
			} else {
				err = fmt.Errorf("failed to remove temp file: %w", removeErr)
			}
		}

		f.file = nil
	}

	return err
}

// CloseWrite signals that no more writes will happen.
// Readers can still read remaining data, and will receive EOF when all data is consumed.
// Does not clean up resources - use Close() for that.
func (f *FIFO) CloseWrite() {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.writeClosed = true

	// Wake up all waiting readers to check the new state
	f.cond.Broadcast()
}

// Stat returns information about the current state of the FIFO.
func (f *FIFO) Stat() (readPos, writePos int64, closed bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.readPos, f.writePos, f.closed
}
