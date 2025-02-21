package utils

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// FormatBytes converts bytes to a human-readable string with appropriate unit
func FormatBytes(bytes int) string {
	size := float64(bytes)
	var unit string
	switch {
	case size >= 1<<30:
		size /= 1 << 30
		unit = "GB"
	case size >= 1<<20:
		size /= 1 << 20
		unit = "MB"
	case size >= 1<<10:
		size /= 1 << 10
		unit = "KB"
	default:
		unit = "bytes"
	}
	return fmt.Sprintf("%.2f %s", size, unit)
}

// ShowProgress displays a progress bar for data transfer operations
func ShowProgress(operation string, progressChan chan int64, totalSize int64) {
	for bytesComplete := range progressChan {
		if totalSize > 0 {
			mbComplete := float64(bytesComplete) / (1024 * 1024)
			mbTotal := float64(totalSize) / (1024 * 1024)
			fmt.Printf("\r%s: %.2f MB / %.2f MB", operation, mbComplete, mbTotal)
		} else {
			mb := float64(bytesComplete) / (1024 * 1024)
			fmt.Printf("\r%s: %.2f MB", operation, mb)
		}
	}
	fmt.Println() // Move to new line after progress
}

// ReadContent reads content from a local file or URL and returns an io.ReadCloser
func ReadContent(source string) (io.ReadCloser, error) {
	// Check if the source is a URL
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		// Parse the URL
		_, err := url.Parse(source)
		if err != nil {
			return nil, fmt.Errorf("invalid URL: %v", err)
		}

		// Make HTTP request
		resp, err := http.Get(source)
		if err != nil {
			return nil, fmt.Errorf("failed to download file: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("failed to download file: HTTP status %d", resp.StatusCode)
		}

		// Create progress reader
		contentLength := resp.ContentLength
		progressChan := make(chan int64, 100)

		// Start progress reporting goroutine
		go ShowProgress("Downloading", progressChan, contentLength)

		// Return a wrapped reader that tracks progress
		return &ProgressReadCloser{
			ReadCloser:   resp.Body,
			ProgressChan: progressChan,
		}, nil
	}

	// If not a URL, treat as local file path
	file, err := os.Open(source)
	if err != nil {
		return nil, err
	}

	return file, nil
}

// ProgressReadCloser wraps an io.ReadCloser to track reading progress
type ProgressReadCloser struct {
	ReadCloser   io.ReadCloser
	ProgressChan chan int64
	Total        int64
}

func (pr *ProgressReadCloser) Read(p []byte) (int, error) {
	n, err := pr.ReadCloser.Read(p)
	if n > 0 {
		pr.Total += int64(n)
		pr.ProgressChan <- pr.Total
	}
	if err == io.EOF {
		close(pr.ProgressChan)
	}
	return n, err
}

func (pr *ProgressReadCloser) Close() error {
	return pr.ReadCloser.Close()
}
