package parallel

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"hash"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

// deterministicDataGenerator generates deterministic data based on position
// This allows us to generate GB-sized data streams without storing them in memory
type deterministicDataGenerator struct {
	position int64
	size     int64
}

func newDeterministicDataGenerator(size int64) *deterministicDataGenerator {
	return &deterministicDataGenerator{
		position: 0,
		size:     size,
	}
}

func (g *deterministicDataGenerator) Read(p []byte) (int, error) {
	if g.position >= g.size {
		return 0, io.EOF
	}

	// Calculate how much we can read
	remaining := g.size - g.position
	toRead := int64(len(p))
	if toRead > remaining {
		toRead = remaining
	}

	// Generate deterministic data based on position
	for i := int64(0); i < toRead; i++ {
		pos := g.position + i
		// Use a simple but deterministic pattern: position mod 256
		// XOR with some constants to make it more interesting
		p[i] = byte((pos ^ (pos >> 8) ^ (pos >> 16)) % 256)
	}

	g.position += toRead
	return int(toRead), nil
}

// createLargeFileServer creates an HTTP server that serves deterministic large files
func createLargeFileServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract size from URL path
		sizeStr := r.URL.Path[len("/data/"):]
		size, err := strconv.ParseInt(sizeStr, 10, 64)
		if err != nil || size <= 0 {
			http.Error(w, "Invalid size", http.StatusBadRequest)
			return
		}

		// Support range requests
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("ETag", fmt.Sprintf(`"test-file-%d"`, size))

		// Handle range requests
		rangeHeader := r.Header.Get("Range")
		if rangeHeader != "" {
			var start, end int64
			if n, err := fmt.Sscanf(rangeHeader, "bytes=%d-%d", &start, &end); n == 2 && err == nil {
				if start >= 0 && end < size && start <= end {
					rangeSize := end - start + 1
					w.Header().Set("Content-Length", strconv.FormatInt(rangeSize, 10))
					w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, size))
					w.WriteHeader(http.StatusPartialContent)

					// Create generator positioned at start
					gen := newDeterministicDataGenerator(size)
					gen.position = start

					// Copy the requested range
					_, err := io.CopyN(w, gen, rangeSize)
					if err != nil && err != io.EOF {
						http.Error(w, err.Error(), http.StatusInternalServerError)
					}
					return
				}
			}
		}

		// Full file request
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
		gen := newDeterministicDataGenerator(size)
		_, err = io.Copy(w, gen)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}))
}

// hashingReader wraps an io.Reader and computes SHA-256 while reading
type hashingReader struct {
	reader    io.Reader
	hasher    hash.Hash
	bytesRead int64
}

func newHashingReader(r io.Reader) *hashingReader {
	return &hashingReader{
		reader:    r,
		hasher:    sha256.New(),
		bytesRead: 0,
	}
}

func (hr *hashingReader) Read(p []byte) (int, error) {
	n, err := hr.reader.Read(p)
	if n > 0 {
		hr.hasher.Write(p[:n])
		hr.bytesRead += int64(n)
	}
	return n, err
}

func (hr *hashingReader) Sum() []byte {
	return hr.hasher.Sum(nil)
}

func (hr *hashingReader) BytesRead() int64 {
	return hr.bytesRead
}

// computeExpectedHash computes the expected SHA-256 hash for a file of given size
func computeExpectedHash(size int64) []byte {
	hasher := sha256.New()
	gen := newDeterministicDataGenerator(size)
	io.Copy(hasher, gen)
	return hasher.Sum(nil)
}

func TestLargeFile_1GB_ParallelVsSequential(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large file test in short mode")
	}

	server := createLargeFileServer()
	defer server.Close()

	// Test with 1GB file
	size := int64(1024 * 1024 * 1024) // 1 GB
	url := server.URL + "/data/" + strconv.FormatInt(size, 10)

	// Compute expected hash
	expectedHash := computeExpectedHash(size)

	t.Run("Sequential", func(t *testing.T) {
		client := &http.Client{Transport: http.DefaultTransport}

		resp, err := client.Get(url)
		if err != nil {
			t.Fatalf("Failed to get %s: %v", url, err)
		}
		defer resp.Body.Close()

		if resp.ContentLength != size {
			t.Errorf("Expected Content-Length %d, got %d", size, resp.ContentLength)
		}

		hashingReader := newHashingReader(resp.Body)
		_, err = io.Copy(io.Discard, hashingReader)
		if err != nil {
			t.Fatalf("Failed to read response body: %v", err)
		}

		if hashingReader.BytesRead() != size {
			t.Errorf("Expected to read %d bytes, actually read %d bytes", size, hashingReader.BytesRead())
		}

		actualHash := hashingReader.Sum()
		if !bytes.Equal(expectedHash, actualHash) {
			t.Errorf("Hash mismatch.\nExpected: %x\nActual:   %x", expectedHash, actualHash)
		}
	})

	t.Run("Parallel", func(t *testing.T) {
		transport := New(
			http.DefaultTransport,
			WithMaxConcurrentPerHost(map[string]uint{"": 0}),
			WithMinChunkSize(4*1024*1024), // 4MB chunks
			WithMaxConcurrentPerRequest(8),
		)
		client := &http.Client{Transport: transport}

		resp, err := client.Get(url)
		if err != nil {
			t.Fatalf("Failed to get %s: %v", url, err)
		}
		defer resp.Body.Close()

		if resp.ContentLength != size {
			t.Errorf("Expected Content-Length %d, got %d", size, resp.ContentLength)
		}

		hashingReader := newHashingReader(resp.Body)
		_, err = io.Copy(io.Discard, hashingReader)
		if err != nil {
			t.Fatalf("Failed to read response body: %v", err)
		}

		if hashingReader.BytesRead() != size {
			t.Errorf("Expected to read %d bytes, actually read %d bytes", size, hashingReader.BytesRead())
		}

		actualHash := hashingReader.Sum()
		if !bytes.Equal(expectedHash, actualHash) {
			t.Errorf("Hash mismatch.\nExpected: %x\nActual:   %x", expectedHash, actualHash)
		}
	})
}

func TestLargeFile_4GB_ParallelDownload(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large file test in short mode")
	}

	server := createLargeFileServer()
	defer server.Close()

	// Test with 4GB file
	size := int64(4 * 1024 * 1024 * 1024) // 4 GB
	url := server.URL + "/data/" + strconv.FormatInt(size, 10)

	// Only test parallel for 4GB due to time constraints
	transport := New(
		http.DefaultTransport,
		WithMaxConcurrentPerHost(map[string]uint{"": 0}),
		WithMinChunkSize(8*1024*1024), // 8MB chunks
		WithMaxConcurrentPerRequest(16),
	)
	client := &http.Client{Transport: transport}

	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("Failed to get %s: %v", url, err)
	}
	defer resp.Body.Close()

	if resp.ContentLength != size {
		t.Errorf("Expected Content-Length %d, got %d", size, resp.ContentLength)
	}

	// For 4GB, let's just verify we can read the correct number of bytes
	// Computing the full hash would take too long
	bytesRead := int64(0)
	buf := make([]byte, 64*1024) // 64KB buffer
	for {
		n, err := resp.Body.Read(buf)
		bytesRead += int64(n)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Failed to read response body: %v", err)
		}
	}

	if bytesRead != size {
		t.Errorf("Expected to read %d bytes, actually read %d bytes", size, bytesRead)
	}

	t.Logf("Successfully read %d bytes (4GB) from parallel download", bytesRead)
}
