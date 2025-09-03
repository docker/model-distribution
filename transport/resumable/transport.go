// Package resumable provides an http.RoundTripper that transparently resumes
// interrupted GET responses from servers that support byte ranges.
//
// ───────────────────────────── How it works ─────────────────────────────
//   - For GET responses with status 200 or 206 and "Accept-Ranges: bytes",
//     the transport replaces resp.Body with a resumable reader.
//   - If a mid-stream read fails (e.g., connection cut), it issues a follow-up
//     request with a "Range" header to continue from the last delivered byte.
//     It uses ETag (strong only) or Last-Modified via If-Range for safety.
//   - If the server doesn’t support ranges (or for non-GET), it passes
//     through the response unmodified.
//
// ───────────────────────────── Notes & caveats ───────────────────────────
//   - Only single byte ranges are supported when the original request already
//     includes Range (multi-range requests are passed through without resuming).
//   - Auto-decompression must not be active, or offsets won’t line up. If the
//     initial response was transparently decompressed (resp.Uncompressed == true)
//     or Content-Encoding was set, resumption is disabled for that response.
//   - Cookies added by an http.Client Jar after the initial response aren’t
//     automatically applied to follow-up range requests (since they bypass
//     http.Client). Existing request headers (incl. Cookie, Authorization, etc.)
//     are preserved, but Set-Cookie from the initial response won't be consulted.
//   - Some servers don’t advertise Accept-Ranges but still support Range.
//     This implementation requires explicit "Accept-Ranges: bytes" for safety.
package resumable

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Option configures a ResumableTransport.
type Option func(*ResumableTransport)

// WithMaxRetries sets the maximum number of resume attempts after an error.
// Default: 3.
func WithMaxRetries(n int) Option {
	return func(rt *ResumableTransport) { rt.maxRetries = n }
}

// BackoffFunc computes the sleep duration for a given retry attempt (0-based).
type BackoffFunc func(attempt int) time.Duration

// WithBackoff sets the backoff strategy for resume attempts.
// Default: jittered exponential starting at 200ms, capped at 5s.
func WithBackoff(f BackoffFunc) Option {
	return func(rt *ResumableTransport) { rt.backoff = f }
}

// ResumableTransport wraps another http.RoundTripper and transparently retries
// mid-stream failures for GET requests against servers that support range requests.
type ResumableTransport struct {
	// base is the underlying RoundTripper actually used to send requests.
	base http.RoundTripper
	// maxRetries is the maximum number of resume attempts that will be made
	// after a read error before giving up.
	maxRetries int
	// backoff computes how long to wait before each retry attempt.
	// Called with the total number of attempts made so far (0-based).
	backoff BackoffFunc
}

// New returns a ResumableTransport wrapping base. If base is nil,
// http.DefaultTransport is used. Options configure retries/backoff.
func New(base http.RoundTripper, opts ...Option) *ResumableTransport {
	if base == nil {
		base = http.DefaultTransport
	}
	rt := &ResumableTransport{
		base:       base,
		maxRetries: 3,
		backoff: func(i int) time.Duration {
			// 200ms * 2^i with ±20% jitter, capped at 5s
			base := 200 * time.Millisecond
			d := time.Duration(float64(base) * math.Pow(2, float64(i)))
			if d > 5*time.Second {
				d = 5 * time.Second
			}
			j := 0.2 + rand.Float64()*0.4 // [0.2,0.6)
			return time.Duration(float64(d) * j)
		},
	}
	for _, o := range opts {
		o(rt)
	}
	return rt
}

// RoundTrip implements http.RoundTripper. It wraps GET requests that return
// 200/206 responses with "Accept-Ranges: bytes" support in a resumable body.
func (rt *ResumableTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Always use the base transport to perform the initial request.
	resp, err := rt.base.RoundTrip(req)
	if resp == nil || err != nil {
		return resp, err
	}

	// If the request doesn't meet our resumability criteria, then return it
	// directly.
	if !isResumable(req, resp) {
		return resp, nil
	}

	// Create a resumable body to perform retries if needed.
	rb := newResumableBody(req, resp, rt)
	resp.Body = rb
	if n, ok := rb.plannedLength(); ok {
		resp.ContentLength = n
	} else {
		resp.ContentLength = -1
	}
	return resp, nil
}

// isResumable checks if the pair (request, response) is eligible for resume.
func isResumable(req *http.Request, resp *http.Response) bool {
	if req.Method != http.MethodGet {
		return false
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return false
	}
	if !supportsRange(resp.Header) {
		return false
	}
	// Disallow when the response was auto-decompressed or has a Content-Encoding.
	if resp.Uncompressed || resp.Header.Get("Content-Encoding") != "" {
		return false
	}
	return true
}

// supportsRange determines whether or not an HTTP response indicates support
// for range requests.
func supportsRange(h http.Header) bool {
	ar := strings.ToLower(h.Get("Accept-Ranges"))
	for _, part := range strings.Split(ar, ",") {
		if strings.TrimSpace(part) == "bytes" {
			return true
		}
	}
	return false
}

// resumableBody wraps a Response.Body to add transparent resume support.
// It keeps track of how many bytes have been delivered and re-issues
// Range requests starting from that offset when a read fails.
type resumableBody struct {
	// mu guards access to all fields below.
	mu sync.Mutex
	// ctx is the request context; canceled if caller cancels.
	ctx context.Context
	// tr is the owning ResumableTransport (for retry/backoff settings).
	tr *ResumableTransport
	// base is the underlying RoundTripper to use for follow-up Range requests.
	base http.RoundTripper
	// origReq is the original *http.Request; used as a template for retries.
	origReq *http.Request
	// current is the most recent *http.Response from which we are reading.
	current *http.Response
	// rc is the active body we are currently reading from.
	rc io.ReadCloser
	// bytesRead is how many bytes we have successfully delivered to the caller
	// (relative to initialStart).
	bytesRead int64
	// initialStart is the starting offset of the stream on the wire, usually 0.
	initialStart int64
	// initialEnd is the inclusive end offset (if known from Range header).
	initialEnd *int64
	// totalSize is the total known length of the resource, if available.
	totalSize *int64
	// etag is the validator used for If-Range in resumed requests (preferred).
	etag string
	// lastModified is a fallback validator for If-Range if no ETag is present.
	lastModified string
	// retriesUsed counts how many resume attempts we have made so far.
	retriesUsed int
	// originalRangeSpec is the Range header sent on the initial request.
	originalRangeSpec string
	// done marks that we’ve finished delivering all bytes (EOF).
	done bool
}

// newResumableBody constructs a resumableBody from the initial response.
func newResumableBody(req *http.Request, resp *http.Response, tr *ResumableTransport) *resumableBody {
	rb := &resumableBody{
		ctx:               req.Context(),
		tr:                tr,
		base:              tr.base,
		origReq:           req,
		current:           resp,
		rc:                resp.Body,
		originalRangeSpec: req.Header.Get("Range"),
	}

	// Extract starting offsets from request Range if present (single-range only).
	if start, end, ok := parseSingleRange(rb.originalRangeSpec); ok {
		rb.initialStart = start
		if end >= 0 {
			rb.initialEnd = &end
		}
	}

	// Refine offsets from Content-Range header if response was 206.
	if resp.StatusCode == http.StatusPartialContent {
		if s, e, total, ok := parseContentRange(resp.Header.Get("Content-Range")); ok {
			rb.initialStart = s
			if e >= 0 {
				rb.initialEnd = &e
			}
			if total >= 0 {
				rb.totalSize = &total
			}
		}
	} else if resp.ContentLength >= 0 { // 200 OK
		total := int64(resp.ContentLength)
		rb.totalSize = &total
	}

	// Capture validators for If-Range to ensure consistency across resumes.
	if et := resp.Header.Get("ETag"); et != "" && !isWeakETag(et) {
		rb.etag = et
	} else if lm := resp.Header.Get("Last-Modified"); lm != "" {
		rb.lastModified = lm
	}
	return rb
}

// Read delivers bytes to the caller. If an error occurs mid-stream, it will
// transparently try to resume by issuing a new Range request.
func (rb *resumableBody) Read(p []byte) (int, error) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if rb.done {
		return 0, io.EOF
	}
	if rb.rc == nil {
		// No active body — must resume from the last delivered offset.
		if err := rb.resume(rb.bytesRead); err != nil {
			return 0, err
		}
	}

	n, err := rb.rc.Read(p)
	rb.bytesRead += int64(n)

	switch {
	case err == nil:
		return n, nil
	case errors.Is(err, io.EOF):
		rb.done = true
		return n, io.EOF
	default:
		// Underlying read failed mid-stream. Try to resume.
		_ = rb.rc.Close()
		rb.rc = nil

		if n > 0 {
			// Surface bytes already read; the caller will call Read again.
			return n, nil
		}
		if rb.retriesUsed >= rb.tr.maxRetries {
			return 0, err
		}
		if rerr := rb.resume(rb.bytesRead); rerr != nil {
			return 0, rerr
		}

		n2, err2 := rb.rc.Read(p)
		rb.bytesRead += int64(n2)
		if err2 == nil {
			return n2, nil
		}
		if errors.Is(err2, io.EOF) {
			rb.done = true
		}
		return n2, err2
	}
}

// Close closes the current response body if present.
func (rb *resumableBody) Close() error {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	if rb.rc != nil {
		return rb.rc.Close()
	}
	return nil
}

// plannedLength returns the exact number of bytes this resumableBody intends to produce, if knowable.
func (rb *resumableBody) plannedLength() (int64, bool) {
	if rb.initialEnd != nil {
		return *rb.initialEnd - rb.initialStart + 1, true
	}
	if rb.current.StatusCode == http.StatusOK && rb.totalSize != nil {
		// 200 OK with known total size at start-of-stream.
		return *rb.totalSize, true
	}
	return 0, false
}

// resume attempts to resume the response stream at the given absolute offset
// (relative to the very first byte on the wire). The method will make up to the
// remaining retry budget attempts. On success it swaps rb.rc with a fresh body.
func (rb *resumableBody) resume(absoluteOffset int64) error {
	remaining := rb.tr.maxRetries - rb.retriesUsed
	for attempt := 0; attempt < remaining; attempt++ {
		if err := rb.ctx.Err(); err != nil {
			return err
		}

		start := rb.initialStart + absoluteOffset
		rangeVal := buildRangeHeader(start, rb.initialEnd)
		req := rb.cloneBaseRequest(rangeVal)

		// Backoff for subsequent attempts.
		if attempt > 0 || rb.retriesUsed > 0 {
			if err := waitBackoff(rb.ctx, rb.tr.backoff, rb.retriesUsed+attempt); err != nil {
				return err
			}
		}

		resp, err := rb.base.RoundTrip(req)
		if err != nil {
			continue // try again within budget
		}

		switch resp.StatusCode {
		case http.StatusPartialContent:
			// Validate server honored our starting offset precisely.
			s, _, _, ok := parseContentRange(resp.Header.Get("Content-Range"))
			if !ok || s != start {
				_ = resp.Body.Close()
				continue // try again; mismatched range
			}
			rb.swapResponse(resp)
			rb.retriesUsed++
			return nil

		case http.StatusOK:
			// If we requested a range but got a full response, it likely means the
			// validator failed (resource changed) or the server ignored Range.
			_ = resp.Body.Close()
			return fmt.Errorf("resumable: server returned 200 to a range request; resource may have changed")

		case http.StatusRequestedRangeNotSatisfiable:
			// If we've already read to/ past the expected end, we are actually done.
			if rb.rangeIsComplete(absoluteOffset) {
				rb.done = true
				_ = resp.Body.Close()
				return io.EOF
			}
			_ = resp.Body.Close()

		default:
			_ = resp.Body.Close()
		}
	}
	return fmt.Errorf("resumable: exceeded retry budget after %d attempts", rb.tr.maxRetries)
}

// swapResponse replaces the current response body with a new one
// from a resumed request, and updates any validators and size info.
func (rb *resumableBody) swapResponse(resp *http.Response) {
	if rb.rc != nil && rb.rc != resp.Body {
		_ = rb.rc.Close()
	}
	rb.current = resp
	rb.rc = resp.Body

	// Persist validators from the server if they are strong.
	if et := resp.Header.Get("ETag"); et != "" && !isWeakETag(et) {
		rb.etag = et
	}
	if lm := resp.Header.Get("Last-Modified"); lm != "" {
		rb.lastModified = lm
	}

	// Merge any updated size info from the Content-Range.
	if s, e, total, ok := parseContentRange(resp.Header.Get("Content-Range")); ok {
		_ = s // start is validated by caller
		if e >= 0 {
			rb.initialEnd = &e
		}
		if total >= 0 {
			rb.totalSize = &total
		}
	}
}

// cloneBaseRequest builds a new GET request with the same headers as the original,
// except with a different Range and If-Range validator. To avoid mismatched
// encodings, we also force identity encoding.
func (rb *resumableBody) cloneBaseRequest(rangeVal string) *http.Request {
	req := rb.origReq.Clone(rb.ctx)
	req.Body = nil
	req.ContentLength = 0
	req.Header = cloneHeader(rb.origReq.Header)

	// Ensure we control the Range validator set.
	req.Header.Set("Range", rangeVal)
	// Remove conditional headers that could conflict with If-Range semantics.
	scrubConditionalHeaders(req.Header)

	if rb.etag != "" {
		req.Header.Set("If-Range", rb.etag)
	} else if rb.lastModified != "" {
		req.Header.Set("If-Range", rb.lastModified)
	} else {
		// If no validator, we still attempt Range but risk a 200 if server can't verify.
	}

	// Prevent transparent decompression on resumed requests.
	req.Header.Set("Accept-Encoding", "identity")
	return req
}

// cloneHeader makes a deep copy of an http.Header map.
func cloneHeader(h http.Header) http.Header {
	out := make(http.Header, len(h))
	for k, vv := range h {
		cp := make([]string, len(vv))
		copy(cp, vv)
		out[k] = cp
	}
	return out
}

// scrubConditionalHeaders removes conditional headers we do not want to forward
// on resumed Range requests, because they can alter semantics or conflict with
// If-Range logic.
func scrubConditionalHeaders(h http.Header) {
	h.Del("If-None-Match")
	h.Del("If-Modified-Since")
	h.Del("If-Match")
	h.Del("If-Unmodified-Since")
	// We overwrite Range/If-Range explicitly elsewhere.
}

// buildRangeHeader constructs a "Range" header value for a given start and
// optional inclusive end.
func buildRangeHeader(start int64, end *int64) string {
	if end == nil {
		return fmt.Sprintf("bytes=%d-", start)
	}
	return fmt.Sprintf("bytes=%d-%d", start, *end)
}

// waitBackoff sleeps using the provided backoff function, unless the context
// is canceled.
func waitBackoff(ctx context.Context, bf BackoffFunc, attempt int) error {
	d := time.Duration(0)
	if bf != nil {
		d = bf(attempt)
	}
	if d <= 0 {
		return nil
	}
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// rangeIsComplete returns true if the bytes we have delivered already meet the
// expected end of the range / resource, so a 416 implies we are done.
func (rb *resumableBody) rangeIsComplete(absoluteOffset int64) bool {
	if rb.totalSize != nil {
		// If we know total size, we are complete if start+offset >= total.
		if rb.initialStart+absoluteOffset >= *rb.totalSize {
			return true
		}
	}
	if rb.initialEnd != nil {
		// initialEnd is inclusive.
		if rb.initialStart+absoluteOffset >= *rb.initialEnd+1 {
			return true
		}
	}
	return false
}

// isWeakETag reports whether the ETag is a weak validator (W/"...") which must
// not be used with If-Range per RFC 7232 §2.1.
func isWeakETag(etag string) bool {
	etag = strings.TrimSpace(etag)
	return strings.HasPrefix(etag, "W/") || strings.HasPrefix(etag, "w/")
}

// ─────────────────────────── Helpers: header parsing ──────────────────────────

// parseSingleRange parses a single "Range: bytes=start-end" header.
// It returns (start, end, ok). When end is omitted, end == -1.
//
// Notes:
//   - Only absolute-start forms are supported (no suffix ranges "-N").
//   - Multi-range specifications (comma separated) return ok == false.
func parseSingleRange(h string) (int64, int64, bool) {
	if h == "" {
		return 0, -1, false
	}
	h = strings.TrimSpace(h)
	if !strings.HasPrefix(strings.ToLower(h), "bytes=") {
		return 0, -1, false
	}
	spec := strings.TrimSpace(h[len("bytes="):])
	if strings.Contains(spec, ",") {
		return 0, -1, false
	}
	parts := strings.SplitN(spec, "-", 2)
	if len(parts) != 2 {
		return 0, -1, false
	}
	if parts[0] == "" {
		// Suffix form is not supported here.
		return 0, -1, false
	}
	start, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
	if err != nil || start < 0 {
		return 0, -1, false
	}
	end := int64(-1)
	if strings.TrimSpace(parts[1]) != "" {
		e, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
		if err != nil || e < start {
			return 0, -1, false
		}
		end = e
	}
	return start, end, true
}

// parseContentRange parses "Content-Range: bytes start-end/total".
// It returns (start, end, total, ok). When total is unknown, total == -1.
func parseContentRange(h string) (int64, int64, int64, bool) {
	if h == "" {
		return 0, -1, -1, false
	}
	h = strings.ToLower(strings.TrimSpace(h))
	if !strings.HasPrefix(h, "bytes ") {
		return 0, -1, -1, false
	}
	body := strings.TrimSpace(h[len("bytes "):])
	seTotal := strings.SplitN(body, "/", 2)
	if len(seTotal) != 2 {
		return 0, -1, -1, false
	}
	se := strings.SplitN(strings.TrimSpace(seTotal[0]), "-", 2)
	if len(se) != 2 {
		return 0, -1, -1, false
	}
	start, err1 := strconv.ParseInt(strings.TrimSpace(se[0]), 10, 64)
	end, err2 := strconv.ParseInt(strings.TrimSpace(se[1]), 10, 64)
	totalStr := strings.TrimSpace(seTotal[1])
	var total int64 = -1
	var err3 error
	if totalStr != "*" {
		total, err3 = strconv.ParseInt(totalStr, 10, 64)
	}
	if err1 != nil || err2 != nil || (err3 != nil && totalStr != "*") {
		return 0, -1, -1, false
	}
	return start, end, total, true
}
