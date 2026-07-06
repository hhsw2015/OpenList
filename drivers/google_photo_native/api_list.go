package google_photo_native

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"
)

// MediaItem is a parsed row from the private-API media list.
type MediaItem struct {
	MediaKey           string
	DedupKey           string
	Filename           string
	MediaType          string // "photo" | "video"
	Timestamp          int64  // unix seconds
	CountsTowardsQuota bool
	Status             int  // 1=Add, 2=Remove/Update
	IsTrash            bool
}

// MediaListResult is one page of media items plus pagination tokens.
type MediaListResult struct {
	Items         []MediaItem
	NextPageToken string
	SyncToken     string
}

const minMediaKeyLength = 10

// GetMediaList fetches one page of the library. pageToken/syncToken come
// from a previous response.
func (a *Api) GetMediaList(pageToken, syncToken string, triggerMode, limit int, requestTrash bool) (*MediaListResult, error) {
	requestData := buildMediaListRequest(pageToken, syncToken, triggerMode, limit, requestTrash)

	bearer, err := a.BearerToken()
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST",
		"https://photosdata-pa.googleapis.com/6439526531001121323/18047484249733410717",
		bytes.NewReader(requestData))
	if err != nil {
		return nil, err
	}
	req.Header.Set("accept-encoding", "gzip")
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
		return nil, fmt.Errorf("GetMediaList status %d: %s", resp.StatusCode, readErrorBody(resp))
	}
	body, err := readResponseBody(resp)
	if err != nil {
		return nil, err
	}
	items, page, sync := extractMediaItemsFromResponse(body)
	return &MediaListResult{Items: items, NextPageToken: page, SyncToken: sync}, nil
}

// -----------------------------------------------------------------------------
// Response parser. Ported verbatim from gotohp_rev/backend/api.go — the
// response is a nested protobuf with no generated bindings for the top-level
// message, so we walk the wire format by hand.
// -----------------------------------------------------------------------------

func extractMediaItemsFromResponse(data []byte) ([]MediaItem, string, string) {
	var items []MediaItem
	var pageToken, syncToken string
	offset := 0
	resync := 0
	const maxResync = 256
	for offset < len(data) {
		fieldNum, wireType, newOffset := readTag(data, offset)
		if newOffset < 0 {
			break
		}
		offset = newOffset
		switch wireType {
		case 0:
			_, newOffset := readVarint(data, offset)
			if newOffset < 0 {
				if resync < maxResync {
					resync++
					offset++
					continue
				}
				return items, pageToken, syncToken
			}
			resync = 0
			offset = newOffset
		case 2:
			length, newOffset := readVarint(data, offset)
			if !lengthFits(length, newOffset, len(data)) {
				if resync < maxResync {
					resync++
					offset++
					continue
				}
				return items, pageToken, syncToken
			}
			resync = 0
			fieldData := data[newOffset : newOffset+int(length)]
			offset = newOffset + int(length)
			if fieldNum == 1 {
				extractedItems, token, sToken := parseResponseField1(fieldData)
				items = append(items, extractedItems...)
				if token != "" {
					pageToken = token
				}
				if sToken != "" {
					syncToken = sToken
				}
				return items, pageToken, syncToken
			}
		case 5:
			if offset+4 > len(data) {
				return items, pageToken, syncToken
			}
			offset += 4
		case 1:
			if offset+8 > len(data) {
				return items, pageToken, syncToken
			}
			offset += 8
		case 3:
			newOffset, ok := skipGroup(data, offset, fieldNum)
			if !ok {
				return items, pageToken, syncToken
			}
			offset = newOffset
		case 4:
			return items, pageToken, syncToken
		default:
			newOffset, ok := skipField(data, wireType, offset, fieldNum)
			if !ok {
				return items, pageToken, syncToken
			}
			offset = newOffset
		}
	}
	return items, pageToken, syncToken
}

func parseResponseField1(data []byte) ([]MediaItem, string, string) {
	var items []MediaItem
	var pageToken, syncToken string
	offset := 0
	for offset < len(data) {
		fieldNum, wireType, newOffset := readTag(data, offset)
		if newOffset < 0 {
			break
		}
		offset = newOffset
		switch wireType {
		case 0:
			_, newOffset := readVarint(data, offset)
			if newOffset < 0 {
				return items, pageToken, syncToken
			}
			offset = newOffset
		case 2:
			length, newOffset := readVarint(data, offset)
			if !lengthFits(length, newOffset, len(data)) {
				return items, pageToken, syncToken
			}
			fieldData := data[newOffset : newOffset+int(length)]
			offset = newOffset + int(length)
			switch fieldNum {
			case 1:
				pageToken = string(fieldData)
			case 2:
				item := tryParseMediaItem(fieldData)
				if item != nil && item.MediaKey != "" {
					items = append(items, *item)
				}
			case 6:
				syncToken = string(fieldData)
			}
		case 5:
			if offset+4 > len(data) {
				return items, pageToken, syncToken
			}
			offset += 4
		case 1:
			if offset+8 > len(data) {
				return items, pageToken, syncToken
			}
			offset += 8
		case 3:
			newOffset, ok := skipGroup(data, offset, fieldNum)
			if !ok {
				return items, pageToken, syncToken
			}
			offset = newOffset
		case 4:
			return items, pageToken, syncToken
		default:
			newOffset, ok := skipField(data, wireType, offset, fieldNum)
			if !ok {
				return items, pageToken, syncToken
			}
			offset = newOffset
		}
	}
	return items, pageToken, syncToken
}

func tryParseMediaItem(data []byte) *MediaItem {
	item := &MediaItem{}
	offset := 0
	for offset < len(data) {
		fieldNum, wireType, newOffset := readTag(data, offset)
		if newOffset < 0 {
			break
		}
		offset = newOffset
		switch wireType {
		case 0:
			val, newOffset := readVarint(data, offset)
			if newOffset < 0 {
				return item
			}
			offset = newOffset
			if fieldNum == 5 {
				switch val {
				case 1:
					item.MediaType = "photo"
				case 2:
					item.MediaType = "video"
				}
			}
		case 2:
			length, newOffset := readVarint(data, offset)
			if !lengthFits(length, newOffset, len(data)) {
				return item
			}
			fieldData := data[newOffset : newOffset+int(length)]
			offset = newOffset + int(length)
			switch fieldNum {
			case 1:
				if isPrintableString(fieldData) && len(fieldData) > minMediaKeyLength {
					item.MediaKey = string(fieldData)
				} else {
					nested := tryParseMediaItem(fieldData)
					if nested != nil && nested.MediaKey != "" {
						item.MediaKey = nested.MediaKey
						if nested.Filename != "" && item.Filename == "" {
							item.Filename = nested.Filename
						}
						if nested.MediaType != "" && item.MediaType == "" {
							item.MediaType = nested.MediaType
						}
						if nested.DedupKey != "" && item.DedupKey == "" {
							item.DedupKey = nested.DedupKey
						}
					}
				}
			case 2:
				filename, cq, status, isTrash := extractField2Metadata(fieldData)
				if filename != "" {
					item.Filename = filename
				} else if isPrintableString(fieldData) {
					str := string(fieldData)
					if item.Filename == "" && strings.Contains(str, ".") {
						item.Filename = str
					} else if item.DedupKey == "" {
						item.DedupKey = str
					}
				}
				item.CountsTowardsQuota = item.CountsTowardsQuota || cq
				if status > 0 {
					item.Status = status
				}
				if isTrash {
					item.IsTrash = true
				}
				if item.DedupKey == "" {
					item.DedupKey = extractDedupKeyFromField2(fieldData)
				}
			case 4:
				ts := tryParseTimestamp(fieldData)
				if ts > 0 {
					item.Timestamp = ts
				}
			case 6:
				if item.MediaKey == "" {
					nested := tryParseMediaItem(fieldData)
					if nested != nil && nested.MediaKey != "" {
						item.MediaKey = nested.MediaKey
					}
				}
			case 22:
				if parseQuotaInfo(fieldData) {
					item.CountsTowardsQuota = true
				}
			}
		case 5:
			if offset+4 > len(data) {
				return item
			}
			offset += 4
		case 1:
			if offset+8 > len(data) {
				return item
			}
			offset += 8
		case 3:
			newOffset, ok := skipGroup(data, offset, fieldNum)
			if !ok {
				return item
			}
			offset = newOffset
		case 4:
			return item
		default:
			newOffset, ok := skipField(data, wireType, offset, fieldNum)
			if !ok {
				return item
			}
			offset = newOffset
		}
	}
	return item
}

func parseQuotaInfo(data []byte) bool {
	offset := 0
	for offset < len(data) {
		fieldNum, wireType, newOffset := readTag(data, offset)
		if newOffset < 0 {
			break
		}
		offset = newOffset
		switch wireType {
		case 0:
			val, newOffset := readVarint(data, offset)
			if newOffset < 0 {
				return false
			}
			offset = newOffset
			if fieldNum == 1 && val == 0 {
				return false
			}
		case 2:
			length, newOffset := readVarint(data, offset)
			if !lengthFits(length, newOffset, len(data)) {
				return false
			}
			offset = newOffset + int(length)
			if fieldNum == 1 {
				return true
			}
		case 5:
			offset += 4
		case 1:
			offset += 8
		default:
			newOffset, ok := skipField(data, wireType, offset, fieldNum)
			if !ok {
				return false
			}
			offset = newOffset
		}
	}
	return false
}

func tryParseTimestamp(data []byte) int64 {
	offset := 0
	for offset < len(data) {
		fieldNum, wireType, newOffset := readTag(data, offset)
		if newOffset < 0 {
			break
		}
		offset = newOffset
		if wireType == 0 && fieldNum == 1 {
			val, _ := readVarint(data, offset)
			return int64(val)
		}
		switch wireType {
		case 0:
			_, offset = readVarint(data, offset)
		case 2:
			length, newOffset := readVarint(data, offset)
			if newOffset < 0 {
				return 0
			}
			offset = newOffset + int(length)
		case 5:
			offset += 4
		case 1:
			offset += 8
		default:
			return 0
		}
	}
	return 0
}

func extractField2Metadata(data []byte) (string, bool, int, bool) {
	offset := 0
	filename := ""
	cq := false
	status := 0
	isTrash := false
	for offset < len(data) {
		fieldNum, wireType, newOffset := readTag(data, offset)
		if newOffset < 0 {
			break
		}
		offset = newOffset
		switch wireType {
		case 0:
			val, newOffset := readVarint(data, offset)
			offset = newOffset
			if fieldNum == 26 && val == 1096 {
				isTrash = true
			}
		case 2:
			length, newOffset := readVarint(data, offset)
			if !lengthFits(length, newOffset, len(data)) {
				return filename, cq, status, isTrash
			}
			fieldData := data[newOffset : newOffset+int(length)]
			offset = newOffset + int(length)
			switch fieldNum {
			case 2:
				fName, c, s, iT := extractField2Metadata(fieldData)
				if fName != "" && filename == "" {
					filename = fName
				}
				if c {
					cq = true
				}
				if s > 0 {
					status = s
				}
				if iT {
					isTrash = true
				}
			case 4:
				if isPrintableString(fieldData) {
					filename = string(fieldData)
				}
			case 16:
				if s := parseStatusField(fieldData); s > 0 {
					status = s
					if s == 2 {
						isTrash = true
					}
				}
			case 22:
				if parseQuotaInfo(fieldData) {
					cq = true
				}
			}
		case 5:
			offset += 4
		case 1:
			offset += 8
		default:
			return filename, cq, status, isTrash
		}
	}
	return filename, cq, status, isTrash
}

func parseStatusField(data []byte) int {
	offset := 0
	for offset < len(data) {
		fieldNum, wireType, newOffset := readTag(data, offset)
		if newOffset < 0 {
			break
		}
		offset = newOffset
		if wireType == 0 && fieldNum == 1 {
			val, _ := readVarint(data, offset)
			return int(val)
		}
		switch wireType {
		case 0:
			_, offset = readVarint(data, offset)
		case 2:
			length, newOffset := readVarint(data, offset)
			if newOffset < 0 {
				return 0
			}
			offset = newOffset + int(length)
		case 5:
			offset += 4
		case 1:
			offset += 8
		default:
			return 0
		}
	}
	return 0
}

func extractDedupKeyFromField21(data []byte) string {
	offset := 0
	for offset < len(data) {
		fieldNum, wireType, newOffset := readTag(data, offset)
		if newOffset < 0 {
			return ""
		}
		offset = newOffset
		switch wireType {
		case 0:
			_, newOffset := readVarint(data, offset)
			if newOffset < 0 {
				return ""
			}
			offset = newOffset
		case 2:
			length, newOffset := readVarint(data, offset)
			if !lengthFits(length, newOffset, len(data)) {
				return ""
			}
			fieldData := data[newOffset : newOffset+int(length)]
			offset = newOffset + int(length)
			if fieldNum == 1 && isPrintableString(fieldData) {
				return string(fieldData)
			}
		case 5:
			offset += 4
		case 1:
			offset += 8
		case 3:
			newOffset, ok := skipGroup(data, offset, fieldNum)
			if !ok {
				return ""
			}
			offset = newOffset
		case 4:
			return ""
		default:
			newOffset, ok := skipField(data, wireType, offset, fieldNum)
			if !ok {
				return ""
			}
			offset = newOffset
		}
	}
	return ""
}

func extractDedupKeyFromField2(data []byte) string {
	offset := 0
	for offset < len(data) {
		fieldNum, wireType, newOffset := readTag(data, offset)
		if newOffset < 0 {
			return ""
		}
		offset = newOffset
		switch wireType {
		case 0:
			_, newOffset := readVarint(data, offset)
			if newOffset < 0 {
				return ""
			}
			offset = newOffset
		case 2:
			length, newOffset := readVarint(data, offset)
			if !lengthFits(length, newOffset, len(data)) {
				return ""
			}
			fieldData := data[newOffset : newOffset+int(length)]
			offset = newOffset + int(length)
			if fieldNum == 21 {
				if k := extractDedupKeyFromField21(fieldData); k != "" {
					return k
				}
			}
			if fieldNum == 2 {
				if k := extractDedupKeyFromField2(fieldData); k != "" {
					return k
				}
			}
		case 5:
			offset += 4
		case 1:
			offset += 8
		case 3:
			newOffset, ok := skipGroup(data, offset, fieldNum)
			if !ok {
				return ""
			}
			offset = newOffset
		case 4:
			return ""
		default:
			newOffset, ok := skipField(data, wireType, offset, fieldNum)
			if !ok {
				return ""
			}
			offset = newOffset
		}
	}
	return ""
}
