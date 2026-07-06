package google_photo_native

import (
	"compress/gzip"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// newHTTPClientWithProxy mirrors gotohp's tuned client: idle-pool sized for
// concurrent uploads, no read timeout (context handles cancellation).
func newHTTPClientWithProxy(proxyURLStr string) (*http.Client, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig.InsecureSkipVerify = false

	transport.MaxIdleConns = 100
	transport.MaxIdleConnsPerHost = 10
	transport.MaxConnsPerHost = 10
	transport.IdleConnTimeout = 90 * time.Second

	if proxyURLStr != "" {
		proxyURL, err := url.Parse(proxyURLStr)
		if err != nil {
			return nil, err
		}
		transport.Proxy = http.ProxyURL(proxyURL)
		// ponytail: proxies (mitmproxy for debugging) often use self-signed
		// certs; skip verify only under an explicit proxy override.
		transport.TLSClientConfig.InsecureSkipVerify = true
	}

	return &http.Client{
		Transport: transport,
		// A generous cap so a hung TCP/TLS conn can't wedge a caller that
		// forgot to pass a context. Upload PUTs pass ctx via
		// http.NewRequestWithContext, and their deadline dominates this.
		Timeout: 10 * time.Minute,
	}, nil
}

type retryConfig struct {
	MaxRetries   int
	InitialDelay time.Duration
	MaxDelay     time.Duration
}

func defaultRetryConfig() retryConfig {
	return retryConfig{MaxRetries: 3, InitialDelay: time.Second, MaxDelay: 30 * time.Second}
}

func calculateBackoff(attempt int, cfg retryConfig) time.Duration {
	// Clamp shift so `1 << attempt` never wraps time.Duration to a
	// negative value even for absurd caller inputs.
	if attempt < 0 {
		attempt = 0
	}
	if attempt > 30 {
		attempt = 30
	}
	delay := cfg.InitialDelay * time.Duration(1<<uint(attempt))
	if delay <= 0 || delay > cfg.MaxDelay {
		delay = cfg.MaxDelay
	}
	// rand.Int63n panics on n<=0; ensure jitter has a positive upper
	// bound. Sub-10ns delays get no jitter, which is fine.
	jitterBound := int64(delay / 10)
	if jitterBound <= 0 {
		return delay
	}
	return delay + time.Duration(rand.Int63n(jitterBound))
}

func readResponseBody(resp *http.Response) ([]byte, error) {
	var reader io.Reader = resp.Body
	if strings.Contains(strings.ToLower(resp.Header.Get("Content-Encoding")), "gzip") {
		gz, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("gzip reader: %w", err)
		}
		defer func() { _ = gz.Close() }()
		reader = gz
	}
	return io.ReadAll(reader)
}
