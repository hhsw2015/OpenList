package google_photo_native

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// TestDisguiseRoundTrip encodes a payload as MP4-disguised bytes, then reads
// the head with ParseDisguiseHead and verifies payload offset + name match.
// Runs the same magic-and-trailer parser the driver relies on in Link.
func TestDisguiseRoundTrip(t *testing.T) {
	tmp := t.TempDir()

	origName := "hello.zip"
	payload := []byte("hello world, this is not really a zip file\n")
	src := filepath.Join(tmp, origName)
	if err := os.WriteFile(src, payload, 0o600); err != nil {
		t.Fatalf("write src: %v", err)
	}

	// 1) streaming disguise
	rc, total, err := OpenDisguiseReader(src, origName)
	if err != nil {
		t.Fatalf("OpenDisguiseReader: %v", err)
	}
	blob, err := io.ReadAll(rc)
	_ = rc.Close()
	if err != nil {
		t.Fatalf("read all: %v", err)
	}
	if int64(len(blob)) != total {
		t.Fatalf("wrapper size mismatch: got %d want %d", len(blob), total)
	}
	if int64(len(blob)) != DisguiseSize(origName, int64(len(payload))) {
		t.Fatalf("DisguiseSize disagrees with reader")
	}

	// 2) head parse
	head := blob
	if int64(len(head)) > DisguiseHeadWindow() {
		head = head[:DisguiseHeadWindow()]
	}
	name, payloadStart, ok, err := ParseDisguiseHead(head)
	if err != nil {
		t.Fatalf("ParseDisguiseHead err: %v", err)
	}
	if !ok {
		t.Fatalf("ParseDisguiseHead did not detect trailer")
	}
	if name != origName {
		t.Fatalf("name mismatch: got %q want %q", name, origName)
	}
	if payloadStart <= 0 || payloadStart >= int64(len(blob)) {
		t.Fatalf("payloadStart out of range: %d (total %d)", payloadStart, len(blob))
	}
	if !bytes.Equal(blob[payloadStart:], payload) {
		t.Fatalf("payload bytes not preserved")
	}

	// 3) real MP4 (no trailer) is detected as non-disguised
	realMP4 := mp4Template[:len(mp4Template):len(mp4Template)]
	_, _, ok2, err2 := ParseDisguiseHead(realMP4)
	if err2 != nil {
		t.Fatalf("real MP4 ParseDisguiseHead err: %v", err2)
	}
	if ok2 {
		t.Fatalf("real MP4 should NOT match the disguise magic")
	}
}
