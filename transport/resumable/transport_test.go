package resumable

import (
	"bytes"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// ───────────────────────── Test Harness Types & Utilities ─────────────────────────

// flakePlan specifies the behavior of the fake transport for a single URL.
// It allows tests to deterministically exercise success and error paths.
type flakePlan struct {
	// CutAfter defines, for each served segment, the number of bytes to deliver
	// before injecting a read error. Segment index 0 corresponds to the initial
	// request (without a Range header). Segment index 1 corresponds to the first
	// resume attempt, and so on. A value of -1 means "no failure for that segment".
	// If the slice does not include an entry for a segment, that segment will
	// default to no failure.
	CutAfter []int

	// ForceNon206OnResume indicates that the server should ignore Range headers
	// on resume requests and respond with HTTP 200 OK (full body). This is used
	// to verify that the client rejects non-206 responses during resume.
	ForceNon206OnResume bool

	// WrongStartOnResume indicates that the server should respond with HTTP 206
	// Partial Content but a Content-Range whose start is strictly different from
	// the requested start (we skew it forward by +1). This should be rejected by
	// the client as unsafe.
	WrongStartOnResume bool

	// NoRangeSupport indicates that the server does not support range requests
	// (no Accept-Ranges header and never returns 206). This ensures the wrapper
	// pass-through behavior is exercised.
	NoRangeSupport bool

	// RequireIfRange indicates that the server requires a valid If-Range header
	// on resumed requests. The expected validator is the current strong ETag, if
	// present; otherwise the current Last-Modified value. If the header is missing
	// or does not match, the server returns HTTP 200 OK with the full body.
	RequireIfRange bool

	// ChangeETagOnResume indicates that the server mutates its ETag for resumed
	// requests. This is used to simulate resource changes between segments.
	ChangeETagOnResume bool

	// ChangeLastModifiedOnResume indicates that the server mutates its
	// Last-Modified timestamp for resumed requests. This also simulates a change.
	ChangeLastModifiedOnResume bool

	// OmitETag indicates that the server should omit the ETag header from
	// responses. This forces clients to fall back to Last-Modified for If-Range.
	OmitETag bool

	// OmitLastModified indicates that the server should omit the Last-Modified
	// header from responses.
	OmitLastModified bool

	// InitialContentEncoding, when non-empty, is set as the Content-Encoding
	// of the initial (non-Range) response to simulate compressed delivery.
	InitialContentEncoding string
}

// fakeTransport is a deterministic, concurrency-safe test double that implements
// http.RoundTripper. It serves byte slices from an in-memory map and can emulate
// flakiness and protocol misbehaviors based on a per-URL flakePlan.
type fakeTransport struct {
	mu sync.Mutex // guards all fields below

	// resources maps absolute URL strings to the byte content that will be served.
	resources map[string][]byte

	// plans maps absolute URL strings to their associated behavioral plan.
	plans map[string]*flakePlan

	// etags maps absolute URL strings to the canonical (usually STRONG) ETag that
	// represents the current version for the initial request (segment 0).
	etags map[string]string

	// lastModified maps absolute URL strings to the Last-Modified timestamp value
	// for the initial request (segment 0), formatted per RFC 7231.
	lastModified map[string]string

	// seg tracks how many segments have been served per URL. The initial request
	// uses segment 0, the first resume uses segment 1, and so forth.
	seg map[string]int

	// lastReqHeaders stores a copy of request headers for each segment per URL,
	// allowing tests to assert on what the client sent (e.g., Accept-Encoding).
	lastReqHeaders map[string][]http.Header
}

// newFakeTransport constructs and returns a new fakeTransport with all internal
// maps initialized. It is ready for use as an http.RoundTripper.
func newFakeTransport() *fakeTransport {
	return &fakeTransport{
		resources:      make(map[string][]byte),
		plans:          make(map[string]*flakePlan),
		etags:          make(map[string]string),
		lastModified:   make(map[string]string),
		seg:            make(map[string]int),
		lastReqHeaders: make(map[string][]http.Header),
	}
}

// add registers a new URL, its byte payload, and its behavior plan with the
// fake transport. The ETag is STRONG by default; Last-Modified is fixed.
func (ft *fakeTransport) add(url string, data []byte, plan *flakePlan) {
	ft.mu.Lock()
	defer ft.mu.Unlock()
	ft.resources[url] = data
	ft.plans[url] = plan
	// Strong ETag to match client logic (If-Range only with strong ETags by spec).
	ft.etags[url] = `"` + strings.ReplaceAll(url, "/", "_") + `"`
	ft.lastModified[url] = time.Unix(1_700_000_000, 0).UTC().Format(http.TimeFormat)
}

// segmentHeaders returns copies of the headers for each segment requested for url.
func (ft *fakeTransport) segmentHeaders(url string) []http.Header {
	ft.mu.Lock()
	defer ft.mu.Unlock()
	hs := ft.lastReqHeaders[url]
	out := make([]http.Header, len(hs))
	for i := range hs {
		out[i] = cloneHeader(hs[i])
	}
	return out
}

// RoundTrip implements http.RoundTripper for fakeTransport. It interprets the
// incoming request, consults the configured plan for the target URL, and returns
// an HTTP response that adheres to the requested scenario (including failures).
func (ft *fakeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Record the headers we received for this URL/segment for later assertions.
	rurl := req.URL.String()
	ft.mu.Lock()
	ft.lastReqHeaders[rurl] = append(ft.lastReqHeaders[rurl], cloneHeader(req.Header))

	data, ok := ft.resources[rurl]
	plan := ft.plans[rurl]
	etag := ft.etags[rurl]
	lm := ft.lastModified[rurl]
	if !ok {
		ft.mu.Unlock()
		return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(bytes.NewReader(nil)), Request: req}, nil
	}
	seg := ft.seg[rurl]
	ft.seg[rurl] = seg + 1
	ft.mu.Unlock()

	total := int64(len(data))
	rangeHdr := req.Header.Get("Range")
	supportsRange := plan == nil || !plan.NoRangeSupport

	// Compute validators for this segment (may be omitted or mutated by the plan).
	curETag := etag
	curLM := lm
	if plan != nil {
		if plan.OmitETag {
			curETag = ""
		}
		if plan.OmitLastModified {
			curLM = ""
		}
		if rangeHdr != "" { // resume only
			if plan.ChangeETagOnResume && curETag != "" {
				curETag = curETag + "-changed"
			}
			if plan.ChangeLastModifiedOnResume && curLM != "" {
				parsed, _ := time.Parse(http.TimeFormat, lm) // Safe default
				curLM = parsed.Add(1 * time.Second).UTC().Format(http.TimeFormat)
			}
		}
	}

	// Determine cut-off point for this segment (if any).
	cutAfter := -1
	if plan != nil && seg < len(plan.CutAfter) {
		cutAfter = plan.CutAfter[seg]
	}

	// Helper to build a body that fails after N bytes, if requested.
	makeBody := func(b []byte, cut int) io.ReadCloser {
		if cut < 0 || cut >= len(b) {
			return io.NopCloser(bytes.NewReader(b))
		}
		return newFlakyReader(b, cut)
	}

	// Initial request (no Range header):
	if rangeHdr == "" {
		if !supportsRange {
			return &http.Response{
				Status:        "200 OK",
				StatusCode:    http.StatusOK,
				Header:        cloneHeader(http.Header{}),
				ContentLength: total,
				Body:          makeBody(data, cutAfter),
				Request:       req,
			}, nil
		}
		h := http.Header{}
		h.Set("Accept-Ranges", "bytes")
		if curETag != "" {
			h.Set("ETag", curETag)
		}
		if curLM != "" {
			h.Set("Last-Modified", curLM)
		}
		if plan != nil && plan.InitialContentEncoding != "" {
			h.Set("Content-Encoding", plan.InitialContentEncoding)
		}
		return &http.Response{
			Status:        "200 OK",
			StatusCode:    http.StatusOK,
			Header:        h,
			ContentLength: total,
			Body:          makeBody(data, cutAfter),
			Request:       req,
		}, nil
	}

	// Resume (Range present):
	if !supportsRange {
		return &http.Response{Status: "200 OK", StatusCode: http.StatusOK, Header: http.Header{}, ContentLength: total, Body: makeBody(data, cutAfter), Request: req}, nil
	}

	// Parse the Range header (bytes=start[-end]).
	var start, end int64 = 0, total - 1
	if !strings.HasPrefix(strings.ToLower(rangeHdr), "bytes=") {
		return &http.Response{StatusCode: http.StatusRequestedRangeNotSatisfiable, Body: io.NopCloser(bytes.NewReader(nil)), Request: req}, nil
	}
	spec := strings.TrimSpace(rangeHdr[len("bytes="):])
	parts := strings.SplitN(spec, "-", 2)
	if len(parts) != 2 || parts[0] == "" {
		return &http.Response{StatusCode: http.StatusRequestedRangeNotSatisfiable, Body: io.NopCloser(bytes.NewReader(nil)), Request: req}, nil
	}
	var err error
	start, err = strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return &http.Response{StatusCode: http.StatusRequestedRangeNotSatisfiable, Body: io.NopCloser(bytes.NewReader(nil)), Request: req}, nil
	}
	if parts[1] != "" {
		end, err = strconv.ParseInt(parts[1], 10, 64)
		if err != nil || end < start {
			return &http.Response{StatusCode: http.StatusRequestedRangeNotSatisfiable, Body: io.NopCloser(bytes.NewReader(nil)), Request: req}, nil
		}
	}
	if start >= total {
		h := http.Header{}
		h.Set("Content-Range", "bytes */"+strconv.FormatInt(total, 10))
		return &http.Response{StatusCode: http.StatusRequestedRangeNotSatisfiable, Header: h, Body: io.NopCloser(bytes.NewReader(nil)), Request: req}, nil
	}
	if end >= total {
		end = total - 1
	}

	// If-Range enforcement (server side), if requested by the plan.
	if plan != nil && plan.RequireIfRange {
		// If the advertised ETag is weak, treat it as unusable for If-Range and
		// require Last-Modified instead (aligns with RFC 7232/7233 and client logic).
		expected := curETag
		if expected == "" || isWeakETag(expected) {
			expected = curLM
		}
		ir := req.Header.Get("If-Range")
		if expected == "" || ir == "" || ir != expected {
			h := http.Header{}
			h.Set("Accept-Ranges", "bytes")
			if curETag != "" {
				h.Set("ETag", curETag)
			}
			if curLM != "" {
				h.Set("Last-Modified", curLM)
			}
			return &http.Response{Status: "200 OK", StatusCode: http.StatusOK, Header: h, ContentLength: total, Body: makeBody(data, cutAfter), Request: req}, nil
		}
	}

	if plan != nil && plan.ForceNon206OnResume {
		h := http.Header{}
		h.Set("Accept-Ranges", "bytes")
		if curETag != "" {
			h.Set("ETag", curETag)
		}
		if curLM != "" {
			h.Set("Last-Modified", curLM)
		}
		return &http.Response{Status: "200 OK", StatusCode: http.StatusOK, Header: h, ContentLength: total, Body: makeBody(data, cutAfter), Request: req}, nil
	}

	// Construct 206 Partial Content. Optionally skew the start to simulate a bad range.
	respStart := start
	if plan != nil && plan.WrongStartOnResume {
		respStart = start + 1
		if respStart > end {
			respStart = end
		}
	}
	chunk := data[respStart : end+1]
	h := http.Header{}
	h.Set("Accept-Ranges", "bytes")
	if curETag != "" {
		h.Set("ETag", curETag)
	}
	if curLM != "" {
		h.Set("Last-Modified", curLM)
	}
	h.Set("Content-Range", "bytes "+strconv.FormatInt(respStart, 10)+"-"+strconv.FormatInt(end, 10)+"/"+strconv.FormatInt(total, 10))
	return &http.Response{Status: "206 Partial Content", StatusCode: http.StatusPartialContent, Header: h, ContentLength: int64(len(chunk)), Body: makeBody(chunk, cutAfter), Request: req}, nil
}

// flakyReader is an io.ReadCloser that serves a byte slice and injects a read
// error (io.ErrUnexpectedEOF) after a configured number of bytes.
type flakyReader struct {
	// data is the payload to be served to the client.
	data []byte
	// cutAfter is the absolute number of bytes to deliver before injecting a failure.
	cutAfter int
	// pos is the current read offset into data.
	pos int
	// closed reports whether Close() has been called; further reads error.
	closed bool
}

func newFlakyReader(data []byte, cutAfter int) io.ReadCloser {
	return &flakyReader{data: data, cutAfter: cutAfter}
}

func (fr *flakyReader) Read(p []byte) (int, error) {
	if fr.closed {
		return 0, io.ErrClosedPipe
	}
	if fr.pos >= len(fr.data) {
		return 0, io.EOF
	}
	remain := len(fr.data) - fr.pos
	n := len(p)
	if n > remain {
		n = remain
	}
	if fr.pos < fr.cutAfter && fr.cutAfter < len(fr.data) {
		max := fr.cutAfter - fr.pos
		if max <= 0 {
			return 0, io.ErrUnexpectedEOF
		}
		if n > max {
			n = max
		}
		copy(p[:n], fr.data[fr.pos:fr.pos+n])
		fr.pos += n
		if fr.pos >= fr.cutAfter {
			return n, io.ErrUnexpectedEOF
		}
		return n, nil
	}
	copy(p[:n], fr.data[fr.pos:fr.pos+n])
	fr.pos += n
	if fr.pos >= len(fr.data) {
		return n, io.EOF
	}
	return n, nil
}

func (fr *flakyReader) Close() error { fr.closed = true; return nil }

// newClient is a small helper to build an http.Client with our resumable transport.
func newClient(rt http.RoundTripper, retries int) *http.Client {
	return &http.Client{Transport: New(rt, WithMaxRetries(retries), WithBackoff(func(int) time.Duration { return 0 }))}
}

// ─────────────────────────────────── Tests ───────────────────────────────────

// TestResumeSingleFailure_Succeeds verifies that a single mid-stream failure on
// the initial response is successfully recovered by a single resume attempt,
// and that the resulting assembled payload matches exactly.
func TestResumeSingleFailure_Succeeds(t *testing.T) {
	// Arrange: create payload and fake transport that cuts once on the initial segment.
	url := "https://example.com/blob"
	payload := bytes.Repeat([]byte("abcde"), 1_000) // 5,000 bytes
	ft := newFakeTransport()
	ft.add(url, payload, &flakePlan{CutAfter: []int{1000, -1}})

	// Act: issue GET and read to completion through resumable transport.
	client := newClient(ft, 3)
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })

	// Assert Content-Length for initial 200
	if resp.ContentLength != int64(len(payload)) {
		t.Errorf("ContentLength = %d, want %d", resp.ContentLength, len(payload))
	}

	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	// Assert: reconstructed body matches original payload.
	if !bytes.Equal(got, payload) {
		t.Errorf("payload mismatch: got %d bytes, want %d", len(got), len(payload))
	}
}

// TestResumeMultipleFailuresWithinBudget_Succeeds verifies that multiple
// consecutive mid-stream failures are handled as long as the retry budget is
// sufficient, resulting in a fully correct payload.
func TestResumeMultipleFailuresWithinBudget_Succeeds(t *testing.T) {
	// Arrange
	url := "https://example.com/multi"
	payload := bytes.Repeat([]byte{0x42}, 10_000)
	ft := newFakeTransport()
	ft.add(url, payload, &flakePlan{CutAfter: []int{500, 700, -1}})

	// Act
	client := newClient(ft, 5)
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })

	// Assert Content-Length for initial 200
	if resp.ContentLength != int64(len(payload)) {
		t.Errorf("ContentLength = %d, want %d", resp.ContentLength, len(payload))
	}

	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	// Assert
	if !bytes.Equal(got, payload) {
		t.Errorf("payload mismatch: got %d bytes, want %d", len(got), len(payload))
	}
}

// TestExceedRetryBudget_Fails verifies that when the number of consecutive failures
// exceeds the configured retry budget, the read ultimately fails with an error.
func TestExceedRetryBudget_Fails(t *testing.T) {
	// Arrange
	url := "https://example.com/toosad"
	payload := bytes.Repeat([]byte{0x99}, 4_096)
	ft := newFakeTransport()
	ft.add(url, payload, &flakePlan{CutAfter: []int{1000, 1000, 1000, 1000, 1000}})

	// Act
	client := newClient(ft, 2)
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })

	// Assert Content-Length for initial 200
	if resp.ContentLength != int64(len(payload)) {
		t.Errorf("ContentLength = %d, want %d", resp.ContentLength, len(payload))
	}

	_, rerr := io.ReadAll(resp.Body)

	// Assert
	if rerr == nil {
		t.Errorf("expected read error after exceeding retry budget, got nil")
	}
}

// TestWrongStartOnResume_IsRejected verifies that the client rejects a resume
// response whose Content-Range start differs from the requested start.
func TestWrongStartOnResume_IsRejected(t *testing.T) {
	// Arrange
	url := "https://example.com/wrongstart"
	payload := bytes.Repeat([]byte("XYZ"), 3000)
	ft := newFakeTransport()
	ft.add(url, payload, &flakePlan{CutAfter: []int{1000, -1}, WrongStartOnResume: true})

	// Act
	client := newClient(ft, 2)
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })

	// Assert Content-Length for initial 200
	if resp.ContentLength != int64(len(payload)) {
		t.Errorf("ContentLength = %d, want %d", resp.ContentLength, len(payload))
	}

	_, rerr := io.ReadAll(resp.Body)

	// Assert
	if rerr == nil {
		t.Errorf("expected read error due to wrong Content-Range start, got nil")
	}
}

// TestNon206OnResume_IsRejected verifies that a non-206 response to a resume
// request is rejected and ultimately causes the read to fail.
func TestNon206OnResume_IsRejected(t *testing.T) {
	// Arrange
	url := "https://example.com/non206"
	payload := bytes.Repeat([]byte{0xAA, 0xBB, 0xCC}, 2000)
	ft := newFakeTransport()
	ft.add(url, payload, &flakePlan{CutAfter: []int{1000, -1}, ForceNon206OnResume: true})

	// Act
	client := newClient(ft, 2)
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })

	// Assert Content-Length for initial 200
	if resp.ContentLength != int64(len(payload)) {
		t.Errorf("ContentLength = %d, want %d", resp.ContentLength, len(payload))
	}

	_, rerr := io.ReadAll(resp.Body)

	// Assert
	if rerr == nil {
		t.Errorf("expected read error due to non-206 on resume, got nil")
	}
}

// TestNoRangeSupport_PassesThrough_NoResume verifies that when the server does
// not advertise range support, the wrapper does not attempt to resume and the
// mid-stream error bubbles up to the caller.
func TestNoRangeSupport_PassesThrough_NoResume(t *testing.T) {
	// Arrange
	url := "https://example.com/norange"
	payload := bytes.Repeat([]byte("hello"), 500)
	ft := newFakeTransport()
	ft.add(url, payload, &flakePlan{NoRangeSupport: true, CutAfter: []int{200}})

	// Act
	client := newClient(ft, 3)
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })

	// Assert Content-Length for initial 200
	if resp.ContentLength != int64(len(payload)) {
		t.Errorf("ContentLength = %d, want %d", resp.ContentLength, len(payload))
	}

	buf := make([]byte, 1<<20)
	_, rerr := resp.Body.Read(buf)
	if rerr == nil {
		_, rerr = io.ReadAll(resp.Body)
	}

	// Assert
	if rerr == nil {
		t.Errorf("expected mid-stream error without resume support")
	}
}

// TestIfRange_ETag_Matches_AllowsResume verifies that when the server requires
// If-Range and provides a strong ETag, the client sends the correct validator
// and the resume succeeds.
func TestIfRange_ETag_Matches_AllowsResume(t *testing.T) {
	// Arrange
	url := "https://example.com/ifrange-etag-ok"
	payload := bytes.Repeat([]byte("data-"), 1500) // 7,500 bytes
	ft := newFakeTransport()
	ft.add(url, payload, &flakePlan{CutAfter: []int{1200, -1}, RequireIfRange: true})

	// Act
	client := newClient(ft, 3)
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })

	// Assert Content-Length for initial 200
	if resp.ContentLength != int64(len(payload)) {
		t.Errorf("ContentLength = %d, want %d", resp.ContentLength, len(payload))
	}

	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	// Assert
	if !bytes.Equal(got, payload) {
		t.Errorf("payload mismatch: got %d bytes, want %d", len(got), len(payload))
	}
}

// TestIfRange_ETag_ChangedOnResume_RejectsResume verifies that if the server
// changes its ETag between the initial response and the resume request, the
// client's If-Range will not match and the resume will be rejected.
func TestIfRange_ETag_ChangedOnResume_RejectsResume(t *testing.T) {
	// Arrange
	url := "https://example.com/ifrange-etag-changed"
	payload := bytes.Repeat([]byte("X"), 6000)
	ft := newFakeTransport()
	ft.add(url, payload, &flakePlan{CutAfter: []int{1000, -1}, RequireIfRange: true, ChangeETagOnResume: true})

	// Act
	client := newClient(ft, 2)
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })

	// Assert Content-Length for initial 200
	if resp.ContentLength != int64(len(payload)) {
		t.Errorf("ContentLength = %d, want %d", resp.ContentLength, len(payload))
	}

	_, rerr := io.ReadAll(resp.Body)

	// Assert
	if rerr == nil {
		t.Errorf("expected read error due to If-Range (ETag) mismatch causing non-206 on resume")
	}
}

// TestIfRange_LastModified_Matches_AllowsResume verifies that when the server
// omits ETag but provides Last-Modified, the client uses Last-Modified as the
// If-Range validator and successfully resumes.
func TestIfRange_LastModified_Matches_AllowsResume(t *testing.T) {
	// Arrange
	url := "https://example.com/ifrange-lm-ok"
	payload := bytes.Repeat([]byte("LMOK"), 3000) // 12,000 bytes
	ft := newFakeTransport()
	ft.add(url, payload, &flakePlan{CutAfter: []int{1500, -1}, RequireIfRange: true, OmitETag: true, OmitLastModified: false})

	// Act
	client := newClient(ft, 3)
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })

	// Assert Content-Length for initial 200
	if resp.ContentLength != int64(len(payload)) {
		t.Errorf("ContentLength = %d, want %d", resp.ContentLength, len(payload))
	}

	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	// Assert
	if !bytes.Equal(got, payload) {
		t.Errorf("payload mismatch: got %d bytes, want %d", len(got), len(payload))
	}
}

// TestIfRange_LastModified_ChangedOnResume_RejectsResume verifies that if the
// server changes its Last-Modified timestamp between initial and resume, the
// client's If-Range will not match and the resume will be rejected.
func TestIfRange_LastModified_ChangedOnResume_RejectsResume(t *testing.T) {
	// Arrange
	url := "https://example.com/ifrange-lm-changed"
	payload := bytes.Repeat([]byte{0xAB, 0xCD}, 5000) // 10,000 bytes
	ft := newFakeTransport()
	ft.add(url, payload, &flakePlan{CutAfter: []int{800, -1}, RequireIfRange: true, OmitETag: true, OmitLastModified: false, ChangeLastModifiedOnResume: true})

	// Act
	client := newClient(ft, 2)
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })

	// Assert Content-Length for initial 200
	if resp.ContentLength != int64(len(payload)) {
		t.Errorf("ContentLength = %d, want %d", resp.ContentLength, len(payload))
	}

	_, rerr := io.ReadAll(resp.Body)

	// Assert
	if rerr == nil {
		t.Errorf("expected read error due to If-Range (Last-Modified) mismatch causing non-206 on resume")
	}
}

// TestIfRange_RequiredButUnavailable_MissingRejected verifies that if the server
// requires If-Range but provides no validators at all, the client cannot form
// an If-Range and the resume will be rejected.
func TestIfRange_RequiredButUnavailable_MissingRejected(t *testing.T) {
	// Arrange
	url := "https://example.com/ifrange-missing"
	payload := bytes.Repeat([]byte("no-validator"), 1000)
	ft := newFakeTransport()
	ft.add(url, payload, &flakePlan{CutAfter: []int{300, -1}, RequireIfRange: true, OmitETag: true, OmitLastModified: true})

	// Act
	client := newClient(ft, 2)
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })

	// Assert Content-Length for initial 200
	if resp.ContentLength != int64(len(payload)) {
		t.Errorf("ContentLength = %d, want %d", resp.ContentLength, len(payload))
	}

	_, rerr := io.ReadAll(resp.Body)

	// Assert
	if rerr == nil {
		t.Errorf("expected read error because server required If-Range but provided no validators")
	}
}

// TestIfRange_WeakETag_Present_UsesLastModified_AllowsResume verifies that when
// the server advertises a WEAK ETag and also a Last-Modified timestamp, the
// client will ignore the weak ETag for If-Range, use Last-Modified instead, and
// the resume will succeed.
func TestIfRange_WeakETag_Present_UsesLastModified_AllowsResume(t *testing.T) {
	// Arrange: resource that advertises weak ETag + LM, requires If-Range, and cuts once.
	url := "https://example.com/ifrange-weak-etag"
	payload := bytes.Repeat([]byte("WEAK"), 2500)
	ft := newFakeTransport()
	ft.add(url, payload, &flakePlan{CutAfter: []int{600, -1}, RequireIfRange: true})
	// Override strong ETag with a WEAK one.
	ft.mu.Lock()
	ft.etags[url] = `W/"weak-` + strings.ReplaceAll(url, "/", "_") + `"`
	ft.mu.Unlock()

	// Act: client should send If-Range with Last-Modified (not the weak ETag).
	client := newClient(ft, 3)
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })

	// Assert Content-Length for initial 200
	if resp.ContentLength != int64(len(payload)) {
		t.Errorf("ContentLength = %d, want %d", resp.ContentLength, len(payload))
	}

	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	// Assert: full payload delivered successfully via resume.
	if !bytes.Equal(got, payload) {
		t.Errorf("payload mismatch: got %d bytes, want %d", len(got), len(payload))
	}
}

// TestGzipContentEncoding_DisablesResume verifies that when the initial 200
// response has Content-Encoding set (e.g., gzip), the transport declines to
// wrap the body for resumption and thus a mid-stream failure bubbles up.
func TestGzipContentEncoding_DisablesResume(t *testing.T) {
	// Arrange: range-capable server that serves gzip on initial response and then cuts.
	url := "https://example.com/gzip-initial"
	payload := bytes.Repeat([]byte("zip"), 4000)
	ft := newFakeTransport()
	ft.add(url, payload, &flakePlan{CutAfter: []int{500}, InitialContentEncoding: "gzip"})

	// Act: client uses resumable transport, but it should refuse to wrap due to encoding.
	client := newClient(ft, 3)
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })

	// Assert Content-Length for initial 200 (server advertises total)
	if resp.ContentLength != int64(len(payload)) {
		t.Errorf("ContentLength = %d, want %d", resp.ContentLength, len(payload))
	}

	_, rerr := io.ReadAll(resp.Body)

	// Assert: we see an error because no resume was attempted under compression.
	if rerr == nil {
		t.Errorf("expected mid-stream error when initial response is compressed (no resume)")
	}
}

// TestResumeHeaders_ScrubbedAndIdentityEncoding verifies that on resume the client
// sets Accept-Encoding to identity and scrubs conditional headers that could
// conflict with If-Range semantics.
func TestResumeHeaders_ScrubbedAndIdentityEncoding(t *testing.T) {
	// Arrange: server supports ranges and will cut to force a resume.
	url := "https://example.com/header-scrub"
	payload := bytes.Repeat([]byte("H"), 4000)
	ft := newFakeTransport()
	ft.add(url, payload, &flakePlan{CutAfter: []int{600, -1}})

	client := newClient(ft, 3)
	// Build initial request with headers that should be scrubbed on resume.
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Accept-Encoding", "gzip") // will be overridden to identity on resume
	req.Header.Set("If-None-Match", "\"foo\"")
	req.Header.Set("If-Modified-Since", time.Unix(1_600_000_000, 0).UTC().Format(http.TimeFormat))
	req.Header.Set("If-Match", "\"bar\"")
	req.Header.Set("If-Unmodified-Since", time.Unix(1_600_000_100, 0).UTC().Format(http.TimeFormat))

	// Act: perform request and read to completion (triggering a resume once).
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })

	// Assert Content-Length for initial 200
	if resp.ContentLength != int64(len(payload)) {
		t.Errorf("ContentLength = %d, want %d", resp.ContentLength, len(payload))
	}

	_, _ = io.ReadAll(resp.Body)

	// Assert: fetch recorded headers for each segment.
	hs := ft.segmentHeaders(url)
	if len(hs) < 2 {
		t.Fatalf("expected at least 2 segments (initial + resume), got %d", len(hs))
	}
	initH, resumeH := hs[0], hs[1]

	// Initial request kept our original Accept-Encoding; resume must be identity.
	if got := strings.ToLower(resumeH.Get("Accept-Encoding")); got != "identity" {
		t.Errorf("resume Accept-Encoding = %q, want %q", got, "identity")
	}

	// Conditional headers must be scrubbed on resume.
	condKeys := []string{"If-None-Match", "If-Modified-Since", "If-Match", "If-Unmodified-Since"}
	for _, k := range condKeys {
		if v := resumeH.Get(k); v != "" {
			t.Errorf("resume header %s = %q, want empty", k, v)
		}
	}

	// Sanity: they were present on the initial request to prove scrubbing happened.
	if initH.Get("If-None-Match") == "" || initH.Get("If-Modified-Since") == "" || initH.Get("If-Match") == "" || initH.Get("If-Unmodified-Since") == "" {
		t.Errorf("expected conditional headers on initial request for comparison")
	}

	// Range and If-Range should be present on resume.
	if r := resumeH.Get("Range"); r == "" || !strings.HasPrefix(strings.ToLower(r), "bytes=") {
		t.Errorf("resume Range missing/invalid: %q", r)
	}
	if ir := resumeH.Get("If-Range"); ir == "" {
		t.Errorf("resume If-Range missing")
	}
}

// ─────────────────────────────── Initial-Range tests ───────────────────────────────

// TestRangeInitial_ZeroToN_NoCuts_Succeeds verifies that when the *initial* request
// specifies a Range from 0..N, the transport delivers exactly that slice without
// any failures or resumes.
func TestRangeInitial_ZeroToN_NoCuts_Succeeds(t *testing.T) {
	// Arrange
	url := "https://example.com/range-0-n"
	payload := bytes.Repeat([]byte("0123456789"), 1024) // 10,240 bytes
	N := int64(2047)
	ft := newFakeTransport()
	ft.add(url, payload, &flakePlan{CutAfter: []int{-1}})

	// Act: initial request is a Range request
	client := newClient(ft, 3)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Range", "bytes=0-"+strconv.FormatInt(N, 10))
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })

	// Assert Content-Length for 206 (0..N inclusive)
	if resp.ContentLength != (N + 1) {
		t.Errorf("ContentLength = %d, want %d", resp.ContentLength, N+1)
	}

	got, err := io.ReadAll(resp.Body)
	if err != nil && err != io.EOF {
		t.Fatalf("read: %v", err)
	}

	// Assert
	want := payload[0 : N+1]
	if !bytes.Equal(got, want) {
		t.Errorf("payload mismatch: got %d bytes, want %d", len(got), len(want))
	}
}

// TestRangeInitial_MidSpan_NoCuts_Succeeds verifies a Range N..M (mid-file)
// succeeds without any resumes and matches the exact slice.
func TestRangeInitial_MidSpan_NoCuts_Succeeds(t *testing.T) {
	// Arrange
	url := "https://example.com/range-n-m"
	payload := bytes.Repeat([]byte("ABCDEFGH"), 2048) // 16,384 bytes
	N := int64(500)
	M := int64(3499)
	ft := newFakeTransport()
	ft.add(url, payload, &flakePlan{CutAfter: []int{-1}})

	// Act
	client := newClient(ft, 3)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Range", "bytes="+strconv.FormatInt(N, 10)+"-"+strconv.FormatInt(M, 10))
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })

	// Assert Content-Length for 206 (N..M inclusive)
	wantCL := (M - N + 1)
	if resp.ContentLength != wantCL {
		t.Errorf("ContentLength = %d, want %d", resp.ContentLength, wantCL)
	}

	got, err := io.ReadAll(resp.Body)
	if err != nil && err != io.EOF {
		t.Fatalf("read: %v", err)
	}

	// Assert
	want := payload[N : M+1]
	if !bytes.Equal(got, want) {
		t.Errorf("payload mismatch: got %d bytes, want %d", len(got), len(want))
	}
}

// TestRangeInitial_FromNToEnd_NoCuts_Succeeds verifies a Range N..end request
// ("bytes=N-") succeeds and returns the tail of the object.
func TestRangeInitial_FromNToEnd_NoCuts_Succeeds(t *testing.T) {
	// Arrange
	url := "https://example.com/range-n-end"
	payload := bytes.Repeat([]byte("xyz"), 5000) // 15,000 bytes
	N := int64(2500)
	ft := newFakeTransport()
	ft.add(url, payload, &flakePlan{CutAfter: []int{-1}})

	// Act
	client := newClient(ft, 3)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Range", "bytes="+strconv.FormatInt(N, 10)+"-")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })

	// Assert Content-Length for 206 (N..end inclusive)
	wantCL := int64(len(payload)) - N
	if resp.ContentLength != wantCL {
		t.Errorf("ContentLength = %d, want %d", resp.ContentLength, wantCL)
	}

	got, err := io.ReadAll(resp.Body)
	if err != nil && err != io.EOF {
		t.Fatalf("read: %v", err)
	}

	// Assert
	want := payload[N:]
	if !bytes.Equal(got, want) {
		t.Errorf("payload mismatch: got %d bytes, want %d", len(got), len(want))
	}
}

// TestRangeInitial_ZeroToN_WithCut_Resumes verifies that a Range 0..N with a
// mid-stream cut resumes correctly and still yields the exact slice.
func TestRangeInitial_ZeroToN_WithCut_Resumes(t *testing.T) {
	// Arrange
	url := "https://example.com/range-0-n-cut"
	payload := bytes.Repeat([]byte("Q"), 9000)
	N := int64(4095)
	ft := newFakeTransport()
	ft.add(url, payload, &flakePlan{CutAfter: []int{512, -1}}) // cut during initial segment

	// Act
	client := newClient(ft, 4)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Range", "bytes=0-"+strconv.FormatInt(N, 10))
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })

	// Assert Content-Length for 206 (0..N inclusive)
	if resp.ContentLength != (N + 1) {
		t.Errorf("ContentLength = %d, want %d", resp.ContentLength, N+1)
	}

	got, err := io.ReadAll(resp.Body)
	if err != nil && err != io.EOF {
		t.Fatalf("read: %v", err)
	}

	// Assert
	want := payload[:N+1]
	if !bytes.Equal(got, want) {
		t.Errorf("payload mismatch: got %d bytes, want %d", len(got), len(want))
	}
}

// TestRangeInitial_MidSpan_WithMultipleCuts_Resumes verifies a Range N..M with
// multiple failures is properly reassembled within the retry budget.
func TestRangeInitial_MidSpan_WithMultipleCuts_Resumes(t *testing.T) {
	// Arrange
	url := "https://example.com/range-n-m-cuts"
	payload := bytes.Repeat([]byte{0xAA, 0xBB, 0xCC, 0xDD}, 5000) // 20,000 bytes
	N := int64(1000)
	M := int64(9999)
	ft := newFakeTransport()
	ft.add(url, payload, &flakePlan{CutAfter: []int{300, 400, -1}}) // two cuts, then ok

	// Act
	client := newClient(ft, 5)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Range", "bytes="+strconv.FormatInt(N, 10)+"-"+strconv.FormatInt(M, 10))
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })

	// Assert Content-Length for 206 (N..M inclusive)
	wantCL := (M - N + 1)
	if resp.ContentLength != wantCL {
		t.Errorf("ContentLength = %d, want %d", resp.ContentLength, wantCL)
	}

	got, err := io.ReadAll(resp.Body)
	if err != nil && err != io.EOF {
		t.Fatalf("read: %v", err)
	}

	// Assert
	want := payload[N : M+1]
	if !bytes.Equal(got, want) {
		t.Errorf("payload mismatch: got %d bytes, want %d", len(got), len(want))
	}
}

// TestRangeInitial_FromNToEnd_WithCut_Resumes verifies a Range N..end request
// resumes correctly after a mid-stream failure and returns the tail of the object.
func TestRangeInitial_FromNToEnd_WithCut_Resumes(t *testing.T) {
	// Arrange
	url := "https://example.com/range-n-end-cut"
	payload := bytes.Repeat([]byte("tail"), 6000) // 24,000 bytes
	N := int64(7777)
	ft := newFakeTransport()
	ft.add(url, payload, &flakePlan{CutAfter: []int{1024, -1}})

	// Act
	client := newClient(ft, 3)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Range", "bytes="+strconv.FormatInt(N, 10)+"-")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })

	// Assert Content-Length for 206 (N..end inclusive)
	wantCL := int64(len(payload)) - N
	if resp.ContentLength != wantCL {
		t.Errorf("ContentLength = %d, want %d", resp.ContentLength, wantCL)
	}

	got, err := io.ReadAll(resp.Body)
	if err != nil && err != io.EOF {
		t.Fatalf("read: %v", err)
	}

	// Assert
	want := payload[N:]
	if !bytes.Equal(got, want) {
		t.Errorf("payload mismatch: got %d bytes, want %d", len(got), len(want))
	}
}

// TestRangeInitial_ResumeHeaderStart_Correct asserts that the resume request's
// Range header starts exactly at initialStart + bytesDelivered.
func TestRangeInitial_ResumeHeaderStart_Correct(t *testing.T) {
	// Arrange: Range 0..2047 with a cut after 512 bytes on initial segment.
	url := "https://example.com/range-header-check"
	payload := bytes.Repeat([]byte("H"), 4096)
	N := int64(2047)
	cut := 512
	ft := newFakeTransport()
	ft.add(url, payload, &flakePlan{CutAfter: []int{cut, -1}})

	client := newClient(ft, 3)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Range", "bytes=0-"+strconv.FormatInt(N, 10))

	// Act: perform the request and read to completion (forcing one resume).
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })

	// Assert Content-Length for 206 (0..N inclusive)
	if resp.ContentLength != (N + 1) {
		t.Errorf("ContentLength = %d, want %d", resp.ContentLength, N+1)
	}

	_, _ = io.ReadAll(resp.Body)

	// Assert: check second segment's Range header.
	hs := ft.segmentHeaders(url)
	if len(hs) < 2 {
		t.Fatalf("expected at least 2 segments (initial + resume), got %d", len(hs))
	}
	resumeRange := hs[1].Get("Range")
	want := "bytes=" + strconv.FormatInt(int64(cut), 10) + "-" + strconv.FormatInt(N, 10)
	if resumeRange != want {
		t.Errorf("resume Range header = %q, want %q", resumeRange, want)
	}
}

// ─────────────────────────────── Parser tests ───────────────────────────────

// TestParseSingleRange exercises valid and invalid single-range specs.
func TestParseSingleRange(t *testing.T) {
	cases := []struct {
		in         string
		start, end int64
		ok         bool
	}{
		{"", 0, -1, false},
		{"bytes=0-99", 0, 99, true},
		{"bytes=0-", 0, -1, true},
		{"bytes=5-5", 5, 5, true},
		{"BYTES=7-9", 7, 9, true},
		{"bytes=10-5", 0, -1, false}, // end before start
		{"bytes=-100", 0, -1, false}, // suffix not supported
		{"items=0-10", 0, -1, false},
		{"bytes=0-1,3-5", 0, -1, false}, // multi-range unsupported
	}
	for _, tc := range cases {
		start, end, ok := parseSingleRange(tc.in)
		if start != tc.start || end != tc.end || ok != tc.ok {
			t.Errorf("parseSingleRange(%q) = (%d,%d,%v), want (%d,%d,%v)", tc.in, start, end, ok, tc.start, tc.end, tc.ok)
		}
	}
}

// TestParseContentRange exercises valid and invalid Content-Range headers.
func TestParseContentRange(t *testing.T) {
	cases := []struct {
		in         string
		start, end int64
		total      int64
		ok         bool
	}{
		{"", 0, -1, -1, false},
		{"bytes 0-99/200", 0, 99, 200, true},
		{"BYTES 1-1/2", 1, 1, 2, true},
		{"bytes 0-0/*", 0, 0, -1, true},
		{"items 0-1/2", 0, -1, -1, false},
		{"bytes 0-99/abc", 0, -1, -1, false},
		{"bytes 5-4/10", 5, 4, 10, true}, // parser accepts; semantic check happens elsewhere
	}
	for _, tc := range cases {
		start, end, total, ok := parseContentRange(tc.in)
		if start != tc.start || end != tc.end || total != tc.total || ok != tc.ok {
			t.Errorf("parseContentRange(%q) = (%d,%d,%d,%v), want (%d,%d,%d,%v)", tc.in, start, end, total, ok, tc.start, tc.end, tc.total, tc.ok)
		}
	}
}
