package google_photo_native

import (
	"fmt"
	"net/http"
)

// ThumbnailURL returns the signed URL for a thumbnail of the given media,
// authenticated with a fresh bearer token. Callers can hand this URL to a
// client that will send its own request, or use FetchThumbnail below to
// pull the bytes directly.
//
// Size hints are approximate: width and height are advisory to Google's
// image server. Passing 0 lets Google choose.
func (a *Api) ThumbnailURL(mediaKey string, width, height int) string {
	url := fmt.Sprintf("https://ap2.googleusercontent.com/gpa/%s=k-sg", mediaKey)
	if width > 0 {
		url += fmt.Sprintf("-w%d", width)
	}
	if height > 0 {
		url += fmt.Sprintf("-h%d", height)
	}
	return url
}

// FetchThumbnail downloads and returns the raw thumbnail bytes.
func (a *Api) FetchThumbnail(mediaKey string, width, height int) ([]byte, error) {
	bearer, err := a.BearerToken()
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("GET", a.ThumbnailURL(mediaKey, width, height), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("User-Agent", a.userAgent)
	req.Header.Set("Accept-Encoding", "gzip")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("thumbnail status %d: %s", resp.StatusCode, readErrorBody(resp))
	}
	return readResponseBody(resp)
}
