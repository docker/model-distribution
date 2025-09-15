package testing

import (
	"errors"
	"io"
	"sync"
)

// ErrFlakyFailure is returned when FlakyReader simulates a failure.
var ErrFlakyFailure = errors.New("simulated read failure")

// FlakyReader simulates a reader that fails after a certain number of bytes.
type FlakyReader struct {
	data      []byte
	failAfter int
	pos       int
	failed    bool
	closed    bool
	mu        sync.Mutex
}

// NewFlakyReader creates a FlakyReader that fails after reading failAfter bytes.
// If failAfter is 0 or negative, it never fails.
func NewFlakyReader(data []byte, failAfter int) *FlakyReader {
	return &FlakyReader{
		data:      data,
		failAfter: failAfter,
	}
}

// Read implements io.Reader.
func (fr *FlakyReader) Read(p []byte) (int, error) {
	fr.mu.Lock()
	defer fr.mu.Unlock()

	if fr.closed {
		return 0, errors.New("read from closed reader")
	}

	if fr.failed {
		return 0, ErrFlakyFailure
	}

	if fr.pos >= len(fr.data) {
		return 0, io.EOF
	}

	// Calculate how much we can read
	remaining := len(fr.data) - fr.pos
	toRead := len(p)
	if toRead > remaining {
		toRead = remaining
	}

	// Check if we should fail
	if fr.failAfter > 0 && fr.pos+toRead > fr.failAfter {
		// Read up to failure point
		toRead = fr.failAfter - fr.pos
		if toRead <= 0 {
			fr.failed = true
			return 0, ErrFlakyFailure
		}
	}

	// Copy data
	n := copy(p, fr.data[fr.pos:fr.pos+toRead])
	fr.pos += n

	// Check if we've hit the failure point
	if fr.failAfter > 0 && fr.pos >= fr.failAfter && fr.pos < len(fr.data) {
		fr.failed = true
		if n == 0 {
			return 0, ErrFlakyFailure
		}
		// Return the data we read, error will come on next read
	}

	if fr.pos >= len(fr.data) {
		return n, io.EOF
	}

	return n, nil
}

// Close implements io.Closer.
func (fr *FlakyReader) Close() error {
	fr.mu.Lock()
	defer fr.mu.Unlock()
	fr.closed = true
	return nil
}

// Reset resets the reader to start from the beginning.
func (fr *FlakyReader) Reset() {
	fr.mu.Lock()
	defer fr.mu.Unlock()
	fr.pos = 0
	fr.failed = false
	fr.closed = false
}

// Position returns the current read position.
func (fr *FlakyReader) Position() int {
	fr.mu.Lock()
	defer fr.mu.Unlock()
	return fr.pos
}

// HasFailed returns true if the reader has simulated a failure.
func (fr *FlakyReader) HasFailed() bool {
	fr.mu.Lock()
	defer fr.mu.Unlock()
	return fr.failed
}

// MultiFailReader simulates multiple failures at different points.
type MultiFailReader struct {
	data         []byte
	failurePoints []int
	failureCount int
	pos          int
	closed       bool
	mu           sync.Mutex
}

// NewMultiFailReader creates a reader that fails at specified byte positions.
func NewMultiFailReader(data []byte, failurePoints []int) *MultiFailReader {
	return &MultiFailReader{
		data:          data,
		failurePoints: failurePoints,
	}
}

// Read implements io.Reader.
func (mfr *MultiFailReader) Read(p []byte) (int, error) {
	mfr.mu.Lock()
	defer mfr.mu.Unlock()

	if mfr.closed {
		return 0, errors.New("read from closed reader")
	}

	if mfr.pos >= len(mfr.data) {
		return 0, io.EOF
	}

	// Check if we're at a failure point
	for i, point := range mfr.failurePoints {
		if i < mfr.failureCount {
			continue // Already failed here
		}
		if mfr.pos == point {
			mfr.failureCount++
			return 0, ErrFlakyFailure
		}
	}

	// Calculate how much to read
	remaining := len(mfr.data) - mfr.pos
	toRead := len(p)
	if toRead > remaining {
		toRead = remaining
	}

	// Check if we would cross a failure point
	for i, point := range mfr.failurePoints {
		if i < mfr.failureCount {
			continue
		}
		if mfr.pos < point && mfr.pos+toRead > point {
			toRead = point - mfr.pos
			break
		}
	}

	// Copy data
	n := copy(p, mfr.data[mfr.pos:mfr.pos+toRead])
	mfr.pos += n

	if mfr.pos >= len(mfr.data) {
		return n, io.EOF
	}

	return n, nil
}

// Close implements io.Closer.
func (mfr *MultiFailReader) Close() error {
	mfr.mu.Lock()
	defer mfr.mu.Unlock()
	mfr.closed = true
	return nil
}

// Reset resets the reader to start from the beginning.
func (mfr *MultiFailReader) Reset() {
	mfr.mu.Lock()
	defer mfr.mu.Unlock()
	mfr.pos = 0
	mfr.failureCount = 0
	mfr.closed = false
}