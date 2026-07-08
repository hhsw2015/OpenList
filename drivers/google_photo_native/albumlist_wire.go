package google_photo_native

import (
	"bytes"
	"encoding/hex"
	"os"

	log "github.com/sirupsen/logrus"
)

// debugAlbumWire dumps the first few album envelopes to stderr so we can
// derive the correct title field number from a real response. Enable by
// setting env OLT_GPHOTO_DEBUG_ALBUMS=1 for one Init cycle.
var debugAlbumWire = os.Getenv("OLT_GPHOTO_DEBUG_ALBUMS") == "1"

func dumpAlbumEnvelope(key string, data []byte) {
	log.Warnf("gphoto album envelope key=%s len=%d hex=%s", key, len(data), hex.EncodeToString(data))
	dumpFields("  ", data, 0)
}

func dumpFields(indent string, data []byte, depth int) {
	if depth > 6 {
		return
	}
	off := 0
	for off < len(data) {
		fn, wt, no := readTag(data, off)
		if no < 0 {
			return
		}
		off = no
		switch wt {
		case 0:
			v, n := readVarint(data, off)
			log.Warnf("%sfield=%d varint=%d", indent, fn, v)
			off = n
		case 2:
			length, n := readVarint(data, off)
			if !lengthFits(length, n, len(data)) {
				return
			}
			f := data[n : n+int(length)]
			off = n + int(length)
			if isPrintableString(f) && !looksLikeProtoEnvelope(f) {
				log.Warnf("%sfield=%d str=%q", indent, fn, string(f))
			} else if looksLikeProtoEnvelope(f) {
				log.Warnf("%sfield=%d nested len=%d {", indent, fn, len(f))
				dumpFields(indent+"  ", f, depth+1)
				log.Warnf("%s}", indent)
			} else {
				log.Warnf("%sfield=%d bytes(%d)=%s", indent, fn, len(f), hex.EncodeToString(f))
			}
		case 5:
			off += 4
		case 1:
			off += 8
		default:
			return
		}
	}
}

// Album-list request/response wire helpers. This is a verbatim port of the
// captured request shape gotohp reverse-engineered. Every field number and
// sub-message layout came from a real Photos client capture — do not touch
// unless a fresh capture forces a schema drift.

func buildAlbumListRequest(pageToken string) []byte {
	var buf bytes.Buffer
	writeProtobufField(&buf, 1, buildAlbumListRequestField1(pageToken))
	writeProtobufField(&buf, 2, buildAlbumListRequestField2())
	return buf.Bytes()
}

func buildAlbumListRequestField1(pageToken string) []byte {
	var buf bytes.Buffer
	writeProtobufField(&buf, 1, buildAlbumListField1_1())
	writeProtobufField(&buf, 2, buildAlbumListField1_2())
	writeProtobufField(&buf, 3, buildAlbumListField1_3())
	if pageToken != "" {
		writeProtobufString(&buf, 4, pageToken)
	}
	writeProtobufVarint(&buf, 7, 2)
	writeProtobufField(&buf, 9, buildAlbumListField1_9())
	writeProtobufVarint(&buf, 11, 1)
	writeProtobufVarint(&buf, 11, 2)
	writeProtobufVarint(&buf, 11, 6)
	writeProtobufField(&buf, 12, buildAlbumListField1_12())
	writeProtobufString(&buf, 13, "")
	writeProtobufField(&buf, 15, buildAlbumListField1_15())
	writeProtobufField(&buf, 18, buildAlbumListField1_18())
	writeProtobufField(&buf, 19, buildAlbumListField1_19())
	writeProtobufField(&buf, 20, buildAlbumListField1_20())
	writeProtobufField(&buf, 21, buildAlbumListField1_21())
	writeProtobufField(&buf, 22, buildAlbumListField1_22())
	writeProtobufField(&buf, 25, buildAlbumListField1_25())
	writeProtobufString(&buf, 26, "")
	return buf.Bytes()
}

func buildAlbumListField1_1() []byte {
	var buf bytes.Buffer
	var field1_1_1 bytes.Buffer
	empty := []int{1, 3, 4, 6, 15, 16, 17, 19, 20, 25, 31, 32, 34, 36, 37, 38, 39, 40, 41, 42}
	for _, f := range empty {
		writeProtobufString(&field1_1_1, f, "")
	}
	var f5 bytes.Buffer
	for _, f := range []int{1, 2, 3, 4, 5, 7} {
		writeProtobufString(&f5, f, "")
	}
	writeProtobufField(&field1_1_1, 5, f5.Bytes())
	var f7 bytes.Buffer
	writeProtobufString(&f7, 2, "")
	writeProtobufField(&field1_1_1, 7, f7.Bytes())
	var f21 bytes.Buffer
	var f21_5 bytes.Buffer
	writeProtobufString(&f21_5, 3, "")
	writeProtobufField(&f21, 5, f21_5.Bytes())
	writeProtobufString(&f21, 6, "")
	var f21_7 bytes.Buffer
	writeProtobufVarint(&f21_7, 2, 0)
	writeProtobufVarint(&f21_7, 3, 1)
	writeProtobufField(&f21, 7, f21_7.Bytes())
	writeProtobufField(&field1_1_1, 21, f21.Bytes())
	var f30 bytes.Buffer
	writeProtobufString(&f30, 2, "")
	writeProtobufField(&field1_1_1, 30, f30.Bytes())
	var f33 bytes.Buffer
	writeProtobufString(&f33, 1, "")
	writeProtobufField(&field1_1_1, 33, f33.Bytes())
	writeProtobufField(&buf, 1, field1_1_1.Bytes())
	return buf.Bytes()
}

func buildAlbumListField1_2() []byte {
	var buf bytes.Buffer

	var f1 bytes.Buffer
	for _, f := range []int{2, 3, 4, 5, 7, 8, 10, 12, 18} {
		writeProtobufString(&f1, f, "")
	}
	var f1_6 bytes.Buffer
	for _, f := range []int{1, 2, 3, 4, 5, 7} {
		writeProtobufString(&f1_6, f, "")
	}
	writeProtobufField(&f1, 6, f1_6.Bytes())
	var f1_13 bytes.Buffer
	writeProtobufString(&f1_13, 2, "")
	writeProtobufString(&f1_13, 3, "")
	writeProtobufField(&f1, 13, f1_13.Bytes())
	var f1_15 bytes.Buffer
	writeProtobufString(&f1_15, 1, "")
	writeProtobufField(&f1, 15, f1_15.Bytes())
	writeProtobufField(&buf, 1, f1.Bytes())

	var f4 bytes.Buffer
	var f4_1 bytes.Buffer
	writeProtobufString(&f4_1, 1, "")
	writeProtobufField(&f4, 1, f4_1.Bytes())
	writeProtobufField(&buf, 4, f4.Bytes())

	writeProtobufString(&buf, 9, "")

	var f11 bytes.Buffer
	var f11_1 bytes.Buffer
	for _, f := range []int{1, 4, 5, 6, 9} {
		writeProtobufString(&f11_1, f, "")
	}
	writeProtobufField(&f11, 1, f11_1.Bytes())
	writeProtobufField(&buf, 11, f11.Bytes())

	var f14 bytes.Buffer
	var f14_1 bytes.Buffer
	var f14_1_1 bytes.Buffer
	writeProtobufString(&f14_1_1, 1, "")

	var f14_1_1_2 bytes.Buffer
	var f14_1_1_2_2 bytes.Buffer
	var f14_1_1_2_2_1 bytes.Buffer
	writeProtobufString(&f14_1_1_2_2_1, 1, "")
	writeProtobufField(&f14_1_1_2_2, 1, f14_1_1_2_2_1.Bytes())
	writeProtobufString(&f14_1_1_2_2, 3, "")
	writeProtobufField(&f14_1_1_2, 2, f14_1_1_2_2.Bytes())
	writeProtobufField(&f14_1_1, 2, f14_1_1_2.Bytes())

	var f14_1_1_3 bytes.Buffer
	var f14_1_1_3_4 bytes.Buffer
	var f14_1_1_3_4_1 bytes.Buffer
	writeProtobufString(&f14_1_1_3_4_1, 1, "")
	writeProtobufField(&f14_1_1_3_4, 1, f14_1_1_3_4_1.Bytes())
	writeProtobufString(&f14_1_1_3_4, 3, "")
	writeProtobufField(&f14_1_1_3, 4, f14_1_1_3_4.Bytes())
	var f14_1_1_3_5 bytes.Buffer
	var f14_1_1_3_5_1 bytes.Buffer
	writeProtobufString(&f14_1_1_3_5_1, 1, "")
	writeProtobufField(&f14_1_1_3_5, 1, f14_1_1_3_5_1.Bytes())
	writeProtobufString(&f14_1_1_3_5, 3, "")
	writeProtobufField(&f14_1_1_3, 5, f14_1_1_3_5.Bytes())
	writeProtobufField(&f14_1_1, 3, f14_1_1_3.Bytes())
	writeProtobufField(&f14_1, 1, f14_1_1.Bytes())
	writeProtobufString(&f14_1, 2, "")
	writeProtobufField(&f14, 1, f14_1.Bytes())
	writeProtobufField(&buf, 14, f14.Bytes())

	writeProtobufString(&buf, 17, "")

	var f18 bytes.Buffer
	writeProtobufString(&f18, 1, "")
	var f18_2 bytes.Buffer
	writeProtobufString(&f18_2, 1, "")
	writeProtobufField(&f18, 2, f18_2.Bytes())
	writeProtobufField(&buf, 18, f18.Bytes())

	var f20 bytes.Buffer
	var f20_2 bytes.Buffer
	writeProtobufString(&f20_2, 1, "")
	writeProtobufString(&f20_2, 2, "")
	writeProtobufField(&f20, 2, f20_2.Bytes())
	writeProtobufField(&buf, 20, f20.Bytes())

	writeProtobufString(&buf, 22, "")
	writeProtobufString(&buf, 23, "")
	return buf.Bytes()
}

func buildAlbumListField1_3() []byte {
	var buf bytes.Buffer
	writeProtobufString(&buf, 2, "")

	var f3 bytes.Buffer
	empty := []int{2, 3, 7, 8, 16, 18, 19, 20, 21, 22, 23, 29, 30, 31, 32, 34, 37, 38, 39, 41, 47}
	for _, f := range empty {
		writeProtobufString(&f3, f, "")
	}
	var f3_14 bytes.Buffer
	writeProtobufString(&f3_14, 1, "")
	writeProtobufField(&f3, 14, f3_14.Bytes())
	var f3_17 bytes.Buffer
	writeProtobufString(&f3_17, 2, "")
	writeProtobufField(&f3, 17, f3_17.Bytes())
	var f3_27 bytes.Buffer
	writeProtobufString(&f3_27, 1, "")
	var f3_27_2 bytes.Buffer
	writeProtobufString(&f3_27_2, 1, "")
	writeProtobufField(&f3_27, 2, f3_27_2.Bytes())
	writeProtobufField(&f3, 27, f3_27.Bytes())
	var f3_45 bytes.Buffer
	var f3_45_1 bytes.Buffer
	writeProtobufString(&f3_45_1, 1, "")
	writeProtobufField(&f3_45, 1, f3_45_1.Bytes())
	writeProtobufField(&f3, 45, f3_45.Bytes())
	var f3_46 bytes.Buffer
	writeProtobufString(&f3_46, 1, "")
	var f3_46_2 bytes.Buffer
	var f3_46_2_1 bytes.Buffer
	writeProtobufString(&f3_46_2_1, 1, "")
	writeProtobufField(&f3_46_2, 1, f3_46_2_1.Bytes())
	writeProtobufField(&f3_46, 2, f3_46_2.Bytes())
	writeProtobufString(&f3_46, 3, "")
	writeProtobufField(&f3, 46, f3_46.Bytes())
	writeProtobufField(&buf, 3, f3.Bytes())

	var f4 bytes.Buffer
	writeProtobufString(&f4, 2, "")
	var f4_3 bytes.Buffer
	writeProtobufString(&f4_3, 1, "")
	writeProtobufField(&f4, 3, f4_3.Bytes())
	writeProtobufString(&f4, 4, "")
	var f4_5 bytes.Buffer
	writeProtobufString(&f4_5, 1, "")
	writeProtobufField(&f4, 5, f4_5.Bytes())
	writeProtobufField(&buf, 4, f4.Bytes())

	writeProtobufString(&buf, 7, "")

	var f8 bytes.Buffer
	var f8_2 bytes.Buffer
	writeProtobufVarint(&f8_2, 1, 1)
	writeProtobufVarint(&f8_2, 2, 1)
	writeProtobufField(&f8, 2, f8_2.Bytes())
	writeProtobufField(&buf, 8, f8.Bytes())

	for _, f := range []int{12, 13, 15, 18, 20, 22, 24, 25} {
		writeProtobufString(&buf, f, "")
	}

	writeProtobufField(&buf, 14, buildAlbumListField1_3_14())

	var f16 bytes.Buffer
	writeProtobufString(&f16, 1, "")
	writeProtobufField(&buf, 16, f16.Bytes())

	writeProtobufField(&buf, 19, buildAlbumListField1_3_19())
	return buf.Bytes()
}

func buildAlbumListField1_3_14() []byte {
	var buf bytes.Buffer
	writeProtobufString(&buf, 1, "")
	var f2 bytes.Buffer
	writeProtobufString(&f2, 1, "")
	var f2_2 bytes.Buffer
	writeProtobufString(&f2_2, 1, "")
	writeProtobufField(&f2, 2, f2_2.Bytes())
	writeProtobufString(&f2, 3, "")
	var f2_4 bytes.Buffer
	writeProtobufString(&f2_4, 1, "")
	writeProtobufField(&f2, 4, f2_4.Bytes())
	writeProtobufField(&buf, 2, f2.Bytes())
	var f3 bytes.Buffer
	writeProtobufString(&f3, 1, "")
	var f3_2 bytes.Buffer
	writeProtobufString(&f3_2, 1, "")
	writeProtobufField(&f3, 2, f3_2.Bytes())
	writeProtobufString(&f3, 3, "")
	writeProtobufString(&f3, 4, "")
	writeProtobufField(&buf, 3, f3.Bytes())
	return buf.Bytes()
}

func buildAlbumListField1_3_19() []byte {
	var buf bytes.Buffer
	var f4 bytes.Buffer
	writeProtobufString(&f4, 2, "")
	writeProtobufField(&buf, 4, f4.Bytes())
	var f6 bytes.Buffer
	writeProtobufString(&f6, 2, "")
	writeProtobufString(&f6, 3, "")
	writeProtobufField(&buf, 6, f6.Bytes())
	var f7 bytes.Buffer
	writeProtobufString(&f7, 2, "")
	writeProtobufString(&f7, 3, "")
	writeProtobufField(&buf, 7, f7.Bytes())
	writeProtobufString(&buf, 8, "")
	return buf.Bytes()
}

func buildAlbumListField1_9() []byte {
	var buf bytes.Buffer
	var f1 bytes.Buffer
	var f1_2 bytes.Buffer
	writeProtobufString(&f1_2, 1, "")
	writeProtobufString(&f1_2, 2, "")
	writeProtobufField(&f1, 2, f1_2.Bytes())
	writeProtobufField(&buf, 1, f1.Bytes())
	var f2 bytes.Buffer
	var f2_3 bytes.Buffer
	writeProtobufVarint(&f2_3, 2, 1)
	writeProtobufField(&f2, 3, f2_3.Bytes())
	writeProtobufField(&buf, 2, f2.Bytes())
	var f3 bytes.Buffer
	writeProtobufString(&f3, 2, "")
	writeProtobufField(&buf, 3, f3.Bytes())
	writeProtobufString(&buf, 4, "")
	var f7 bytes.Buffer
	writeProtobufString(&f7, 1, "")
	writeProtobufField(&buf, 7, f7.Bytes())
	var f8 bytes.Buffer
	writeProtobufVarint(&f8, 1, 2)
	for _, v := range []int64{1, 2, 3, 5, 6} {
		writeProtobufVarint(&f8, 2, v)
	}
	writeProtobufField(&buf, 8, f8.Bytes())
	writeProtobufString(&buf, 9, "")
	return buf.Bytes()
}

func buildAlbumListField1_12() []byte {
	var buf bytes.Buffer
	var f2 bytes.Buffer
	writeProtobufString(&f2, 1, "")
	writeProtobufString(&f2, 2, "")
	writeProtobufField(&buf, 2, f2.Bytes())
	var f3 bytes.Buffer
	writeProtobufString(&f3, 1, "")
	writeProtobufField(&buf, 3, f3.Bytes())
	writeProtobufString(&buf, 4, "")
	return buf.Bytes()
}

func buildAlbumListField1_15() []byte {
	var buf bytes.Buffer
	var f3 bytes.Buffer
	writeProtobufVarint(&f3, 1, 1)
	writeProtobufField(&buf, 3, f3.Bytes())
	return buf.Bytes()
}

func buildAlbumListField1_18() []byte {
	var buf bytes.Buffer
	var outer bytes.Buffer
	var l1 bytes.Buffer
	var l2 bytes.Buffer
	for _, v := range []int64{2, 1, 6, 8, 10, 15, 18, 13, 17, 19, 14, 20} {
		writeProtobufVarint(&l2, 4, v)
	}
	writeProtobufVarint(&l2, 5, 6)
	writeProtobufVarint(&l2, 6, 2)
	writeProtobufVarint(&l2, 7, 1)
	writeProtobufVarint(&l2, 8, 2)
	writeProtobufVarint(&l2, 11, 3)
	writeProtobufVarint(&l2, 12, 1)
	writeProtobufVarint(&l2, 13, 3)
	writeProtobufVarint(&l2, 15, 1)
	writeProtobufVarint(&l2, 16, 1)
	writeProtobufVarint(&l2, 17, 1)
	writeProtobufVarint(&l2, 18, 2)
	writeProtobufField(&l1, 1, l2.Bytes())
	writeProtobufField(&outer, 1, l1.Bytes())
	writeProtobufField(&buf, 169945741, outer.Bytes())
	return buf.Bytes()
}

func buildAlbumListField1_19() []byte {
	var buf bytes.Buffer
	var f1 bytes.Buffer
	writeProtobufString(&f1, 1, "")
	writeProtobufString(&f1, 2, "")
	writeProtobufField(&buf, 1, f1.Bytes())
	var f2 bytes.Buffer
	for _, v := range []int64{1, 2, 4, 6, 5, 7} {
		writeProtobufVarint(&f2, 1, v)
	}
	writeProtobufField(&buf, 2, f2.Bytes())
	var f3 bytes.Buffer
	writeProtobufString(&f3, 1, "")
	writeProtobufString(&f3, 2, "")
	writeProtobufField(&buf, 3, f3.Bytes())
	var f5 bytes.Buffer
	writeProtobufString(&f5, 1, "")
	writeProtobufString(&f5, 2, "")
	writeProtobufField(&buf, 5, f5.Bytes())
	var f6 bytes.Buffer
	writeProtobufString(&f6, 1, "")
	writeProtobufField(&buf, 6, f6.Bytes())
	var f7 bytes.Buffer
	writeProtobufString(&f7, 1, "")
	writeProtobufString(&f7, 2, "")
	writeProtobufField(&buf, 7, f7.Bytes())
	var f8 bytes.Buffer
	writeProtobufString(&f8, 1, "")
	writeProtobufField(&buf, 8, f8.Bytes())
	return buf.Bytes()
}

func buildAlbumListField1_20() []byte {
	var buf bytes.Buffer
	writeProtobufVarint(&buf, 1, 1)
	var f3 bytes.Buffer
	writeProtobufString(&f3, 1, "type.googleapis.com/photos.printing.client.PrintingPromotionSyncOptions")
	var f3_2 bytes.Buffer
	var f3_2_1 bytes.Buffer
	for _, v := range []int64{2, 1, 6, 8, 10, 15, 18, 13, 17, 19, 14, 20} {
		writeProtobufVarint(&f3_2_1, 4, v)
	}
	writeProtobufVarint(&f3_2_1, 5, 6)
	writeProtobufVarint(&f3_2_1, 6, 2)
	writeProtobufVarint(&f3_2_1, 7, 1)
	writeProtobufVarint(&f3_2_1, 8, 2)
	writeProtobufVarint(&f3_2_1, 11, 3)
	writeProtobufVarint(&f3_2_1, 12, 1)
	writeProtobufVarint(&f3_2_1, 13, 3)
	writeProtobufVarint(&f3_2_1, 15, 1)
	writeProtobufVarint(&f3_2_1, 16, 1)
	writeProtobufVarint(&f3_2_1, 17, 1)
	writeProtobufVarint(&f3_2_1, 18, 2)
	writeProtobufField(&f3_2, 1, f3_2_1.Bytes())
	writeProtobufField(&f3, 2, f3_2.Bytes())
	writeProtobufField(&buf, 3, f3.Bytes())
	return buf.Bytes()
}

func buildAlbumListField1_21() []byte {
	var buf bytes.Buffer

	var f2 bytes.Buffer
	writeProtobufString(&f2, 2, "")
	writeProtobufString(&f2, 4, "")
	writeProtobufString(&f2, 5, "")
	writeProtobufField(&buf, 2, f2.Bytes())

	var f3 bytes.Buffer
	var f3_2 bytes.Buffer
	writeProtobufVarint(&f3_2, 1, 1)
	writeProtobufField(&f3, 2, f3_2.Bytes())
	var f3_4 bytes.Buffer
	writeProtobufString(&f3_4, 2, "")
	var f3_4_7 bytes.Buffer
	writeProtobufVarint(&f3_4_7, 2, 0)
	writeProtobufField(&f3_4, 7, f3_4_7.Bytes())
	writeProtobufField(&f3, 4, f3_4.Bytes())
	writeProtobufString(&f3, 8, "")
	writeProtobufField(&buf, 3, f3.Bytes())

	var f5 bytes.Buffer
	writeProtobufString(&f5, 1, "")
	writeProtobufField(&buf, 5, f5.Bytes())

	var f6 bytes.Buffer
	writeProtobufString(&f6, 1, "")
	var f6_2 bytes.Buffer
	writeProtobufString(&f6_2, 1, "")
	writeProtobufField(&f6, 2, f6_2.Bytes())
	writeProtobufField(&buf, 6, f6.Bytes())

	var f7 bytes.Buffer
	writeProtobufVarint(&f7, 1, 2)
	for _, v := range []int64{1, 7, 8, 9, 10, 13, 14, 15, 17, 19, 20, 22, 23, 45, 46, 47, 48, 49, 58, 6, 24, 50, 54, 55, 59, 62, 63, 64, 65, 56, 57, 60, 69} {
		writeProtobufVarint(&f7, 2, v)
	}
	writeProtobufVarint(&f7, 3, 1)
	writeProtobufField(&buf, 7, f7.Bytes())

	writeProtobufField(&buf, 8, buildAlbumListField1_21_8())

	var f9 bytes.Buffer
	writeProtobufString(&f9, 1, "")
	writeProtobufField(&buf, 9, f9.Bytes())

	var f10 bytes.Buffer
	var f10_1 bytes.Buffer
	writeProtobufString(&f10_1, 1, "")
	writeProtobufField(&f10, 1, f10_1.Bytes())
	for _, f := range []int{3, 5, 7, 9, 10} {
		writeProtobufString(&f10, f, "")
	}
	var f10_6 bytes.Buffer
	writeProtobufString(&f10_6, 1, "")
	writeProtobufField(&f10, 6, f10_6.Bytes())
	writeProtobufField(&buf, 10, f10.Bytes())

	for _, f := range []int{11, 12, 13, 14} {
		writeProtobufString(&buf, f, "")
	}

	var f19 bytes.Buffer
	writeProtobufString(&f19, 1, "")
	writeProtobufString(&f19, 2, "")
	writeProtobufField(&buf, 19, f19.Bytes())
	return buf.Bytes()
}

func buildAlbumListField1_21_8() []byte {
	var buf bytes.Buffer
	inner := buildAlbumListInnerTriple()

	// f3 = {1:{1:<inner>}, 3:""}
	var f3 bytes.Buffer
	var f3_1 bytes.Buffer
	writeProtobufField(&f3_1, 1, inner)
	writeProtobufField(&f3, 1, f3_1.Bytes())
	writeProtobufString(&f3, 3, "")
	writeProtobufField(&buf, 3, f3.Bytes())

	// f4 = {1:""}
	var f4 bytes.Buffer
	writeProtobufString(&f4, 1, "")
	writeProtobufField(&buf, 4, f4.Bytes())

	// f5 = {1:<inner>}
	var f5 bytes.Buffer
	writeProtobufField(&f5, 1, inner)
	writeProtobufField(&buf, 5, f5.Bytes())

	// f6 = {1:{1:<inner>}, 2:{1:<inner>}}
	var f6 bytes.Buffer
	var f6_1 bytes.Buffer
	writeProtobufField(&f6_1, 1, inner)
	writeProtobufField(&f6, 1, f6_1.Bytes())
	var f6_2 bytes.Buffer
	writeProtobufField(&f6_2, 1, inner)
	writeProtobufField(&f6, 2, f6_2.Bytes())
	writeProtobufField(&buf, 6, f6.Bytes())

	return buf.Bytes()
}

// buildAlbumListInnerTriple is the recurring `{2:{1:1}, 4:{2:"",7:{2:0}}, 8:""}`
// substructure that appears many times inside 1.21.8.
func buildAlbumListInnerTriple() []byte {
	var buf bytes.Buffer
	var f2 bytes.Buffer
	writeProtobufVarint(&f2, 1, 1)
	writeProtobufField(&buf, 2, f2.Bytes())
	var f4 bytes.Buffer
	writeProtobufString(&f4, 2, "")
	var f4_7 bytes.Buffer
	writeProtobufVarint(&f4_7, 2, 0)
	writeProtobufField(&f4, 7, f4_7.Bytes())
	writeProtobufField(&buf, 4, f4.Bytes())
	writeProtobufString(&buf, 8, "")
	return buf.Bytes()
}

func buildAlbumListField1_22() []byte {
	var buf bytes.Buffer
	writeProtobufVarint(&buf, 1, 2)
	return buf.Bytes()
}

func buildAlbumListField1_25() []byte {
	var buf bytes.Buffer
	var f1 bytes.Buffer
	var f1_1 bytes.Buffer
	var f1_1_1 bytes.Buffer
	writeProtobufString(&f1_1_1, 1, "")
	writeProtobufField(&f1_1, 1, f1_1_1.Bytes())
	writeProtobufField(&f1, 1, f1_1.Bytes())
	writeProtobufField(&buf, 1, f1.Bytes())
	writeProtobufString(&buf, 2, "")
	return buf.Bytes()
}

func buildAlbumListRequestField2() []byte {
	var buf bytes.Buffer
	var f1 bytes.Buffer
	var f1_1 bytes.Buffer
	var f1_1_1 bytes.Buffer
	writeProtobufString(&f1_1_1, 1, "")
	writeProtobufField(&f1_1, 1, f1_1_1.Bytes())
	writeProtobufString(&f1_1, 2, "")
	writeProtobufField(&f1, 1, f1_1.Bytes())
	writeProtobufField(&buf, 1, f1.Bytes())
	writeProtobufString(&buf, 2, "")
	return buf.Bytes()
}

// -----------------------------------------------------------------------------
// Response parser
// -----------------------------------------------------------------------------

func extractAlbumsFromResponse(data []byte) ([]AlbumItem, string) {
	var albums []AlbumItem
	var pageToken string
	offset := 0
	for offset < len(data) {
		fieldNum, wireType, newOffset := readTag(data, offset)
		if newOffset < 0 {
			break
		}
		offset = newOffset
		switch wireType {
		case 0:
			_, offset = readVarint(data, offset)
		case 2:
			length, newOffset := readVarint(data, offset)
			if !lengthFits(length, newOffset, len(data)) {
				return albums, pageToken
			}
			fieldData := data[newOffset : newOffset+int(length)]
			offset = newOffset + int(length)
			if fieldNum == 1 {
				a, t := parseAlbumResponseField1(fieldData)
				albums = append(albums, a...)
				if t != "" {
					pageToken = t
				}
			}
		case 5:
			offset += 4
		case 1:
			offset += 8
		default:
			return albums, pageToken
		}
	}
	return albums, pageToken
}

func parseAlbumResponseField1(data []byte) ([]AlbumItem, string) {
	var albums []AlbumItem
	var pageToken string
	offset := 0
	for offset < len(data) {
		fieldNum, wireType, newOffset := readTag(data, offset)
		if newOffset < 0 {
			break
		}
		offset = newOffset
		switch wireType {
		case 0:
			_, offset = readVarint(data, offset)
		case 2:
			length, newOffset := readVarint(data, offset)
			if !lengthFits(length, newOffset, len(data)) {
				return albums, pageToken
			}
			fieldData := data[newOffset : newOffset+int(length)]
			offset = newOffset + int(length)
			if fieldNum == 4 {
				pageToken = string(fieldData)
			}
			if a := tryParseAlbumItem(fieldData); a != nil && a.AlbumKey != "" {
				if debugAlbumWire && len(albums) < 3 {
					dumpAlbumEnvelope(a.AlbumKey, fieldData)
				}
				albums = append(albums, *a)
			}
		case 5:
			offset += 4
		case 1:
			offset += 8
		default:
			return albums, pageToken
		}
	}
	return albums, pageToken
}

// isAlbumKeyLike matches Google Photos album keys — a leading "AF1Qip"
// followed by URL-safe base64 chars. Anything else at field 1 is either
// a nested envelope (which we descend into) or noise.
func isAlbumKeyLike(data []byte) bool {
	if len(data) < 10 || len(data) > 200 {
		return false
	}
	if !bytes.HasPrefix(data, []byte("AF1Qip")) {
		return false
	}
	for _, b := range data {
		if b >= 'A' && b <= 'Z' {
			continue
		}
		if b >= 'a' && b <= 'z' {
			continue
		}
		if b >= '0' && b <= '9' {
			continue
		}
		if b == '_' || b == '-' {
			continue
		}
		return false
	}
	return true
}

// looksLikeProtoEnvelope checks whether the buffer starts with a plausible
// protobuf tag whose length-prefix would consume most of the buffer.
func looksLikeProtoEnvelope(data []byte) bool {
	if len(data) < 2 {
		return false
	}
	// The wire byte encodes tag+wireType; tag == 1 (0x0A) or 2 (0x12) with
	// wireType 2 (length-delimited) is what we see in album envelopes.
	if data[0] != 0x0A && data[0] != 0x12 {
		return false
	}
	length, off := readVarint(data, 1)
	if off < 0 {
		return false
	}
	return int(length) > 0 && off+int(length) <= len(data)
}

// extractInnerAlbumKey looks for a field-1 (or field-2) string inside a
// nested envelope that matches Google's AF1Qip album key format.
func extractInnerAlbumKey(data []byte) string {
	offset := 0
	for offset < len(data) {
		fieldNum, wireType, newOffset := readTag(data, offset)
		if newOffset < 0 {
			return ""
		}
		offset = newOffset
		if wireType == 2 {
			length, no := readVarint(data, offset)
			if !lengthFits(length, no, len(data)) {
				return ""
			}
			f := data[no : no+int(length)]
			offset = no + int(length)
			if (fieldNum == 1 || fieldNum == 2) && isAlbumKeyLike(f) {
				return string(f)
			}
		} else if wireType == 0 {
			_, offset = readVarint(data, offset)
		} else if wireType == 5 {
			offset += 4
		} else if wireType == 1 {
			offset += 8
		} else {
			return ""
		}
	}
	return ""
}

// extractCoverFilename walks the album envelope's nested field 2 to find
// the cover media's filename (field 4 in the media descriptor). The
// public album-list endpoint does not return album titles, so the cover
// filename is the most useful display string we can synthesize.
func extractCoverFilename(envelope []byte) string {
	// Envelope structure:
	//   field=1: album key (top-level string)
	//   field=2: {   // nested media descriptor
	//     field=1..3: media metadata (uploader ids etc)
	//     field=4: filename
	//     ...
	//   }
	offset := 0
	for offset < len(envelope) {
		fn, wt, no := readTag(envelope, offset)
		if no < 0 {
			return ""
		}
		offset = no
		if wt == 2 {
			length, n2 := readVarint(envelope, offset)
			if !lengthFits(length, n2, len(envelope)) {
				return ""
			}
			body := envelope[n2 : n2+int(length)]
			offset = n2 + int(length)
			if fn == 2 {
				// Descend and pull field 4 as the filename.
				io := 0
				for io < len(body) {
					ifn, iwt, ino := readTag(body, io)
					if ino < 0 {
						break
					}
					io = ino
					if iwt == 2 {
						il, iln := readVarint(body, io)
						if !lengthFits(il, iln, len(body)) {
							break
						}
						f := body[iln : iln+int(il)]
						io = iln + int(il)
						if ifn == 4 && isPrintableString(f) && !looksLikeProtoEnvelope(f) {
							return string(f)
						}
					} else if iwt == 0 {
						_, io = readVarint(body, io)
					} else if iwt == 5 {
						io += 4
					} else if iwt == 1 {
						io += 8
					} else {
						break
					}
				}
			}
		} else if wt == 0 {
			_, offset = readVarint(envelope, offset)
		} else if wt == 5 {
			offset += 4
		} else if wt == 1 {
			offset += 8
		} else {
			return ""
		}
	}
	return ""
}

// extractInnerTitle looks for a printable string inside a nested title
// envelope, preferring field 2 (the direct title slot) then falling back
// to the first printable non-key field.
func extractInnerTitle(data []byte) string {
	offset := 0
	var fallback string
	for offset < len(data) {
		fieldNum, wireType, newOffset := readTag(data, offset)
		if newOffset < 0 {
			break
		}
		offset = newOffset
		if wireType == 2 {
			length, no := readVarint(data, offset)
			if !lengthFits(length, no, len(data)) {
				break
			}
			f := data[no : no+int(length)]
			offset = no + int(length)
			if isPrintableString(f) && !isAlbumKeyLike(f) && !looksLikeProtoEnvelope(f) {
				if fieldNum == 2 {
					return string(f)
				}
				if fallback == "" {
					fallback = string(f)
				}
			}
		} else if wireType == 0 {
			_, offset = readVarint(data, offset)
		} else if wireType == 5 {
			offset += 4
		} else if wireType == 1 {
			offset += 8
		} else {
			break
		}
	}
	return fallback
}

func tryParseAlbumItem(data []byte) *AlbumItem {
	album := &AlbumItem{}
	hasData := false
	offset := 0
	for offset < len(data) {
		fieldNum, wireType, newOffset := readTag(data, offset)
		if newOffset < 0 {
			break
		}
		offset = newOffset
		switch wireType {
		case 0:
			value, newOffset := readVarint(data, offset)
			if newOffset >= 0 && (fieldNum == 3 || fieldNum == 5) {
				album.MediaCount = int(value)
				hasData = true
			}
			offset = newOffset
		case 2:
			length, newOffset := readVarint(data, offset)
			if !lengthFits(length, newOffset, len(data)) {
				return nil
			}
			fieldData := data[newOffset : newOffset+int(length)]
			offset = newOffset + int(length)
			if fieldNum == 1 {
				// The album envelope wraps the actual AlbumKey in an inner
				// field-1. Try to descend before falling back to raw bytes,
				// so we don't stamp `\n<len>AF1Qip...` as the key.
				if inner := extractInnerAlbumKey(fieldData); inner != "" {
					album.AlbumKey = inner
					hasData = true
				} else if isAlbumKeyLike(fieldData) {
					album.AlbumKey = string(fieldData)
					hasData = true
				}
			}
			if fieldNum == 2 && isPrintableString(fieldData) {
				// Guard against the same envelope-vs-string confusion for
				// titles: raw wire tag bytes are also "printable" per
				// isPrintableString's rules.
				if looksLikeProtoEnvelope(fieldData) {
					if t := extractInnerTitle(fieldData); t != "" {
						album.Title = t
						hasData = true
					}
				} else {
					album.Title = string(fieldData)
					hasData = true
				}
			}
		case 5:
			offset += 4
		case 1:
			offset += 8
		}
	}
	if hasData {
		return album
	}
	return nil
}
