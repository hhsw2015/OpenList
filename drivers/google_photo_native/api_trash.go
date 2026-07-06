package google_photo_native

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"
)

const moveToTrashEndpoint = "https://photosdata-pa.googleapis.com/6439526531001121323/17490284929287180316"

// MoveToTrash soft-deletes media items. The parameter is misleadingly called
// dedupKeys upstream, but the field carries mediaKey values (verified in
// gotohp source comments).
func (a *Api) MoveToTrash(mediaKeys []string) error {
	keys := cleanKeys(mediaKeys)
	if len(keys) == 0 {
		return fmt.Errorf("no valid keys provided")
	}
	body := buildMoveToTrashRequest(keys, a.clientVersionCode, a.androidAPIVersion)
	return a.doTrashRequest(body)
}

// PermanentlyDelete removes items permanently by dedup key (field 2.21.1
// from a list response).
func (a *Api) PermanentlyDelete(dedupKeys []string) error {
	keys := cleanKeys(dedupKeys)
	if len(keys) == 0 {
		return fmt.Errorf("no valid keys provided")
	}
	body := buildPermanentlyDeleteRequest(keys)
	return a.doTrashRequest(body)
}

func cleanKeys(in []string) []string {
	out := make([]string, 0, len(in))
	for _, k := range in {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		out = append(out, k)
	}
	return out
}

func (a *Api) doTrashRequest(body []byte) error {
	bearer, err := a.BearerToken()
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", moveToTrashEndpoint, bytes.NewReader(body))
	if err != nil {
		return err
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
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("trash status %d: %s", resp.StatusCode, readErrorBody(resp))
	}
	return nil
}

func buildMoveToTrashRequest(mediaKeys []string, clientVersionCode, androidAPIVersion int64) []byte {
	var buf bytes.Buffer
	writeProtobufVarint(&buf, 2, 1)
	for _, k := range mediaKeys {
		writeProtobufString(&buf, 3, k)
	}
	writeProtobufVarint(&buf, 4, 1)

	var field8 bytes.Buffer
	var field8_4 bytes.Buffer
	writeProtobufField(&field8_4, 2, []byte{})
	var field8_4_3 bytes.Buffer
	writeProtobufField(&field8_4_3, 1, []byte{})
	writeProtobufField(&field8_4, 3, field8_4_3.Bytes())
	writeProtobufField(&field8_4, 4, []byte{})
	var field8_4_5 bytes.Buffer
	writeProtobufField(&field8_4_5, 1, []byte{})
	writeProtobufField(&field8_4, 5, field8_4_5.Bytes())
	writeProtobufField(&field8, 4, field8_4.Bytes())
	writeProtobufField(&buf, 8, field8.Bytes())

	var field9 bytes.Buffer
	writeProtobufVarint(&field9, 1, 5)
	var field9_2 bytes.Buffer
	writeProtobufVarint(&field9_2, 1, clientVersionCode)
	writeProtobufString(&field9_2, 2, fmt.Sprintf("%d", androidAPIVersion))
	writeProtobufField(&field9, 2, field9_2.Bytes())
	writeProtobufField(&buf, 9, field9.Bytes())
	return buf.Bytes()
}

func buildPermanentlyDeleteRequest(dedupKeys []string) []byte {
	var buf bytes.Buffer
	writeProtobufVarint(&buf, 2, 2)
	for _, k := range dedupKeys {
		writeProtobufString(&buf, 3, k)
	}
	writeProtobufVarint(&buf, 4, 2)

	var field8 bytes.Buffer
	var field8_4 bytes.Buffer
	writeProtobufField(&field8_4, 2, []byte{})
	var field8_4_3 bytes.Buffer
	writeProtobufField(&field8_4_3, 1, []byte{})
	writeProtobufField(&field8_4, 3, field8_4_3.Bytes())
	writeProtobufField(&field8_4, 4, []byte{})
	var field8_4_5 bytes.Buffer
	writeProtobufField(&field8_4_5, 1, []byte{})
	writeProtobufField(&field8_4, 5, field8_4_5.Bytes())
	writeProtobufField(&field8, 4, field8_4.Bytes())
	writeProtobufField(&buf, 8, field8.Bytes())

	writeProtobufString(&buf, 9, "")
	return buf.Bytes()
}
