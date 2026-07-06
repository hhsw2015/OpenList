package google_photo_native

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/OpenListTeam/OpenList/v4/drivers/google_photo_native/generated"
	"google.golang.org/protobuf/proto"
)

// GetUploadToken reserves an upload slot for a payload of size bytes with the
// given SHA-1 (base64 std-encoded). Returns the X-GUploader-UploadID which
// the subsequent PUT appends to the upload URL.
func (a *Api) GetUploadToken(shaHashB64 string, fileSize int64) (string, error) {
	protoBody := generated.GetUploadToken{
		F1:            2,
		F2:            2,
		F3:            1,
		F4:            3,
		FileSizeBytes: fileSize,
	}
	body, err := proto.Marshal(&protoBody)
	if err != nil {
		return "", fmt.Errorf("marshal GetUploadToken: %w", err)
	}
	bearer, err := a.BearerToken()
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", "https://photos.googleapis.com/data/upload/uploadmedia/interactive", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("Accept-Language", a.language)
	req.Header.Set("Content-Type", "application/x-protobuf")
	req.Header.Set("User-Agent", a.userAgent)
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("X-Goog-Hash", "sha1="+shaHashB64)
	req.Header.Set("X-Upload-Content-Length", strconv.FormatInt(fileSize, 10))

	resp, err := a.client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("GetUploadToken status %d: %s", resp.StatusCode, readErrorBody(resp))
	}
	tok := resp.Header.Get("X-GUploader-UploadID")
	if tok == "" {
		return "", errors.New("GetUploadToken: missing X-GUploader-UploadID")
	}
	return tok, nil
}

// FindRemoteMediaByHash looks up a media item by its raw SHA-1 bytes. Returns
// empty string (not error) when the account does not have a matching item.
func (a *Api) FindRemoteMediaByHash(sha []byte) (string, error) {
	protoBody := generated.HashCheck{
		Field1: &generated.HashCheckField1Type{
			Field1: &generated.HashCheckField1TypeField1Type{
				Sha1Hash: sha,
			},
			Field2: &generated.HashCheckField1TypeField2Type{},
		},
	}
	body, err := proto.Marshal(&protoBody)
	if err != nil {
		return "", fmt.Errorf("marshal HashCheck: %w", err)
	}
	respBytes, err := a.doProtobufPOST("https://photosdata-pa.googleapis.com/6439526531001121323/5084965799730810217", body)
	if err != nil {
		return "", err
	}
	var pbResp generated.RemoteMatches
	if err := proto.Unmarshal(respBytes, &pbResp); err != nil {
		return "", fmt.Errorf("unmarshal RemoteMatches: %w", err)
	}
	return pbResp.GetMediaKey(), nil
}

// UploadStream PUTs the whole reader as a single chunked-transfer upload.
// contentLength is the total size to advertise via X-Upload-Content-Length;
// pass -1 to send no length hint (chunked only). Retries on 5xx/network.
func (a *Api) UploadStream(ctx context.Context, uploadID string, reader io.Reader, contentLength int64) (*generated.CommitToken, error) {
	uploadURL := "https://photos.googleapis.com/data/upload/uploadmedia/interactive?upload_id=" + uploadID
	cfg := defaultRetryConfig()
	var lastErr error
	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(calculateBackoff(attempt-1, cfg)):
			}
		}
		result, err := a.doUploadRequest(ctx, uploadURL, reader, contentLength)
		if err == nil {
			return result, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		// A retry requires the reader to be re-seekable. If we cannot rewind
		// (payload is a raw stream), do not retry — the caller must handle
		// resumption at a higher level.
		if seeker, ok := reader.(io.Seeker); ok {
			if _, seekErr := seeker.Seek(0, io.SeekStart); seekErr != nil {
				return nil, fmt.Errorf("upload failed and reader is not rewindable: %w (rewind: %v)", err, seekErr)
			}
		} else {
			return nil, err
		}
	}
	return nil, fmt.Errorf("upload failed after %d attempts: %w", cfg.MaxRetries+1, lastErr)
}

func (a *Api) doUploadRequest(ctx context.Context, uploadURL string, reader io.Reader, contentLength int64) (*generated.CommitToken, error) {
	req, err := http.NewRequestWithContext(ctx, "PUT", uploadURL, reader)
	if err != nil {
		return nil, err
	}
	req.ContentLength = contentLength

	bearer, err := a.BearerToken()
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("Accept-Language", a.language)
	req.Header.Set("User-Agent", a.userAgent)
	req.Header.Set("Authorization", "Bearer "+bearer)

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("upload PUT status %d: %s", resp.StatusCode, readErrorBody(resp))
	}

	body, err := readResponseBody(resp)
	if err != nil {
		return nil, err
	}
	var out generated.CommitToken
	if err := proto.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("unmarshal CommitToken: %w", err)
	}
	return &out, nil
}

// CommitUpload finalizes an upload identified by the CommitToken returned
// from the PUT, attaches metadata (filename, mtime, quality), and returns
// the new mediaKey.
func (a *Api) CommitUpload(tok *generated.CommitToken, fileName string, sha1Hash []byte, uploadTimestamp int64, quality CommitQuality) (string, error) {
	if uploadTimestamp == 0 {
		uploadTimestamp = time.Now().Unix()
	}

	model := a.model
	userAgent := a.userAgent
	qualityVal := int64(3)
	switch quality {
	case QualitySaver:
		qualityVal = 1
		model = "Pixel 2"
		userAgent = buildUserAgent(a.clientVersionCode, a.language, model)
	case QualityQuota:
		model = "Pixel 8"
		userAgent = buildUserAgent(a.clientVersionCode, a.language, model)
	}

	protoBody := generated.CommitUpload{
		Field1: &generated.CommitUploadField1Type{
			Field1: &generated.CommitUploadField1TypeField1Type{
				Field1: tok.Field1,
				Field2: tok.Field2,
			},
			FileName: fileName,
			Sha1Hash: sha1Hash,
			Field4: &generated.CommitUploadField1TypeField4Type{
				FileLastModifiedTimestamp: uploadTimestamp,
				Field2:                    46000000,
			},
			Quality: qualityVal,
			Field10: 1,
		},
		Field2: &generated.CommitUploadField2Type{
			Model:             model,
			Make:              a.make,
			AndroidApiVersion: a.androidAPIVersion,
		},
		Field3: []byte{1, 3},
	}

	body, err := proto.Marshal(&protoBody)
	if err != nil {
		return "", fmt.Errorf("marshal CommitUpload: %w", err)
	}

	cfg := defaultRetryConfig()
	var lastErr error
	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(calculateBackoff(attempt-1, cfg))
		}
		key, err := a.doCommitRequest(body, userAgent)
		if err == nil {
			return key, nil
		}
		lastErr = err
		// Deterministic parse failures should not be retried.
		if strings.Contains(err.Error(), "invalid response structure") {
			break
		}
	}
	return "", fmt.Errorf("commit failed after %d attempts: %w", cfg.MaxRetries+1, lastErr)
}

func (a *Api) doCommitRequest(body []byte, userAgent string) (string, error) {
	bearer, err := a.BearerToken()
	if err != nil {
		return "", err
	}
	req, err := http.NewRequest("POST",
		"https://photosdata-pa.googleapis.com/6439526531001121323/16538846908252377752",
		bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("accept-Encoding", "gzip")
	req.Header.Set("accept-Language", a.language)
	req.Header.Set("content-Type", "application/x-protobuf")
	req.Header.Set("user-Agent", userAgent)
	req.Header.Set("authorization", "Bearer "+bearer)
	req.Header.Set("x-goog-ext-173412678-bin", "CgcIAhClARgC")
	req.Header.Set("x-goog-ext-174067345-bin", "CgIIAg==")

	resp, err := a.client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("commit status %d: %s", resp.StatusCode, readErrorBody(resp))
	}
	respBody, err := readResponseBody(resp)
	if err != nil {
		return "", err
	}
	var pbResp generated.CommitUploadResponse
	if err := proto.Unmarshal(respBody, &pbResp); err != nil {
		return "", fmt.Errorf("unmarshal CommitUploadResponse: %w", err)
	}
	if pbResp.GetField1() == nil || pbResp.GetField1().GetField3() == nil {
		return "", errors.New("commit: invalid response structure (upload rejected by Photos)")
	}
	mediaKey := pbResp.GetField1().GetField3().GetMediaKey()
	if mediaKey == "" {
		return "", errors.New("commit: no media key returned")
	}
	return mediaKey, nil
}

// CommitQuality selects the quality tier and matching device fingerprint
// used during CommitUpload.
type CommitQuality int

const (
	// QualityOriginal keeps the payload as uploaded. Combined with disguise
	// this is the free-unlimited path.
	QualityOriginal CommitQuality = iota
	// QualitySaver forces "Storage saver" mode.
	QualitySaver
	// QualityQuota is a debug knob that reports Pixel 8 fingerprint,
	// which the API treats as quota-counting.
	QualityQuota
)
