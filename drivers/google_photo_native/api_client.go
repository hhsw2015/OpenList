package google_photo_native

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Api is the wire client for Google Photos' internal Android endpoints.
// One Api instance per storage. All fields except client are guarded by mu.
type Api struct {
	client   *http.Client
	authData string
	Email    string

	androidAPIVersion int64
	model             string
	make              string
	clientVersionCode int64
	language          string
	userAgent         string

	mu           sync.Mutex
	bearerExpiry int64 // unix seconds
	bearerToken  string
}

const (
	apiAndroidAPIVersion = 28
	apiModel             = "Pixel XL"
	apiMake              = "Google"
	apiClientVersion     = 49029607
	apiBuild             = "PQ2A.190205.001"
	apiCronet            = "127.0.6510.5"
)

func newApi(authData, email, language string, client *http.Client) *Api {
	a := &Api{
		client:            client,
		authData:          strings.TrimSpace(authData),
		Email:             email,
		androidAPIVersion: apiAndroidAPIVersion,
		model:             apiModel,
		make:              apiMake,
		clientVersionCode: apiClientVersion,
		language:          language,
	}
	a.userAgent = buildUserAgent(a.clientVersionCode, a.language, a.model)
	return a
}

func buildUserAgent(clientVersionCode int64, language, model string) string {
	return fmt.Sprintf(
		"com.google.android.apps.photos/%d (Linux; U; Android 9; %s; %s; Build/%s; Cronet/%s) (gzip)",
		clientVersionCode, language, model, apiBuild, apiCronet,
	)
}

// BearerToken returns a live short-lived bearer, minting a fresh one if the
// cached copy is within 30 s of expiry. Serialized behind mu so racing
// requests coalesce onto one /auth refresh.
func (a *Api) BearerToken() (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.bearerToken != "" && a.bearerExpiry > time.Now().Unix() {
		return a.bearerToken, nil
	}

	resp, err := a.refreshBearerLocked()
	if err != nil {
		return "", err
	}
	auth := resp["Auth"]
	if auth == "" {
		return "", errors.New("auth response missing Auth token")
	}
	expiryStr := resp["Expiry"]
	expirySeconds, err := strconv.ParseInt(expiryStr, 10, 64)
	if err != nil {
		return "", fmt.Errorf("invalid expiry: %w", err)
	}
	now := time.Now().Unix()
	// Google returns the expiry as a relative duration in some builds.
	if expirySeconds < now {
		expirySeconds = now + expirySeconds
	}
	// Refresh 30 s early to keep in-flight requests from tripping the wall.
	expirySeconds -= 30
	if expirySeconds < now {
		expirySeconds = now
	}
	a.bearerToken = auth
	a.bearerExpiry = expirySeconds
	return auth, nil
}

func (a *Api) refreshBearerLocked() (map[string]string, error) {
	values, err := url.ParseQuery(a.authData)
	if err != nil {
		return nil, fmt.Errorf("parse auth data: %w", err)
	}
	values.Set("app", "com.google.android.apps.photos")
	values.Set("callerPkg", "com.google.android.apps.photos")
	values.Del("it_caveat_types")
	values.Del("assertion_jwt")
	values.Del("token_binding_alias") // v1 does not support token binding

	headers := map[string]string{
		"Accept-Encoding": "gzip",
		"app":             "com.google.android.apps.photos",
		"Connection":      "Keep-Alive",
		"Content-Type":    "application/x-www-form-urlencoded",
		"device":          values.Get("androidId"),
		"User-Agent":      "GoogleAuth/1.4 (Pixel XL PQ2A.190205.001); gzip",
	}

	req, err := http.NewRequest("POST", "https://android.googleapis.com/auth", strings.NewReader(values.Encode()))
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("auth request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := readResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("read auth body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("auth status %d: %s", resp.StatusCode, string(body))
	}

	out := make(map[string]string, 16)
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			continue
		}
		out[line[:eq]] = line[eq+1:]
	}
	return out, nil
}

// doProtobufPOST is a shared wrapper for authenticated protobuf endpoints.
func (a *Api) doProtobufPOST(endpoint string, requestData []byte) ([]byte, error) {
	bearer, err := a.BearerToken()
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", endpoint, bytes.NewReader(requestData))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("Accept-Language", a.language)
	req.Header.Set("Content-Type", "application/x-protobuf")
	req.Header.Set("User-Agent", a.userAgent)
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("x-goog-ext-173412678-bin", "CgcIAhClARgC")
	req.Header.Set("x-goog-ext-174067345-bin", "CgIIAg==")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body := readErrorBody(resp)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, body)
	}
	return readResponseBody(resp)
}

func readErrorBody(resp *http.Response) string {
	var reader io.Reader = resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gz, err := gzip.NewReader(resp.Body)
		if err == nil {
			defer func() { _ = gz.Close() }()
			reader = gz
		}
	}
	b, _ := io.ReadAll(reader)
	return string(b)
}
