package google_photo_native

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sync"

	log "github.com/sirupsen/logrus"
)

// mediaListRequestTemplateJSON mirrors the captured Google Photos private-API
// request shape. Dynamic fields are patched in via buildMediaListRequest:
//   1.2  = page size
//   1.4  = page token (only when non-empty)
//   1.6  = sync token
//   1.22.1 = trigger mode
const mediaListRequestTemplateJSON = `{
  "1": {
    "1": {
      "1": {"19": "", "20": "", "25": "", "30": {"2": ""}},
      "3": "", "4": "", "5": "", "6": "", "7": "",
      "15": "", "16": "", "17": "", "19": "", "20": "",
      "21": {"1": ""},
      "22": "", "25": "", "30": "", "31": "", "32": "", "33": "",
      "34": "", "36": "", "37": "", "38": "", "39": "", "40": "", "41": ""
    },
    "2": 50,
    "3": {
      "2": "", "3": "", "7": "", "8": "", "14": "", "16": "", "17": "",
      "18": "", "19": "", "20": "", "21": "", "22": "", "23": "",
      "27": "", "29": "", "30": "", "31": "", "32": "", "34": "",
      "37": "", "38": "", "39": "", "41": ""
    },
    "7": 2,
    "11": [1, 2],
    "22": {"1": 2}
  },
  "2": {
    "1": {"1": {"1": {"1": ""}, "2": ""}},
    "2": ""
  }
}`

var (
	mediaListTemplateOnce sync.Once
	mediaListTemplateRoot map[string]any
	mediaListTemplateErr  error
)

func getMediaListTemplate() (map[string]any, error) {
	mediaListTemplateOnce.Do(func() {
		dec := json.NewDecoder(bytes.NewReader([]byte(mediaListRequestTemplateJSON)))
		dec.UseNumber()
		var v any
		if err := dec.Decode(&v); err != nil {
			mediaListTemplateErr = fmt.Errorf("media list template: %w", err)
			return
		}
		root, ok := v.(map[string]any)
		if !ok {
			mediaListTemplateErr = fmt.Errorf("media list template root is not an object")
			return
		}
		mediaListTemplateRoot = root
	})
	return mediaListTemplateRoot, mediaListTemplateErr
}

// buildMediaListRequest wraps the template and legacy fallback builder.
// If template patching fails (log-worthy — the template is checked in and
// exercised by tests), we fall back to the legacy hand-built wire and log
// the reason so a drift is not invisible.
func buildMediaListRequest(pageToken, syncToken string, triggerMode, limit int, requestTrash bool) []byte {
	req, err := buildMediaListRequestFromTemplate(pageToken, syncToken, triggerMode, limit, requestTrash)
	if err == nil && len(req) > 0 {
		return req
	}
	if err != nil {
		log.Warnf("google_photo_native: template media-list build failed, using legacy: %v", err)
	}
	return buildMediaListRequestLegacy(pageToken, syncToken, triggerMode, limit, requestTrash)
}

func buildMediaListRequestFromTemplate(pageToken, syncToken string, triggerMode, limit int, requestTrash bool) ([]byte, error) {
	base, err := getMediaListTemplate()
	if err != nil {
		return nil, err
	}
	rootAny := deepCopyJSON(base)
	root, ok := rootAny.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("template root not object")
	}
	field1Any, ok := root["1"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("template missing field 1")
	}
	if limit > 0 {
		field1Any["2"] = int64(limit)
	}
	if pageToken != "" {
		field1Any["4"] = pageToken
	} else {
		delete(field1Any, "4")
	}
	// Only write sync token when non-empty. An initial sync (empty token)
	// should not emit a zero-length field 6 on the wire, which the legacy
	// builder also omits.
	if syncToken != "" {
		field1Any["6"] = syncToken
	} else {
		delete(field1Any, "6")
	}
	tMode := int64(2)
	if triggerMode == 1 {
		tMode = 1
	}
	field22, err := ensureMapPath(field1Any, "22")
	if err != nil {
		return nil, err
	}
	field22["1"] = tMode

	// Propagate requestTrash into the same 1.1.1.21.1 varint the legacy
	// builder writes. Without this the template silently defaulted to the
	// empty-string encoding at that field.
	trashMode := int64(2)
	if !requestTrash {
		trashMode = 1
	}
	trashPath, err := ensureMapPath(field1Any, "1", "1", "21")
	if err != nil {
		return nil, err
	}
	trashPath["1"] = trashMode

	return buildProtobufFromMap(root)
}

func buildMediaListRequestLegacy(pageToken, syncToken string, triggerMode, limit int, requestTrash bool) []byte {
	var buf bytes.Buffer
	field1 := buildMediaListRequestField1(pageToken, syncToken, triggerMode, limit, requestTrash)
	writeProtobufField(&buf, 1, field1)
	field2 := buildMediaListRequestField2()
	writeProtobufField(&buf, 2, field2)
	return buf.Bytes()
}

func buildMediaListRequestField1(pageToken, syncToken string, triggerMode, limit int, requestTrash bool) []byte {
	var buf bytes.Buffer
	mediaMetadataFields := []int{1, 3, 4, 5, 6, 7, 15, 16, 17, 19, 20, 21, 25, 30, 31, 32, 33, 34, 36, 37, 38, 39, 40, 41}
	trashMode := int64(2)
	if !requestTrash {
		trashMode = 1
	}
	writeProtobufField(&buf, 1, buildMediaListMetadataOptions(mediaMetadataFields, trashMode))
	if limit > 0 {
		writeProtobufVarint(&buf, 2, int64(limit))
	}
	albumOptions := []int{2, 3, 7, 8, 14, 16, 17, 18, 19, 20, 21, 22, 23, 27, 29, 30, 31, 32, 34, 37, 38, 39, 41}
	writeProtobufField(&buf, 3, buildEmptyNestedMessage(albumOptions))
	if pageToken != "" {
		writeProtobufString(&buf, 4, pageToken)
	}
	if syncToken != "" {
		writeProtobufString(&buf, 6, syncToken)
	}
	writeProtobufVarint(&buf, 7, 2)
	writeProtobufVarint(&buf, 11, 1)
	writeProtobufVarint(&buf, 11, 2)
	var field22 bytes.Buffer
	tMode := int64(2)
	if triggerMode == 1 {
		tMode = 1
	}
	writeProtobufVarint(&field22, 1, tMode)
	writeProtobufField(&buf, 22, field22.Bytes())
	return buf.Bytes()
}

func buildMediaListMetadataOptions(fields []int, trashMode int64) []byte {
	var inner bytes.Buffer
	for _, f := range fields {
		if f == 21 {
			continue
		}
		writeProtobufString(&inner, f, "")
	}
	var field21 bytes.Buffer
	writeProtobufVarint(&field21, 1, trashMode)
	var field215 bytes.Buffer
	writeProtobufString(&field215, 3, "")
	writeProtobufField(&field21, 5, field215.Bytes())
	writeProtobufField(&inner, 21, field21.Bytes())
	var level2 bytes.Buffer
	writeProtobufField(&level2, 1, inner.Bytes())
	var level1 bytes.Buffer
	writeProtobufField(&level1, 1, level2.Bytes())
	return level1.Bytes()
}

func buildMediaListRequestField2() []byte {
	var buf bytes.Buffer
	var f21 bytes.Buffer
	var f211 bytes.Buffer
	var f2111 bytes.Buffer
	writeProtobufField(&f2111, 1, []byte{})
	writeProtobufField(&f211, 1, f2111.Bytes())
	writeProtobufField(&f211, 2, []byte{})
	writeProtobufField(&f21, 1, f211.Bytes())
	writeProtobufField(&buf, 1, f21.Bytes())
	writeProtobufField(&buf, 2, []byte{})
	return buf.Bytes()
}

func buildEmptyNestedMessage(fields []int) []byte {
	var buf bytes.Buffer
	for _, f := range fields {
		writeProtobufField(&buf, f, []byte{})
	}
	return buf.Bytes()
}
