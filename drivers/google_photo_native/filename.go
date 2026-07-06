package google_photo_native

import (
	"path/filepath"
	"strings"
)

// supportedFormats lists extensions Google Photos ingests natively. Anything
// outside this set is a candidate for disguise wrapping when DisguiseNonMedia
// is enabled.
var supportedFormats = map[string]bool{
	// Photo formats
	"avif": true, "bmp": true, "gif": true, "heic": true, "heif": true, "ico": true,
	"jpg": true, "jpeg": true, "png": true, "tif": true, "tiff": true, "webp": true,
	"cr2": true, "cr3": true, "nef": true, "arw": true, "orf": true,
	"raf": true, "rw2": true, "pef": true, "sr2": true, "dng": true,
	// Video formats
	"3gp": true, "3g2": true, "asf": true, "avi": true, "divx": true,
	"m2t": true, "m2ts": true, "m4v": true, "mkv": true, "mmv": true,
	"mod": true, "mov": true, "mp4": true, "mpg": true, "mpeg": true,
	"mts": true, "tod": true, "wmv": true, "ts": true,
}

func isSupportedByGooglePhotos(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	if ext == "" {
		return false
	}
	return supportedFormats[ext[1:]]
}

// IsMediaFilename returns true when Google Photos accepts the extension
// natively, i.e. we do NOT need to disguise the file.
func IsMediaFilename(name string) bool {
	return isSupportedByGooglePhotos(name)
}
