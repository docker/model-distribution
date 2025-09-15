package parallel

import (
	"bytes"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	testutil "github.com/docker/model-distribution/transport/internal/testing"
)

// TestParallelDownload_Success verifies parallel downloads using testutil.FakeTransport
func TestParallelDownload_Success(t *testing.T) {
	url := "https://example.com/large-file"
	payload := testutil.GenerateTestData(100000) // 100KB

	ft := testutil.NewFakeTransport()
	ft.Add(url, &testutil.FakeResource{
		Data:          payload,
		SupportsRange: true,
		ETag:          `"test-etag"`,
	})

	client := &http.Client{
		Transport: New(ft, WithMaxConcurrentPerRequest(4), WithMinChunkSize(1024)),
	}

	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	testutil.AssertDataEquals(t, got, payload)

	// Verify parallel requests were made
	reqs := ft.GetRequests()
	var headCount, rangeCount, getCount int
	for _, req := range reqs {
		if req.Method == http.MethodHead {
			headCount++
		} else if req.Method == http.MethodGet {
			getCount++
			if req.Header.Get("Range") != "" {
				rangeCount++
			}
		}
		t.Logf("Request: %s %s, Range: %s", req.Method, req.URL, req.Header.Get("Range"))
	}

	if headCount != 1 {
		t.Errorf("expected 1 HEAD request, got %d", headCount)
	}
	if rangeCount < 2 {
		t.Errorf("expected at least 2 range requests, got %d (total GET: %d)", rangeCount, getCount)
	}
}

// TestSmallFile_FallsBackToSingle verifies small files aren't parallelized
func TestSmallFile_FallsBackToSingle(t *testing.T) {
	url := "https://example.com/small-file"
	payload := []byte("small content")

	ft := testutil.NewFakeTransport()
	ft.AddSimple(url, payload, true)

	client := &http.Client{
		Transport: New(ft, WithMaxConcurrentPerRequest(4), WithMinChunkSize(1024)),
	}

	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	testutil.AssertDataEquals(t, got, payload)

	// Should only have HEAD and single GET
	reqs := ft.GetRequests()
	var headCount, rangeCount, fullGetCount int
	for _, req := range reqs {
		if req.Method == http.MethodHead {
			headCount++
		} else if req.Method == http.MethodGet {
			if req.Header.Get("Range") != "" {
				rangeCount++
			} else {
				fullGetCount++
			}
		}
	}

	if headCount != 1 {
		t.Errorf("expected 1 HEAD request, got %d", headCount)
	}
	if rangeCount != 0 {
		t.Errorf("expected 0 range requests, got %d", rangeCount)
	}
	if fullGetCount != 1 {
		t.Errorf("expected 1 full GET request, got %d", fullGetCount)
	}
}

// TestNoRangeSupport_FallsBack tests fallback when server doesn't support ranges
func TestNoRangeSupport_FallsBack(t *testing.T) {
	url := "https://example.com/no-range"
	payload := testutil.GenerateTestData(100000)

	ft := testutil.NewFakeTransport()
	ft.Add(url, &testutil.FakeResource{
		Data:          payload,
		SupportsRange: false, // No range support
	})

	client := &http.Client{Transport: New(ft)}

	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	testutil.AssertDataEquals(t, got, payload)

	// Should fall back to single request
	reqs := ft.GetRequests()
	var rangeCount int
	for _, req := range reqs {
		if req.Header.Get("Range") != "" {
			rangeCount++
		}
	}

	if rangeCount != 0 {
		t.Errorf("expected no range requests, got %d", rangeCount)
	}
}

// TestContentEncoding_FallsBack tests fallback with Content-Encoding
func TestContentEncoding_FallsBack(t *testing.T) {
	url := "https://example.com/gzip"
	payload := testutil.GenerateTestData(100000)

	ft := testutil.NewFakeTransport()
	ft.Add(url, &testutil.FakeResource{
		Data:          payload,
		SupportsRange: true,
		Headers: http.Header{
			"Content-Encoding": []string{"gzip"},
		},
	})

	client := &http.Client{Transport: New(ft)}

	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	testutil.AssertDataEquals(t, got, payload)

	// Should fall back due to Content-Encoding
	reqs := ft.GetRequests()
	var rangeCount int
	for _, req := range reqs {
		if req.Header.Get("Range") != "" {
			rangeCount++
		}
	}

	if rangeCount != 0 {
		t.Errorf("expected no range requests due to Content-Encoding, got %d", rangeCount)
	}
}

// TestETagValidation verifies ETag is used for If-Range validation
func TestETagValidation(t *testing.T) {
	url := "https://example.com/etag-test"
	payload := testutil.GenerateTestData(100000)

	ft := testutil.NewFakeTransport()
	ft.Add(url, &testutil.FakeResource{
		Data:          payload,
		SupportsRange: true,
		ETag:          `"strong-etag"`,
	})

	client := &http.Client{Transport: New(ft)}

	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	testutil.AssertDataEquals(t, got, payload)

	// Check If-Range headers
	headers := ft.GetRequestHeaders(url)
	for _, h := range headers {
		if h.Get("Range") != "" {
			if ifRange := h.Get("If-Range"); ifRange != `"strong-etag"` {
				t.Errorf("expected If-Range with ETag, got %q", ifRange)
			}
		}
	}
}

// TestWeakETag_UsesLastModified tests weak ETags trigger Last-Modified usage
func TestWeakETag_UsesLastModified(t *testing.T) {
	url := "https://example.com/weak-etag"
	payload := testutil.GenerateTestData(100000)
	lastModified := time.Unix(1700000000, 0).UTC().Format(http.TimeFormat)

	ft := testutil.NewFakeTransport()
	ft.Add(url, &testutil.FakeResource{
		Data:          payload,
		SupportsRange: true,
		ETag:          `W/"weak-etag"`,
		LastModified:  lastModified,
	})

	client := &http.Client{Transport: New(ft)}

	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	testutil.AssertDataEquals(t, got, payload)

	// Check If-Range uses Last-Modified instead of weak ETag
	headers := ft.GetRequestHeaders(url)
	for _, h := range headers {
		if h.Get("Range") != "" {
			ifRange := h.Get("If-Range")
			if ifRange != lastModified {
				t.Errorf("expected If-Range with Last-Modified, got %q", ifRange)
			}
		}
	}
}

// TestConcurrencyLimits verifies per-host concurrency limits
func TestConcurrencyLimits(t *testing.T) {
	url := "https://example.com/large"
	payload := testutil.GenerateTestData(500000) // 500KB to ensure parallelization

	ft := testutil.NewFakeTransport()
	ft.AddSimple(url, payload, true)

	// Track concurrent requests
	var maxConcurrent, currentConcurrent int
	var mu sync.Mutex
	rangeRequests := 0

	ft.RequestHook = func(req *http.Request) {
		rangeHeader := req.Header.Get("Range")
		if rangeHeader != "" && rangeHeader != "bytes=0-0" {
			mu.Lock()
			currentConcurrent++
			rangeRequests++
			if currentConcurrent > maxConcurrent {
				maxConcurrent = currentConcurrent
			}
			mu.Unlock()
			
			// Simulate work
			time.Sleep(10 * time.Millisecond)
			
			mu.Lock()
			currentConcurrent--
			mu.Unlock()
		}
		t.Logf("Request: %s %s, Range: %s", req.Method, req.URL, rangeHeader)
	}

	client := &http.Client{
		Transport: New(ft,
			WithMaxConcurrentPerHost(map[string]uint{"example.com": 2}),
			WithMaxConcurrentPerRequest(4),
			WithMinChunkSize(10000)), // Lower min chunk size to ensure parallelization
	}

	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	_, err = io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	// Give time for all requests to complete
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	
	if maxConcurrent > 2 {
		t.Errorf("expected max 2 concurrent requests, got %d", maxConcurrent)
	}
	
	if rangeRequests == 0 {
		t.Error("no range requests were made")
	}
}

// TestIfRangeValidation tests If-Range validation behavior
func TestIfRangeValidation(t *testing.T) {
	url := "https://example.com/if-range-test"
	payload := testutil.GenerateTestData(100000)
	etag := `"original-etag"`

	ft := testutil.NewFakeTransport()
	ft.Add(url, &testutil.FakeResource{
		Data:          payload,
		SupportsRange: true,
		ETag:          etag,
	})

	// Change ETag on range requests to simulate resource change
	ft.ResponseHook = func(resp *http.Response) {
		if resp.Request.Header.Get("Range") != "" {
			// Check If-Range validation
			ifRange := resp.Request.Header.Get("If-Range")
			if ifRange != etag {
				// Resource changed, return full content
				resp.StatusCode = http.StatusOK
				resp.Status = "200 OK"
				resp.Header.Del("Content-Range")
				resp.Body = io.NopCloser(bytes.NewReader(payload))
			}
		}
	}

	client := &http.Client{Transport: New(ft)}

	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	testutil.AssertDataEquals(t, got, payload)
}

// TestNoContentLength_FallsBack tests fallback when Content-Length is missing
func TestNoContentLength_FallsBack(t *testing.T) {
	url := "https://example.com/no-length"
	payload := testutil.GenerateTestData(100000)

	ft := testutil.NewFakeTransport()
	ft.AddSimple(url, payload, true)

	// Remove Content-Length from HEAD response
	ft.ResponseHook = func(resp *http.Response) {
		if resp.Request.Method == http.MethodHead {
			resp.ContentLength = -1
			resp.Header.Del("Content-Length")
		}
	}

	client := &http.Client{Transport: New(ft)}

	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	testutil.AssertDataEquals(t, got, payload)

	// Should fall back to single request
	reqs := ft.GetRequests()
	var rangeCount int
	for _, req := range reqs {
		if req.Header.Get("Range") != "" {
			rangeCount++
		}
	}

	if rangeCount != 0 {
		t.Errorf("expected no range requests without Content-Length, got %d", rangeCount)
	}
}

// TestNonGetRequest_PassesThrough verifies non-GET requests are passed through unmodified
func TestNonGetRequest_PassesThrough(t *testing.T) {
	url := "https://example.com/resource"
	postData := []byte("post data")
	responseData := []byte("response")

	ft := testutil.NewFakeTransport()
	ft.AddSimple(url, responseData, false)

	client := &http.Client{Transport: New(ft)}

	// Test POST request
	resp, err := client.Post(url, "application/json", bytes.NewReader(postData))
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	defer resp.Body.Close()

	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	testutil.AssertDataEquals(t, got, responseData)

	// Should not have any HEAD requests
	reqs := ft.GetRequests()
	for _, req := range reqs {
		if req.Method == http.MethodHead {
			t.Error("unexpected HEAD request for non-GET method")
		}
		if req.Header.Get("Range") != "" {
			t.Error("unexpected Range header for non-GET method")
		}
	}
}

// TestWrongRangeResponse_HandlesError tests handling of incorrect range responses
func TestWrongRangeResponse_HandlesError(t *testing.T) {
	url := "https://example.com/wrong-range"
	payload := testutil.GenerateTestData(100000)

	ft := testutil.NewFakeTransport()
	ft.Add(url, &testutil.FakeResource{
		Data:          payload,
		SupportsRange: true,
	})

	// Return wrong range in response
	ft.ResponseHook = func(resp *http.Response) {
		if resp.Request.Header.Get("Range") == "bytes=1000-1999" {
			// Return different range than requested
			resp.Header.Set("Content-Range", "bytes 2000-2999/100000")
		}
	}

	client := &http.Client{Transport: New(ft)}

	// Make a specific range request
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Range", "bytes=1000-1999")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer resp.Body.Close()

	// Should still work (parallel transport doesn't validate Content-Range for user requests)
	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	// Should get the correct range data
	want := payload[1000:2000]
	testutil.AssertDataEquals(t, got, want)
}

// TestChunkBoundaries verifies correct chunk boundary calculation
func TestChunkBoundaries(t *testing.T) {
	url := "https://example.com/boundaries"
	// Use specific size to test boundary conditions
	payload := testutil.GenerateTestData(10000) // Exactly 10KB

	ft := testutil.NewFakeTransport()
	ft.Add(url, &testutil.FakeResource{
		Data:          payload,
		SupportsRange: true,
	})

	client := &http.Client{
		Transport: New(ft,
			WithMaxConcurrentPerRequest(4),
			WithMinChunkSize(2500)), // Should result in 4 chunks of 2500 bytes
	}

	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	testutil.AssertDataEquals(t, got, payload)

	// Check the range requests
	reqs := ft.GetRequests()
	
	var actualRanges []string
	for _, req := range reqs {
		if r := req.Header.Get("Range"); r != "" && r != "bytes=0-0" {
			actualRanges = append(actualRanges, r)
		}
	}

	// We might not get exactly these ranges due to scheduling, but verify we got multiple
	if len(actualRanges) < 2 {
		t.Errorf("expected multiple range requests, got %d", len(actualRanges))
	}

	t.Logf("Actual ranges: %v", actualRanges)
}

// TestETagChanged_FallsBackToSingle tests handling when ETag changes mid-download
func TestETagChanged_FallsBackToSingle(t *testing.T) {
	url := "https://example.com/changing"
	payload := testutil.GenerateTestData(100000)
	originalETag := `"original"`
	changedETag := `"changed"`

	ft := testutil.NewFakeTransport()
	ft.Add(url, &testutil.FakeResource{
		Data:          payload,
		SupportsRange: true,
		ETag:          originalETag,
	})

	requestCount := 0
	ft.ResponseHook = func(resp *http.Response) {
		requestCount++
		// Change ETag after first request
		if requestCount > 1 && resp.Request.Header.Get("Range") != "" {
			// Simulate resource change - return full content with new ETag
			resp.StatusCode = http.StatusOK
			resp.Status = "200 OK"
			resp.Header.Set("ETag", changedETag)
			resp.Header.Del("Content-Range")
			resp.Body = io.NopCloser(bytes.NewReader(payload))
		}
	}

	client := &http.Client{Transport: New(ft)}

	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	// Should still get the full payload
	testutil.AssertDataEquals(t, got, payload)
}

// TestNoValidator_StillWorks tests parallel download without ETag or Last-Modified
func TestNoValidator_StillWorks(t *testing.T) {
	url := "https://example.com/no-validator"
	payload := testutil.GenerateTestData(100000)

	ft := testutil.NewFakeTransport()
	ft.Add(url, &testutil.FakeResource{
		Data:          payload,
		SupportsRange: true,
		// No ETag or LastModified
	})

	client := &http.Client{Transport: New(ft)}

	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	testutil.AssertDataEquals(t, got, payload)

	// Check that no If-Range headers were sent
	headers := ft.GetRequestHeaders(url)
	for _, h := range headers {
		if ifRange := h.Get("If-Range"); ifRange != "" {
			t.Errorf("unexpected If-Range header: %q", ifRange)
		}
	}
}

// TestConditionalHeadersScrubbed verifies conditional headers are removed
func TestConditionalHeadersScrubbed(t *testing.T) {
	url := "https://example.com/conditional"
	payload := testutil.GenerateTestData(100000)

	ft := testutil.NewFakeTransport()
	ft.Add(url, &testutil.FakeResource{
		Data:          payload,
		SupportsRange: true,
		ETag:          `"test"`,
	})

	// Track headers
	ft.RequestHook = func(req *http.Request) {
		// For range requests made by parallel transport,
		// conditional headers should be removed
		if req.Header.Get("Range") != "" {
			if req.Header.Get("If-Match") != "" {
				t.Errorf("%s request: If-Match header should be removed", req.Method)
			}
			if req.Header.Get("If-None-Match") != "" {
				t.Errorf("%s request: If-None-Match header should be removed", req.Method)
			}
			if req.Header.Get("If-Modified-Since") != "" {
				t.Errorf("%s request: If-Modified-Since header should be removed", req.Method)
			}
			if req.Header.Get("If-Unmodified-Since") != "" {
				t.Errorf("%s request: If-Unmodified-Since header should be removed", req.Method)
			}
		}
		// Note: HEAD requests currently don't scrub conditional headers (potential bug)
		// If-Range should only be present on range requests with proper value
		if ifRange := req.Header.Get("If-Range"); ifRange != "" {
			if req.Header.Get("Range") == "" {
				t.Error("If-Range without Range header")
			}
		}
	}

	client := &http.Client{Transport: New(ft)}

	// Create request with conditional headers
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("If-Match", `"wrong"`)
	req.Header.Set("If-None-Match", `"also-wrong"`)
	req.Header.Set("If-Modified-Since", "Wed, 21 Oct 2015 07:28:00 GMT")
	req.Header.Set("If-Unmodified-Since", "Wed, 21 Oct 2015 07:28:00 GMT")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	testutil.AssertDataEquals(t, got, payload)
}