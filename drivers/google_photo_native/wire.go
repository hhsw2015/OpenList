package google_photo_native

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"unicode/utf8"
)

// Low-level protobuf wire primitives. Copied from gotohp_rev/backend so we
// can build and inspect the private-API messages that do not have generated
// bindings (specifically the MediaList request template and its response).

func writeVarint(buf *bytes.Buffer, v uint64) {
	for v >= 0x80 {
		buf.WriteByte(byte(v) | 0x80)
		v >>= 7
	}
	buf.WriteByte(byte(v))
}

func writeProtobufField(buf *bytes.Buffer, fieldNum int, data []byte) {
	tag := (fieldNum << 3) | 2
	writeVarint(buf, uint64(tag))
	writeVarint(buf, uint64(len(data)))
	buf.Write(data)
}

func writeProtobufVarint(buf *bytes.Buffer, fieldNum int, value int64) {
	tag := (fieldNum << 3) | 0
	writeVarint(buf, uint64(tag))
	writeVarint(buf, uint64(value))
}

func writeProtobufString(buf *bytes.Buffer, fieldNum int, value string) {
	writeProtobufField(buf, fieldNum, []byte(value))
}

func readTag(data []byte, offset int) (fieldNum int, wireType int, newOffset int) {
	if offset >= len(data) {
		return 0, 0, -1
	}
	tag, newOffset := readVarint(data, offset)
	if newOffset < 0 {
		return 0, 0, -1
	}
	return int(tag >> 3), int(tag & 0x7), newOffset
}

func readVarint(data []byte, offset int) (uint64, int) {
	var result uint64
	var shift uint
	for offset < len(data) {
		b := data[offset]
		offset++
		result |= uint64(b&0x7F) << shift
		if b < 0x80 {
			return result, offset
		}
		shift += 7
		if shift >= 64 {
			return 0, -1
		}
	}
	return 0, -1
}

// lengthFits reports whether a wire-decoded length can safely be added to
// `newOffset` and used to slice `data`. It rejects lengths that would
// overflow the int type or push past the buffer, so callers do not need to
// worry about a malformed remote response producing a negative int cast.
func lengthFits(length uint64, newOffset, dataLen int) bool {
	if newOffset < 0 {
		return false
	}
	return length <= uint64(dataLen-newOffset)
}

func skipField(data []byte, wireType int, offset int, fieldNum int) (int, bool) {
	switch wireType {
	case 0:
		_, newOffset := readVarint(data, offset)
		if newOffset < 0 {
			return offset, false
		}
		return newOffset, true
	case 1:
		if offset+8 > len(data) {
			return offset, false
		}
		return offset + 8, true
	case 2:
		length, newOffset := readVarint(data, offset)
		if !lengthFits(length, newOffset, len(data)) {
			return offset, false
		}
		return newOffset + int(length), true
	case 3:
		return skipGroup(data, offset, fieldNum)
	case 4:
		return offset, true
	case 5:
		if offset+4 > len(data) {
			return offset, false
		}
		return offset + 4, true
	default:
		return offset, false
	}
}

func skipGroup(data []byte, offset int, groupFieldNum int) (int, bool) {
	for offset < len(data) {
		fieldNum, wireType, newOffset := readTag(data, offset)
		if newOffset < 0 {
			return offset, false
		}
		offset = newOffset
		if wireType == 4 && fieldNum == groupFieldNum {
			return offset, true
		}
		var ok bool
		offset, ok = skipField(data, wireType, offset, fieldNum)
		if !ok {
			return offset, false
		}
	}
	return offset, false
}

func isPrintableString(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	for i := 0; i < len(data); {
		r, size := utf8.DecodeRune(data[i:])
		if r == utf8.RuneError && size == 1 {
			return false
		}
		if r < 32 && r != '\t' && r != '\n' && r != '\r' {
			return false
		}
		i += size
	}
	return true
}

// buildProtobufFromMap turns a JSON-shaped map (string-numeric keys, values
// scalar or nested map/array) into a serialized protobuf message. Used to
// materialize the MediaList request template.
func buildProtobufFromMap(m map[string]any) ([]byte, error) {
	var buf bytes.Buffer
	nums := make([]int, 0, len(m))
	for k := range m {
		n, err := strconv.Atoi(k)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("invalid field number key: %q", k)
		}
		nums = append(nums, n)
	}
	sort.Ints(nums)
	for _, fieldNum := range nums {
		v := m[strconv.Itoa(fieldNum)]
		if v == nil {
			continue
		}
		if err := writeAnyField(&buf, fieldNum, v); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

func writeAnyField(buf *bytes.Buffer, fieldNum int, v any) error {
	switch vv := v.(type) {
	case string:
		writeProtobufString(buf, fieldNum, vv)
	case int:
		writeProtobufVarint(buf, fieldNum, int64(vv))
	case int64:
		writeProtobufVarint(buf, fieldNum, vv)
	case int32:
		writeProtobufVarint(buf, fieldNum, int64(vv))
	case uint64:
		writeProtobufVarint(buf, fieldNum, int64(vv))
	case uint32:
		writeProtobufVarint(buf, fieldNum, int64(vv))
	case json.Number:
		i, err := vv.Int64()
		if err != nil {
			return fmt.Errorf("field %d: invalid number %q: %w", fieldNum, vv.String(), err)
		}
		writeProtobufVarint(buf, fieldNum, i)
	case float64:
		writeProtobufVarint(buf, fieldNum, int64(vv))
	case map[string]any:
		nested, err := buildProtobufFromMap(vv)
		if err != nil {
			return err
		}
		writeProtobufField(buf, fieldNum, nested)
	case []any:
		for _, elem := range vv {
			if err := writeAnyField(buf, fieldNum, elem); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("field %d: unsupported type %T", fieldNum, v)
	}
	return nil
}

func deepCopyJSON(v any) any {
	switch vv := v.(type) {
	case map[string]any:
		m := make(map[string]any, len(vv))
		for k, val := range vv {
			m[k] = deepCopyJSON(val)
		}
		return m
	case []any:
		out := make([]any, len(vv))
		for i, val := range vv {
			out[i] = deepCopyJSON(val)
		}
		return out
	default:
		return vv
	}
}

func ensureMapPath(root map[string]any, keys ...string) (map[string]any, error) {
	cur := root
	for _, k := range keys {
		next, ok := cur[k]
		if !ok || next == nil {
			created := map[string]any{}
			cur[k] = created
			cur = created
			continue
		}
		asMap, ok := next.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("path %q is not an object", k)
		}
		cur = asMap
	}
	return cur, nil
}
