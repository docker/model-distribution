package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/docker/model-distribution/transport/parallel"
)

var (
	minChunkSize  int64
	maxConcurrent uint
)

var rootCmd = &cobra.Command{
	Use:   "parallelget <url>",
	Short: "Benchmark parallel vs non-parallel HTTP GET requests",
	Long: `parallelget is a benchmarking tool that compares the performance of standard
HTTP GET requests against parallelized requests using the transport/parallel package.

It downloads the same URL twice - once using the standard HTTP client and once
using a parallel transport - then compares the results and reports performance metrics.`,
	Args: cobra.ExactArgs(1),
	RunE: runBenchmark,
}

func init() {
	rootCmd.Flags().Int64Var(&minChunkSize, "chunk-size", 1024*1024, "Minimum chunk size in bytes for parallelization (default 1MB)")
	rootCmd.Flags().UintVar(&maxConcurrent, "max-concurrent", 4, "Maximum concurrent requests for parallel transport (default 4)")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runBenchmark(cmd *cobra.Command, args []string) error {
	url := args[0]

	fmt.Printf("Benchmarking HTTP GET performance for: %s\n", url)
	fmt.Printf("Configuration: chunk-size=%d bytes, max-concurrent=%d\n\n", minChunkSize, maxConcurrent)

	// Create temporary files for storing responses
	nonParallelFile, err := os.CreateTemp("", "benchmark-non-parallel-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file for non-parallel response: %w", err)
	}
	defer os.Remove(nonParallelFile.Name())
	defer nonParallelFile.Close()

	parallelFile, err := os.CreateTemp("", "benchmark-parallel-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file for parallel response: %w", err)
	}
	defer os.Remove(parallelFile.Name())
	defer parallelFile.Close()

	// Run non-parallel benchmark
	fmt.Println("Running non-parallel benchmark...")
	nonParallelDuration, nonParallelSize, err := benchmarkNonParallel(url, nonParallelFile)
	if err != nil {
		return fmt.Errorf("non-parallel benchmark failed: %w", err)
	}
	fmt.Printf("✓ Non-parallel: %d bytes in %v (%.2f MB/s)\n", nonParallelSize, nonParallelDuration,
		float64(nonParallelSize)/nonParallelDuration.Seconds()/(1024*1024))

	// Run parallel benchmark
	fmt.Println("Running parallel benchmark...")
	parallelDuration, parallelSize, err := benchmarkParallel(url, parallelFile)
	if err != nil {
		return fmt.Errorf("parallel benchmark failed: %w", err)
	}
	fmt.Printf("✓ Parallel: %d bytes in %v (%.2f MB/s)\n", parallelSize, parallelDuration,
		float64(parallelSize)/parallelDuration.Seconds()/(1024*1024))

	// Validate responses match
	fmt.Println("Validating response consistency...")
	if err := validateResponses(nonParallelFile, parallelFile); err != nil {
		return fmt.Errorf("response validation failed: %w", err)
	}
	fmt.Println("✓ Responses match perfectly")

	// Print performance comparison
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("PERFORMANCE COMPARISON")
	fmt.Println(strings.Repeat("=", 60))

	speedup := float64(nonParallelDuration) / float64(parallelDuration)
	if speedup > 1.0 {
		fmt.Printf("🚀 Parallel was %.2fx faster than non-parallel\n", speedup)
		timeSaved := nonParallelDuration - parallelDuration
		fmt.Printf("⏱️  Time saved: %v (%.1f%%)\n", timeSaved, (1.0-1.0/speedup)*100)
	} else if speedup < 1.0 {
		slowdown := 1.0 / speedup
		fmt.Printf("⚠️  Parallel was %.2fx slower than non-parallel\n", slowdown)
		fmt.Printf("⏱️  Time penalty: %v (%.1f%%)\n", parallelDuration-nonParallelDuration, (slowdown-1.0)*100)
	} else {
		fmt.Println("📊 Both approaches performed equally")
	}

	fmt.Printf("\nDetailed timing:\n")
	fmt.Printf("  Non-parallel: %v\n", nonParallelDuration)
	fmt.Printf("  Parallel:     %v\n", parallelDuration)
	fmt.Printf("  Difference:   %v\n", parallelDuration-nonParallelDuration)

	return nil
}

func benchmarkNonParallel(url string, outputFile *os.File) (time.Duration, int64, error) {
	client := &http.Client{
		Transport: http.DefaultTransport,
	}

	start := time.Now()

	resp, err := client.Get(url)
	if err != nil {
		return 0, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, 0, fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	written, err := io.Copy(outputFile, resp.Body)
	if err != nil {
		return 0, 0, err
	}

	duration := time.Since(start)
	return duration, written, nil
}

func benchmarkParallel(url string, outputFile *os.File) (time.Duration, int64, error) {
	// Create parallel transport with configuration
	parallelTransport := parallel.New(
		http.DefaultTransport,
		parallel.WithMinChunkSize(minChunkSize),
		parallel.WithMaxConcurrentPerRequest(maxConcurrent),
	)

	client := &http.Client{
		Transport: parallelTransport,
	}

	start := time.Now()

	resp, err := client.Get(url)
	if err != nil {
		return 0, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, 0, fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	written, err := io.Copy(outputFile, resp.Body)
	if err != nil {
		return 0, 0, err
	}

	duration := time.Since(start)
	return duration, written, nil
}

func validateResponses(file1, file2 *os.File) error {
	// Get file sizes
	stat1, err := file1.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat non-parallel file: %w", err)
	}

	stat2, err := file2.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat parallel file: %w", err)
	}

	if stat1.Size() != stat2.Size() {
		return fmt.Errorf("file sizes differ: non-parallel=%d bytes, parallel=%d bytes",
			stat1.Size(), stat2.Size())
	}

	// Compare file contents
	if _, err := file1.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("failed to seek non-parallel file: %w", err)
	}

	if _, err := file2.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("failed to seek parallel file: %w", err)
	}

	const bufSize = 64 * 1024 // 64KB buffer
	buf1 := make([]byte, bufSize)
	buf2 := make([]byte, bufSize)

	offset := int64(0)
	for {
		n1, err1 := file1.Read(buf1)
		n2, err2 := file2.Read(buf2)

		if n1 != n2 {
			return fmt.Errorf("read size mismatch at offset %d: non-parallel=%d, parallel=%d", offset, n1, n2)
		}

		if n1 > 0 && !bytes.Equal(buf1[:n1], buf2[:n1]) {
			return fmt.Errorf("content mismatch starting at offset %d", offset)
		}

		offset += int64(n1)

		if err1 == io.EOF && err2 == io.EOF {
			break
		}

		if err1 != nil {
			return fmt.Errorf("error reading non-parallel file: %w", err1)
		}

		if err2 != nil {
			return fmt.Errorf("error reading parallel file: %w", err2)
		}
	}

	return nil
}
