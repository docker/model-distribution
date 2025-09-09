package parallel

import (
	"bytes"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/docker/model-distribution/transport/internal/common"
)

// ───────────────────────── Test Harness Types & Utilities ─────────────────────────

// testPlan specifies the behavior of the fake transport for a single URL.
type testPlan struct {
	// NoRangeSupport indicates that the server does not support range requests
	// (no Accept-Ranges header and never returns 206).
	NoRangeSupport bool

	// NoContentLength indicates that the server should not provide a
	// Content-Length header in HEAD responses.
	NoContentLength bool

	// ContentEncoding, when non-empty, is set as the Content-Encoding
	// of HEAD responses to simulate compressed delivery.
	ContentEncoding string

	// ForceError causes the transport to return an error for any request.
	ForceError bool

	// SlowChunk simulates slow chunk downloads by adding delays.
	SlowChunk bool

	// WrongRangeResponse causes range requests to return incorrect ranges.
	WrongRangeResponse bool

	// CustomETag allows setting a custom ETag value.
	CustomETag string

	// OmitETag indicates that the server should not provide ETag headers.
	OmitETag bool

	// WeakETag indicates that the server should provide a weak ETag.
	WeakETag bool

	// ChangeETagOnRange indicates that the server should change the ETag
	// when serving range requests (simulating resource changes).
	ChangeETagOnRange bool

	// RequireIfRange indicates that the server requires If-Range validation
	// and returns 200 OK if the validator doesn't match.
	RequireIfRange bool

	// OmitLastModified indicates that the server should not provide Last-Modified headers.
	OmitLastModified bool
}

// fakeTransport is a deterministic test double that implements http.RoundTripper.
type fakeTransport struct {
	mu sync.Mutex

	// resources maps absolute URL strings to the byte content that will be served.
	resources map[string][]byte

	// plans maps absolute URL strings to their associated behavioral plan.
	plans map[string]*testPlan

	// requestLog tracks all requests made, for verification.
	requestLog []http.Request
}

// newFakeTransport constructs and returns a new fakeTransport.
func newFakeTransport() *fakeTransport {
	return &fakeTransport{
		resources: make(map[string][]byte),
		plans:     make(map[string]*testPlan),
	}
}

// add registers a new URL, its byte payload, and its behavior plan.
func (ft *fakeTransport) add(url string, data []byte, plan *testPlan) {
	ft.mu.Lock()
	defer ft.mu.Unlock()
	ft.resources[url] = data
	ft.plans[url] = plan
}

// getRequestLog returns a copy of all requests made.
func (ft *fakeTransport) getRequestLog() []http.Request {
	ft.mu.Lock()
	defer ft.mu.Unlock()
	log := make([]http.Request, len(ft.requestLog))
	copy(log, ft.requestLog)
	return log
}

// RoundTrip implements http.RoundTripper for fakeTransport.
func (ft *fakeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ft.mu.Lock()
	// Log the request (clone it to avoid races).
	reqCopy := *req
	if req.Header != nil {
		reqCopy.Header = common.CloneHeader(req.Header)
	}
	ft.requestLog = append(ft.requestLog, reqCopy)

	rurl := req.URL.String()
	data, ok := ft.resources[rurl]
	plan := ft.plans[rurl]
	ft.mu.Unlock()

	if plan != nil && plan.ForceError {
		return nil, io.ErrUnexpectedEOF
	}

	if !ok {
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(bytes.NewReader(nil)),
			Request:    req,
		}, nil
	}

	if plan != nil && plan.SlowChunk {
		time.Sleep(50 * time.Millisecond)
	}

	total := int64(len(data))

	// Handle HEAD requests.
	if req.Method == http.MethodHead {
		h := http.Header{}
		if plan == nil || !plan.NoRangeSupport {
			h.Set("Accept-Ranges", "bytes")
		}
		if plan == nil || !plan.NoContentLength {
			h.Set("Content-Length", strconv.FormatInt(total, 10))
		}
		if plan != nil && plan.ContentEncoding != "" {
			h.Set("Content-Encoding", plan.ContentEncoding)
		}

		// Add ETag handling.
		if plan != nil && !plan.OmitETag {
			etag := plan.CustomETag
			if etag == "" {
				if plan.WeakETag {
					etag = `W/"` + strings.ReplaceAll(rurl, "/", "_") + `"`
				} else {
					etag = `"` + strings.ReplaceAll(rurl, "/", "_") + `"`
				}
			}
			h.Set("ETag", etag)
		}

		// Add Last-Modified if not omitted and no ETag.
		if plan != nil && !plan.OmitLastModified && (plan.OmitETag || plan.WeakETag) {
			h.Set("Last-Modified", time.Unix(1_700_000_000, 0).UTC().Format(http.TimeFormat))
		}

		contentLength := total
		if plan != nil && plan.NoContentLength {
			contentLength = -1
		}
		return &http.Response{
			Status:        "200 OK",
			StatusCode:    http.StatusOK,
			Header:        h,
			ContentLength: contentLength,
			Body:          io.NopCloser(bytes.NewReader(nil)),
			Request:       req,
		}, nil
	}

	// Handle GET requests.
	rangeHdr := req.Header.Get("Range")
	if rangeHdr == "" {
		// Full GET request.
		h := http.Header{}
		if plan == nil || !plan.NoRangeSupport {
			h.Set("Accept-Ranges", "bytes")
		}

		contentLength := total
		if plan != nil && plan.NoContentLength {
			contentLength = -1
		} else {
			h.Set("Content-Length", strconv.FormatInt(total, 10))
		}
		return &http.Response{
			Status:        "200 OK",
			StatusCode:    http.StatusOK,
			Header:        h,
			ContentLength: contentLength,
			Body:          io.NopCloser(bytes.NewReader(data)),
			Request:       req,
		}, nil
	}

	// Range GET request.
	if plan != nil && plan.NoRangeSupport {
		// Server doesn't support ranges, return full response.
		return &http.Response{
			Status:        "200 OK",
			StatusCode:    http.StatusOK,
			Header:        http.Header{},
			ContentLength: total,
			Body:          io.NopCloser(bytes.NewReader(data)),
			Request:       req,
		}, nil
	}

	// Parse the Range header.
	var start, end int64 = 0, total - 1
	if !strings.HasPrefix(strings.ToLower(rangeHdr), "bytes=") {
		return &http.Response{
			StatusCode: http.StatusRequestedRangeNotSatisfiable,
			Body:       io.NopCloser(bytes.NewReader(nil)),
			Request:    req,
		}, nil
	}

	spec := strings.TrimSpace(rangeHdr[len("bytes="):])
	parts := strings.SplitN(spec, "-", 2)
	if len(parts) != 2 || parts[0] == "" {
		return &http.Response{
			StatusCode: http.StatusRequestedRangeNotSatisfiable,
			Body:       io.NopCloser(bytes.NewReader(nil)),
			Request:    req,
		}, nil
	}

	var err error
	start, err = strconv.ParseInt(parts[0], 10, 64)
	if err != nil || start < 0 || start >= total {
		return &http.Response{
			StatusCode: http.StatusRequestedRangeNotSatisfiable,
			Body:       io.NopCloser(bytes.NewReader(nil)),
			Request:    req,
		}, nil
	}

	if parts[1] != "" {
		end, err = strconv.ParseInt(parts[1], 10, 64)
		if err != nil || end < start || end >= total {
			return &http.Response{
				StatusCode: http.StatusRequestedRangeNotSatisfiable,
				Body:       io.NopCloser(bytes.NewReader(nil)),
				Request:    req,
			}, nil
		}
	} else {
		end = total - 1
	}

	// Handle If-Range validation for range requests.
	if plan != nil && plan.RequireIfRange {
		expectedETag := plan.CustomETag
		if expectedETag == "" {
			if plan.WeakETag {
				expectedETag = `W/"` + strings.ReplaceAll(rurl, "/", "_") + `"`
			} else {
				expectedETag = `"` + strings.ReplaceAll(rurl, "/", "_") + `"`
			}
		}

		// Check if ETag changed for range requests.
		if plan.ChangeETagOnRange && !plan.OmitETag {
			expectedETag = expectedETag + "-changed"
		}

		ifRange := req.Header.Get("If-Range")
		expectedValidator := expectedETag
		if plan.OmitETag || plan.WeakETag {
			// Use Last-Modified as fallback.
			expectedValidator = time.Unix(1_700_000_000, 0).UTC().Format(http.TimeFormat)
		}

		if ifRange == "" || ifRange != expectedValidator {
			// Return 200 OK instead of 206 when If-Range validation fails.
			h := http.Header{}
			h.Set("Accept-Ranges", "bytes")
			h.Set("Content-Length", strconv.FormatInt(total, 10))
			if !plan.OmitETag {
				h.Set("ETag", expectedETag)
			}
			if !plan.OmitLastModified {
				h.Set("Last-Modified", time.Unix(1_700_000_000, 0).UTC().Format(http.TimeFormat))
			}

			return &http.Response{
				Status:        "200 OK",
				StatusCode:    http.StatusOK,
				Header:        h,
				ContentLength: total,
				Body:          io.NopCloser(bytes.NewReader(data)),
				Request:       req,
			}, nil
		}
	}

	// Optionally return wrong range for testing error handling.
	respStart, respEnd := start, end
	if plan != nil && plan.WrongRangeResponse {
		respStart = start + 1
		respEnd = end + 1
		if respEnd >= total {
			respEnd = total - 1
		}
	}

	chunk := data[respStart : respEnd+1]
	h := http.Header{}
	h.Set("Accept-Ranges", "bytes")
	h.Set("Content-Range", "bytes "+strconv.FormatInt(respStart, 10)+"-"+strconv.FormatInt(respEnd, 10)+"/"+strconv.FormatInt(total, 10))

	// Add ETag to range responses.
	if plan == nil || !plan.OmitETag {
		etag := ""
		if plan != nil && plan.CustomETag != "" {
			etag = plan.CustomETag
		} else if plan != nil && plan.WeakETag {
			etag = `W/"` + strings.ReplaceAll(rurl, "/", "_") + `"`
		} else {
			etag = `"` + strings.ReplaceAll(rurl, "/", "_") + `"`
		}

		// Change ETag for range requests if requested.
		if plan != nil && plan.ChangeETagOnRange {
			etag = etag + "-changed"
		}

		h.Set("ETag", etag)
	}

	// Add Last-Modified if not omitted.
	if plan == nil || !plan.OmitLastModified {
		h.Set("Last-Modified", time.Unix(1_700_000_000, 0).UTC().Format(http.TimeFormat))
	}

	return &http.Response{
		Status:        "206 Partial Content",
		StatusCode:    http.StatusPartialContent,
		Header:        h,
		ContentLength: int64(len(chunk)),
		Body:          io.NopCloser(bytes.NewReader(chunk)),
		Request:       req,
	}, nil
}

// newClient creates an http.Client with the parallel transport.
func newClient(rt http.RoundTripper, opts ...Option) *http.Client {
	return &http.Client{Transport: New(rt, opts...)}
}

// ─────────────────────────────────── Tests ───────────────────────────────────

// TestParallelDownload_Success verifies that a large file is downloaded in parallel
// and the result matches the original.
func TestParallelDownload_Success(t *testing.T) {
	url := "https://example.com/large-file"
	payload := bytes.Repeat([]byte("abcdefghij"), 10000) // 100KB
	ft := newFakeTransport()
	ft.add(url, payload, &testPlan{})

	client := newClient(ft, WithMaxConcurrentPerRequest(4), WithMinChunkSize(1024))
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if !bytes.Equal(got, payload) {
		t.Errorf("payload mismatch: got %d bytes, want %d", len(got), len(payload))
	}

	// Verify parallel requests were made.
	log := ft.getRequestLog()
	headCount := 0
	rangeCount := 0
	for _, req := range log {
		if req.Method == http.MethodHead {
			headCount++
		} else if req.Method == http.MethodGet && req.Header.Get("Range") != "" {
			rangeCount++
		}
	}

	if headCount != 1 {
		t.Errorf("expected 1 HEAD request, got %d", headCount)
	}
	if rangeCount < 2 {
		t.Errorf("expected at least 2 range requests, got %d", rangeCount)
	}
}

// TestSmallFile_FallsBackToSingle verifies that small files are not parallelized.
func TestSmallFile_FallsBackToSingle(t *testing.T) {
	url := "https://example.com/small-file"
	payload := []byte("small content")
	ft := newFakeTransport()
	ft.add(url, payload, &testPlan{})

	client := newClient(ft, WithMaxConcurrentPerRequest(4), WithMinChunkSize(1024))
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if !bytes.Equal(got, payload) {
		t.Errorf("payload mismatch: got %d bytes, want %d", len(got), len(payload))
	}

	// Verify no parallel requests were made (should be single GET).
	log := ft.getRequestLog()
	headCount := 0
	rangeCount := 0
	fullGetCount := 0
	for _, req := range log {
		if req.Method == http.MethodHead {
			headCount++
		} else if req.Method == http.MethodGet && req.Header.Get("Range") != "" {
			rangeCount++
		} else if req.Method == http.MethodGet {
			fullGetCount++
		}
	}

	if headCount != 1 {
		t.Errorf("expected 1 HEAD request, got %d", headCount)
	}
	if rangeCount != 0 {
		t.Errorf("expected 0 range requests for small file, got %d", rangeCount)
	}
	if fullGetCount != 1 {
		t.Errorf("expected 1 full GET request, got %d", fullGetCount)
	}
}

// TestNoRangeSupport_FallsBack verifies fallback when server doesn't support ranges.
func TestNoRangeSupport_FallsBack(t *testing.T) {
	url := "https://example.com/no-ranges"
	payload := bytes.Repeat([]byte("no-range"), 10000) // 80KB
	ft := newFakeTransport()
	ft.add(url, payload, &testPlan{NoRangeSupport: true})

	client := newClient(ft, WithMaxConcurrentPerRequest(4), WithMinChunkSize(1024))
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if !bytes.Equal(got, payload) {
		t.Errorf("payload mismatch: got %d bytes, want %d", len(got), len(payload))
	}

	// Should fall back to single GET after HEAD check.
	log := ft.getRequestLog()
	headCount := 0
	rangeCount := 0
	fullGetCount := 0
	for _, req := range log {
		if req.Method == http.MethodHead {
			headCount++
		} else if req.Method == http.MethodGet && req.Header.Get("Range") != "" {
			rangeCount++
		} else if req.Method == http.MethodGet {
			fullGetCount++
		}
	}

	if headCount != 1 {
		t.Errorf("expected 1 HEAD request, got %d", headCount)
	}
	if rangeCount != 0 {
		t.Errorf("expected 0 range requests when no range support, got %d", rangeCount)
	}
	if fullGetCount != 1 {
		t.Errorf("expected 1 full GET request, got %d", fullGetCount)
	}
}

// TestContentEncoding_FallsBack verifies fallback when content is encoded.
func TestContentEncoding_FallsBack(t *testing.T) {
	url := "https://example.com/compressed"
	payload := bytes.Repeat([]byte("compressed"), 10000) // 100KB
	ft := newFakeTransport()
	ft.add(url, payload, &testPlan{ContentEncoding: "gzip"})

	client := newClient(ft, WithMaxConcurrentPerRequest(4), WithMinChunkSize(1024))
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if !bytes.Equal(got, payload) {
		t.Errorf("payload mismatch: got %d bytes, want %d", len(got), len(payload))
	}

	// Should fall back to single GET after detecting compression.
	log := ft.getRequestLog()
	headCount := 0
	rangeCount := 0
	for _, req := range log {
		if req.Method == http.MethodHead {
			headCount++
		} else if req.Method == http.MethodGet && req.Header.Get("Range") != "" {
			rangeCount++
		}
	}

	if headCount != 1 {
		t.Errorf("expected 1 HEAD request, got %d", headCount)
	}
	if rangeCount != 0 {
		t.Errorf("expected 0 range requests when content is encoded, got %d", rangeCount)
	}
}

// TestNoContentLength_FallsBack verifies fallback when content length is unknown.
func TestNoContentLength_FallsBack(t *testing.T) {
	url := "https://example.com/no-length"
	payload := bytes.Repeat([]byte("unknown"), 10000) // 70KB
	ft := newFakeTransport()
	ft.add(url, payload, &testPlan{NoContentLength: true})

	client := newClient(ft, WithMaxConcurrentPerRequest(4), WithMinChunkSize(1024))
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if !bytes.Equal(got, payload) {
		t.Errorf("payload mismatch: got %d bytes, want %d", len(got), len(payload))
	}

	// Should fall back to single GET when content length is unknown.
	log := ft.getRequestLog()
	rangeCount := 0
	for _, req := range log {
		if req.Method == http.MethodGet && req.Header.Get("Range") != "" {
			rangeCount++
		}
	}

	if rangeCount != 0 {
		t.Errorf("expected 0 range requests when content length unknown, got %d", rangeCount)
	}
}

// TestNonGetRequest_PassesThrough verifies that non-GET requests pass through unchanged.
func TestNonGetRequest_PassesThrough(t *testing.T) {
	url := "https://example.com/post-data"
	payload := []byte("response data")
	ft := newFakeTransport()
	ft.add(url, payload, &testPlan{})

	client := newClient(ft)
	resp, err := client.Post(url, "text/plain", strings.NewReader("post body"))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	// Verify no HEAD request was made.
	log := ft.getRequestLog()
	if len(log) != 1 {
		t.Errorf("expected 1 request, got %d", len(log))
	}
	if log[0].Method != http.MethodPost {
		t.Errorf("expected POST method, got %s", log[0].Method)
	}
}

// TestWrongRangeResponse_HandlesError verifies error handling when server returns wrong ranges.
func TestWrongRangeResponse_HandlesError(t *testing.T) {
	url := "https://example.com/wrong-range"
	payload := bytes.Repeat([]byte("wrongrange"), 10000) // 100KB
	ft := newFakeTransport()
	ft.add(url, payload, &testPlan{WrongRangeResponse: true})

	client := newClient(ft, WithMaxConcurrentPerRequest(4), WithMinChunkSize(1024))
	_, err := client.Get(url)
	if err == nil {
		t.Fatalf("expected error during GET due to wrong ranges")
	}

	if !strings.Contains(err.Error(), "server returned range") {
		t.Errorf("expected range error, got: %v", err)
	} else {
		t.Logf("Got expected error: %v", err)
	}
}

// TestConcurrencyLimits verifies that per-host concurrency limits are respected.
func TestConcurrencyLimits(t *testing.T) {
	url := "https://example.com/concurrent-test"
	payload := bytes.Repeat([]byte("concurrent"), 50000) // 500KB
	ft := newFakeTransport()
	ft.add(url, payload, &testPlan{SlowChunk: true}) // Slow to observe concurrency

	// Set low concurrency limit.
	limits := map[string]uint{"example.com": 2}
	client := newClient(ft,
		WithMaxConcurrentPerHost(limits),
		WithMaxConcurrentPerRequest(8),
		WithMinChunkSize(1024))

	start := time.Now()
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	_, err = io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	elapsed := time.Since(start)

	// With concurrency limit of 2 and slow chunks, this should take some time.
	// This is a rough test - in practice we'd need more sophisticated timing verification.
	if elapsed < 100*time.Millisecond {
		t.Errorf("download completed too quickly, concurrency limits may not be working")
	}
}

// TestChunkBoundaries verifies that chunk boundaries are calculated correctly.
func TestChunkBoundaries(t *testing.T) {
	url := "https://example.com/boundaries"
	payload := bytes.Repeat([]byte("0123456789"), 1000) // 10KB, should split nicely
	ft := newFakeTransport()
	ft.add(url, payload, &testPlan{})

	client := newClient(ft, WithMaxConcurrentPerRequest(3), WithMinChunkSize(1024))
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if !bytes.Equal(got, payload) {
		t.Errorf("payload mismatch: got %d bytes, want %d", len(got), len(payload))
	}

	// Verify that range requests cover the entire file without gaps or overlaps.
	log := ft.getRequestLog()
	ranges := make([][2]int64, 0)
	for _, req := range log {
		if req.Method == http.MethodGet && req.Header.Get("Range") != "" {
			rangeHdr := req.Header.Get("Range")
			if strings.HasPrefix(rangeHdr, "bytes=") {
				spec := strings.TrimSpace(rangeHdr[len("bytes="):])
				parts := strings.SplitN(spec, "-", 2)
				if len(parts) == 2 {
					start, _ := strconv.ParseInt(parts[0], 10, 64)
					end, _ := strconv.ParseInt(parts[1], 10, 64)
					// Skip the header request (bytes=0-0).
					if start == 0 && end == 0 {
						continue
					}
					ranges = append(ranges, [2]int64{start, end})
				}
			}
		}
	}

	if len(ranges) == 0 {
		t.Fatal("no range requests found")
	}

	// Sort ranges by start position.
	for i := 0; i < len(ranges)-1; i++ {
		for j := i + 1; j < len(ranges); j++ {
			if ranges[j][0] < ranges[i][0] {
				ranges[i], ranges[j] = ranges[j], ranges[i]
			}
		}
	}

	// Verify coverage is complete.
	expectedNext := int64(0)
	for _, r := range ranges {
		if r[0] != expectedNext {
			t.Errorf("gap in coverage: expected start %d, got %d", expectedNext, r[0])
		}
		expectedNext = r[1] + 1
	}
	if expectedNext != int64(len(payload)) {
		t.Errorf("incomplete coverage: expected end %d, got %d", len(payload), expectedNext)
	}
}

// ─────────────────────────────── ETag Tests ───────────────────────────────

// TestETagValidation_Success verifies that ETag validation works correctly.
func TestETagValidation_Success(t *testing.T) {
	url := "https://example.com/etag-test"
	payload := bytes.Repeat([]byte("etag-data"), 10000) // 90KB
	ft := newFakeTransport()
	ft.add(url, payload, &testPlan{CustomETag: `"test-etag-123"`, RequireIfRange: true})

	client := newClient(ft, WithMaxConcurrentPerRequest(4), WithMinChunkSize(1024))
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if !bytes.Equal(got, payload) {
		t.Errorf("payload mismatch: got %d bytes, want %d", len(got), len(payload))
	}

	// Verify If-Range headers were sent.
	log := ft.getRequestLog()
	ifRangeCount := 0
	for _, req := range log {
		if req.Method == http.MethodGet && req.Header.Get("Range") != "" {
			if ifRange := req.Header.Get("If-Range"); ifRange == `"test-etag-123"` {
				ifRangeCount++
			}
		}
	}

	if ifRangeCount == 0 {
		t.Error("expected If-Range headers to be sent with range requests")
	}
}

// TestWeakETag_FallsBackToLastModified verifies that weak ETags are ignored
// and Last-Modified is used instead.
func TestWeakETag_FallsBackToLastModified(t *testing.T) {
	url := "https://example.com/weak-etag-test"
	payload := bytes.Repeat([]byte("weak-etag"), 10000) // 90KB
	ft := newFakeTransport()
	ft.add(url, payload, &testPlan{WeakETag: true, RequireIfRange: true})

	client := newClient(ft, WithMaxConcurrentPerRequest(4), WithMinChunkSize(1024))
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if !bytes.Equal(got, payload) {
		t.Errorf("payload mismatch: got %d bytes, want %d", len(got), len(payload))
	}

	// Verify Last-Modified was used instead of weak ETag.
	log := ft.getRequestLog()
	lastModifiedCount := 0
	expectedLM := time.Unix(1_700_000_000, 0).UTC().Format(http.TimeFormat)

	for _, req := range log {
		if req.Method == http.MethodGet && req.Header.Get("Range") != "" {
			if ifRange := req.Header.Get("If-Range"); ifRange == expectedLM {
				lastModifiedCount++
			}
		}
	}

	if lastModifiedCount == 0 {
		t.Error("expected If-Range headers to use Last-Modified for weak ETags")
	}
}

// TestETagChanged_FallsBackToSingle verifies fallback when ETag changes between requests.
func TestETagChanged_FallsBackToSingle(t *testing.T) {
	url := "https://example.com/etag-changed"
	payload := bytes.Repeat([]byte("changed-etag"), 10000) // 120KB
	ft := newFakeTransport()
	ft.add(url, payload, &testPlan{
		CustomETag:        `"original-etag"`,
		RequireIfRange:    true,
		ChangeETagOnRange: true,
	})

	client := newClient(ft, WithMaxConcurrentPerRequest(4), WithMinChunkSize(1024))
	_, err := client.Get(url)

	// Should get an error due to ETag mismatch.
	if err == nil {
		t.Fatal("expected error due to ETag change, got nil")
	}

	if !strings.Contains(err.Error(), "resource may have changed") {
		t.Errorf("expected 'resource may have changed' error, got: %v", err)
	}
}

// TestNoValidator_StillWorks verifies that parallel requests work even without ETags or Last-Modified.
func TestNoValidator_StillWorks(t *testing.T) {
	url := "https://example.com/no-validator"
	payload := bytes.Repeat([]byte("no-validator"), 10000) // 120KB
	ft := newFakeTransport()
	ft.add(url, payload, &testPlan{
		OmitETag:         true,
		OmitLastModified: true,
	})

	client := newClient(ft, WithMaxConcurrentPerRequest(4), WithMinChunkSize(1024))
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if !bytes.Equal(got, payload) {
		t.Errorf("payload mismatch: got %d bytes, want %d", len(got), len(payload))
	}

	// Verify no If-Range headers were sent (no validator available).
	log := ft.getRequestLog()
	for _, req := range log {
		if req.Method == http.MethodGet && req.Header.Get("Range") != "" {
			if ifRange := req.Header.Get("If-Range"); ifRange != "" {
				t.Errorf("expected no If-Range header when no validator available, got: %s", ifRange)
			}
		}
	}
}

// TestConditionalHeadersScrubbed verifies that conditional headers are removed from range requests.
func TestConditionalHeadersScrubbed(t *testing.T) {
	url := "https://example.com/scrub-headers"
	payload := bytes.Repeat([]byte("scrub-test"), 10000) // 100KB
	ft := newFakeTransport()
	ft.add(url, payload, &testPlan{CustomETag: `"scrub-etag"`})

	client := newClient(ft, WithMaxConcurrentPerRequest(4), WithMinChunkSize(1024))

	// Create request with conditional headers that should be scrubbed.
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("If-None-Match", `"some-etag"`)
	req.Header.Set("If-Modified-Since", time.Now().Format(http.TimeFormat))
	req.Header.Set("If-Match", `"another-etag"`)
	req.Header.Set("If-Unmodified-Since", time.Now().Format(http.TimeFormat))

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	_, err = io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	// Verify conditional headers were scrubbed from range requests.
	log := ft.getRequestLog()
	conditionalHeaders := []string{"If-None-Match", "If-Modified-Since", "If-Match", "If-Unmodified-Since"}

	rangeRequestCount := 0
	for _, req := range log {
		if req.Method == http.MethodGet && req.Header.Get("Range") != "" {
			rangeRequestCount++
			rangeHeader := req.Header.Get("Range")
			t.Logf("Range request %d: Range=%s, If-Range=%s", rangeRequestCount, rangeHeader, req.Header.Get("If-Range"))

			for _, header := range conditionalHeaders {
				if value := req.Header.Get(header); value != "" {
					t.Errorf("expected %s header to be scrubbed from range request, got: %s", header, value)
				}
			}

			// Verify If-Range is present for chunk requests (not the header request bytes=0-0).
			if rangeHeader != "bytes=0-0" {
				if ifRange := req.Header.Get("If-Range"); ifRange == "" {
					t.Errorf("expected If-Range header in chunk range request %s", rangeHeader)
				}
			}
		}
	}

	if rangeRequestCount == 0 {
		t.Error("no range requests found - parallel download may not have been triggered")
	}
}
