package google_photo_native

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/OpenListTeam/OpenList/v4/pkg/http_range"
)

// disguiseInfo caches the parsed trailer for a signed URL so subsequent range
// reads for the same file skip the head fetch.
type disguiseInfo struct {
	origName     string
	payloadStart int64
	payloadSize  int64
	isDisguised  bool // false = real media, byte-for-byte proxy
}

// disguiseRangeReader implements model.RangeReaderIF. On first call it fetches
// a 66 KB head from the signed URL and inspects it for the disguise magic. If
// present, subsequent reads are byte-shifted into the payload region; if
// absent the URL is proxied verbatim (real MP4 or other media).
type disguiseRangeReader struct {
	url         string
	totalSize   int64 // wire size (Google's stored total)
	client      *http.Client
	mu          sync.Mutex
	info        *disguiseInfo
	inspectOnce sync.Once
	inspectErr  error
}

func newDisguiseRangeReader(url string, totalSize int64, client *http.Client) *disguiseRangeReader {
	return &disguiseRangeReader{url: url, totalSize: totalSize, client: client}
}

func (r *disguiseRangeReader) inspect(ctx context.Context) error {
	r.inspectOnce.Do(func() {
		buf, wireTotal, err := r.fetchHeadWithSize(ctx)
		if err != nil {
			// Preserve the error — callers must be able to tell a network
			// failure ("head fetch failed, can't judge") from a legitimate
			// "not disguised" outcome, otherwise disguised payloads would
			// silently corrupt on transient errors.
			r.inspectErr = fmt.Errorf("disguise inspect: %w", err)
			return
		}
		if wireTotal > 0 {
			r.mu.Lock()
			r.totalSize = wireTotal
			r.mu.Unlock()
		}
		name, start, ok, parseErr := ParseDisguiseHead(buf)
		if parseErr != nil {
			// Structurally invalid trailer — treat as non-disguised. The
			// bytes are valid media as far as we know; the parser flag
			// only guards against escaping outDir on extraction.
			r.info = &disguiseInfo{isDisguised: false}
			return
		}
		if !ok {
			r.info = &disguiseInfo{isDisguised: false}
			return
		}
		r.mu.Lock()
		r.info = &disguiseInfo{
			origName:     name,
			payloadStart: start,
			payloadSize:  r.totalSize - start,
			isDisguised:  true,
		}
		r.mu.Unlock()
	})
	return r.inspectErr
}

// fetchHeadWithSize does the initial head read and returns the parsed head
// bytes plus the total wire size learned from Content-Length /
// Content-Range. wireTotal == 0 signals "unknown; caller keeps the size it
// already had".
func (r *disguiseRangeReader) fetchHeadWithSize(ctx context.Context) (head []byte, wireTotal int64, err error) {
	req, err := http.NewRequestWithContext(ctx, "GET", r.url, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=0-%d", DisguiseHeadWindow()-1))

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 || resp.StatusCode < 200 {
		return nil, 0, fmt.Errorf("head GET status %d", resp.StatusCode)
	}
	// Prefer Content-Range's `/<total>` suffix when the server honored our
	// Range (206). Fall back to Content-Length if the server returned 200
	// with the full body.
	if cr := resp.Header.Get("Content-Range"); cr != "" {
		if slash := lastSlash(cr); slash >= 0 && slash+1 < len(cr) {
			var n int64
			if _, err := fmt.Sscanf(cr[slash+1:], "%d", &n); err == nil {
				wireTotal = n
			}
		}
	}
	if wireTotal == 0 {
		if cl := resp.ContentLength; cl > 0 {
			wireTotal = cl
		}
	}
	head, err = io.ReadAll(io.LimitReader(resp.Body, DisguiseHeadWindow()))
	if err != nil {
		return nil, 0, fmt.Errorf("head read: %w", err)
	}
	return head, wireTotal, nil
}

func lastSlash(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '/' {
			return i
		}
	}
	return -1
}

// PayloadSize returns the on-disk payload size after stripping the disguise
// wrapper. Callers must have called inspect() (or a prior RangeRead) first.
// If the file is not disguised, returns the total wire size.
func (r *disguiseRangeReader) PayloadSize() int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.info == nil {
		return r.totalSize
	}
	if !r.info.isDisguised {
		return r.totalSize
	}
	return r.info.payloadSize
}

// IsDisguised is meaningful after inspect().
func (r *disguiseRangeReader) IsDisguised() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.info != nil && r.info.isDisguised
}

func (r *disguiseRangeReader) RangeRead(ctx context.Context, hr http_range.Range) (io.ReadCloser, error) {
	if err := r.inspect(ctx); err != nil {
		return nil, err
	}
	r.mu.Lock()
	info := r.info
	r.mu.Unlock()

	upstreamStart := hr.Start
	upstreamLen := hr.Length
	if info != nil && info.isDisguised {
		upstreamStart = info.payloadStart + hr.Start
		// hr.Length == -1 means "to end of file"; keep it that way upstream.
	}
	return r.fetchRange(ctx, upstreamStart, upstreamLen)
}

// fetchRange issues one signed-URL GET with a Range header. Length -1 means
// unbounded.
func (r *disguiseRangeReader) fetchRange(ctx context.Context, start, length int64) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", r.url, nil)
	if err != nil {
		return nil, err
	}
	if length < 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", start))
	} else {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, start+length-1))
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 || resp.StatusCode < 200 {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("range GET %s status %d", req.Header.Get("Range"), resp.StatusCode)
	}
	// If the server ignored our Range header (200 instead of 206), we still
	// received the full wire body. Wrap the reader so downstream code sees
	// only the requested slice starting at `start` for `length` bytes.
	if resp.StatusCode == 200 && (start > 0 || length > 0) {
		return &shiftedReader{
			rc:     resp.Body,
			skip:   start,
			remain: length, // -1 = to end
		}, nil
	}
	return resp.Body, nil
}

// shiftedReader emulates a satisfied Range request by discarding `skip`
// leading bytes from the underlying stream and, if `remain >= 0`, limiting
// the visible length to `remain`. Used when Google's signed URL returns
// 200 OK with the full body instead of honoring our Range header.
//
// ponytail: this downloads the full wire body on every range request. For
// GB-scale disguised payloads with many seeks this is painful. Upgrade to
// per-Link spool-to-disk when profiling proves it matters.
type shiftedReader struct {
	rc      io.ReadCloser
	skip    int64
	remain  int64 // -1 = unbounded
	skipped bool
}

func (s *shiftedReader) Read(p []byte) (int, error) {
	if !s.skipped {
		if _, err := io.CopyN(io.Discard, s.rc, s.skip); err != nil {
			return 0, err
		}
		s.skipped = true
	}
	if s.remain == 0 {
		return 0, io.EOF
	}
	buf := p
	if s.remain > 0 && int64(len(buf)) > s.remain {
		buf = buf[:s.remain]
	}
	n, err := s.rc.Read(buf)
	if s.remain > 0 {
		s.remain -= int64(n)
	}
	return n, err
}

func (s *shiftedReader) Close() error {
	return s.rc.Close()
}
