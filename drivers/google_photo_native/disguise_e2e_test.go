package google_photo_native

// End-to-end disguise round-trip test.
//
// Uploads a random non-media payload wrapped in the MP4 disguise, waits for
// it to appear in the media list, opens a Link, and verifies the byte-shifted
// RangeReader returns the original payload verbatim. Cleans up by trashing
// the uploaded item.
//
// Runs only when GPN_SMOKE_CRED_A is set. Skipped in normal CI.
//
// Usage:
//   GPN_SMOKE_CRED_A='...' go test -run TestDisguiseE2E -v \
//     -timeout 5m ./drivers/google_photo_native/

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"io"
	"os"
	"testing"
	"time"

	"github.com/OpenListTeam/OpenList/v4/pkg/http_range"
)

func TestDisguiseE2E(t *testing.T) {
	credA := os.Getenv("GPN_SMOKE_CRED_A")
	if credA == "" {
		t.Skip("GPN_SMOKE_CRED_A not set; skipping disguise E2E")
	}

	ctx := context.Background()
	client, err := newHTTPClientWithProxy("")
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	api := newApi(credA, "cred_a", "en", client)

	// A ~1 MB random payload — big enough to force byte-shift math to matter
	// and small enough to make the round-trip fast.
	payload := make([]byte, 1024*1024)
	if _, err := rand.Read(payload); err != nil {
		t.Fatal(err)
	}
	origName := "e2e-disguise-" + hex.EncodeToString(payload[:8]) + ".bin"

	// Write payload to a tmp file (HideAsMP4 needs a path).
	payloadFile, err := os.CreateTemp("", "gpn-e2e-payload-*.bin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := payloadFile.Write(payload); err != nil {
		t.Fatal(err)
	}
	_ = payloadFile.Close()
	defer os.Remove(payloadFile.Name())

	// Wrap as MP4 disguise; the file we actually upload is the wrapper.
	disguisedPath, err := HideAsMP4(payloadFile.Name(), "")
	if err != nil {
		t.Fatalf("HideAsMP4: %v", err)
	}
	defer os.Remove(disguisedPath)

	// Rename disguise blob's embedded filename to origName. HideAsMP4 uses
	// filepath.Base(src), which would be the tmp file's random name — we
	// want the readable one so List displays it sensibly.
	//
	// Re-wrap with the desired name:
	os.Remove(disguisedPath)
	rc, total, err := OpenDisguiseReader(payloadFile.Name(), origName)
	if err != nil {
		t.Fatalf("OpenDisguiseReader: %v", err)
	}
	disguisedPath2, err := os.CreateTemp("", "gpn-e2e-disguise-*.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(disguisedPath2, rc); err != nil {
		t.Fatalf("copy disguise: %v", err)
	}
	_ = rc.Close()
	_ = disguisedPath2.Close()
	defer os.Remove(disguisedPath2.Name())
	t.Logf("payload size=%d disguise size=%d name=%s", len(payload), total, origName)

	// Hash the disguised bytes (upload SHA-1 is over the wrapper).
	disguisedHash, err := sha1OfFile(disguisedPath2.Name())
	if err != nil {
		t.Fatal(err)
	}
	shaBytes, _ := hex.DecodeString(disguisedHash)
	shaB64 := base64.StdEncoding.EncodeToString(shaBytes)

	// Upload.
	uploadID, err := api.GetUploadToken(shaB64, total)
	if err != nil {
		t.Fatalf("GetUploadToken: %v", err)
	}
	f, err := os.Open(disguisedPath2.Name())
	if err != nil {
		t.Fatal(err)
	}
	tok, err := api.UploadStream(ctx, uploadID, f, total)
	_ = f.Close()
	if err != nil {
		t.Fatalf("UploadStream: %v", err)
	}
	// Wire filename must end in `.mp4` for Photos to accept the disguised
	// container as media.
	mediaKey, err := api.CommitUpload(tok, origName+".mp4", shaBytes, 0, QualityOriginal)
	if err != nil {
		t.Fatalf("CommitUpload: %v", err)
	}
	t.Logf("uploaded mediaKey=%s", mediaKey)

	defer func() {
		if err := api.MoveToTrash([]string{mediaKey}); err != nil {
			t.Logf("cleanup failed: %v (manual cleanup: mediaKey=%s)", err, mediaKey)
		} else {
			t.Log("cleanup: trashed")
		}
	}()

	// Get download URL and inspect head via our RangeReader.
	urls, err := api.GetDownloadURLs(mediaKey)
	if err != nil {
		t.Fatalf("GetDownloadURLs: %v", err)
	}
	url := urls.OriginalURL
	if url == "" {
		url = urls.EditedURL
	}
	if url == "" {
		t.Fatal("no download URL returned")
	}

	rr := newDisguiseRangeReader(url, total, client)
	if err := rr.inspect(ctx); err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if !rr.IsDisguised() {
		t.Fatal("uploaded blob was not detected as disguised — parser bug")
	}
	if rr.PayloadSize() != int64(len(payload)) {
		t.Errorf("payload size mismatch: got %d want %d", rr.PayloadSize(), len(payload))
	}

	// Read the whole payload back and compare byte-for-byte.
	rd, err := rr.RangeRead(ctx, http_range.Range{Start: 0, Length: rr.PayloadSize()})
	if err != nil {
		t.Fatalf("RangeRead full: %v", err)
	}
	got, err := io.ReadAll(rd)
	_ = rd.Close()
	if err != nil {
		t.Fatalf("read all: %v", err)
	}
	if len(got) != len(payload) {
		t.Fatalf("read length mismatch: got %d want %d", len(got), len(payload))
	}
	for i := range got {
		if got[i] != payload[i] {
			t.Fatalf("payload byte mismatch at offset %d: got %x want %x", i, got[i], payload[i])
		}
	}
	t.Log("full-range read verified byte-for-byte")

	// Also verify a mid-file range so byte-shift math is exercised.
	mid := int64(len(payload) / 2)
	rd2, err := rr.RangeRead(ctx, http_range.Range{Start: mid, Length: 4096})
	if err != nil {
		t.Fatalf("RangeRead mid: %v", err)
	}
	slice, err := io.ReadAll(rd2)
	_ = rd2.Close()
	if err != nil {
		t.Fatalf("mid read: %v", err)
	}
	if len(slice) < 4096 {
		t.Fatalf("short mid read: %d", len(slice))
	}
	for i := 0; i < 4096; i++ {
		if slice[i] != payload[int(mid)+i] {
			t.Fatalf("mid byte mismatch at %d: got %x want %x", i, slice[i], payload[int(mid)+i])
		}
	}
	t.Log("mid-range read verified byte-for-byte")

	// Suppress unused import warning if `time` gets trimmed by a future edit.
	_ = time.Second
}
