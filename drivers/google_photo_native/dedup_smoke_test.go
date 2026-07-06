package google_photo_native

// Cross-account dedup smoke test.
//
// Runs only when GPN_SMOKE_CRED_A and GPN_SMOKE_CRED_B env vars are set to
// full "authData" query-strings for two different Google accounts. This is
// a real-world integration test that uploads a tiny JPEG, verifies the
// hash lookup semantics, and trashes the result. Skipped in CI.
//
// Usage:
//   GPN_SMOKE_CRED_A='androidId=...' \
//   GPN_SMOKE_CRED_B='androidId=...' \
//   go test -run TestCrossAccountDedup -v ./drivers/google_photo_native/

import (
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"os"
	"testing"
)

func TestCrossAccountDedup(t *testing.T) {
	credA := os.Getenv("GPN_SMOKE_CRED_A")
	credB := os.Getenv("GPN_SMOKE_CRED_B")
	if credA == "" || credB == "" {
		t.Skip("GPN_SMOKE_CRED_A / GPN_SMOKE_CRED_B not set; skipping cross-account dedup smoke test")
	}

	ctx := context.Background()
	client, err := newHTTPClientWithProxy("")
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	apiA := newApi(credA, "cred_a", "en", client)
	apiB := newApi(credB, "cred_b", "en", client)

	// Real JPEG base; small tail keeps the SHA-1 unique per run.
	basePath := os.Getenv("GPN_SMOKE_BASE_JPEG")
	if basePath == "" {
		basePath = "/tmp/gpn-real.jpg"
	}
	baseBody, err := os.ReadFile(basePath)
	if err != nil {
		t.Fatalf("read base jpeg %s: %v (set GPN_SMOKE_BASE_JPEG or place a real jpeg at /tmp/gpn-real.jpg)", basePath, err)
	}
	if len(baseBody) < 1024 {
		t.Fatalf("base jpeg too small (%d bytes) — Google rejects tiny images", len(baseBody))
	}
	tail := make([]byte, 32)
	if _, err := rand.Read(tail); err != nil {
		t.Fatal(err)
	}
	body := append(baseBody, tail...)
	sum := sha1.Sum(body)
	shaHex := hex.EncodeToString(sum[:])
	shaB64 := base64.StdEncoding.EncodeToString(sum[:])
	t.Logf("payload size=%d sha1=%s", len(body), shaHex)

	tmp, err := os.CreateTemp("", "gpn-dedup-*.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tmp.Write(body); err != nil {
		t.Fatal(err)
	}
	_ = tmp.Close()
	defer os.Remove(tmp.Name())

	// Pre-check A: fresh hash should miss.
	preA, err := apiA.FindRemoteMediaByHash(sum[:])
	if err != nil {
		t.Fatalf("preA: %v", err)
	}
	if preA != "" {
		t.Fatalf("preA already has this hash %q — rare-hash guarantee broken", preA)
	}
	t.Log("preA hash miss: OK")

	// Upload from A.
	uploadID, err := apiA.GetUploadToken(shaB64, int64(len(body)))
	if err != nil {
		t.Fatalf("GetUploadToken: %v", err)
	}
	f, err := os.Open(tmp.Name())
	if err != nil {
		t.Fatal(err)
	}
	tok, err := apiA.UploadStream(ctx, uploadID, f, int64(len(body)))
	_ = f.Close()
	if err != nil {
		t.Fatalf("UploadStream: %v", err)
	}
	mediaKey, err := apiA.CommitUpload(tok, "smoke.jpg", sum[:], 0, QualityOriginal)
	if err != nil {
		t.Fatalf("CommitUpload: %v", err)
	}
	t.Logf("upload OK from A, mediaKey=%s", mediaKey)

	// Cleanup runs regardless of subsequent assertions.
	defer func() {
		if err := apiA.MoveToTrash([]string{mediaKey}); err != nil {
			t.Logf("cleanup MoveToTrash failed: %v (manual cleanup: mediaKey=%s)", err, mediaKey)
		} else {
			t.Log("cleanup: moved to trash")
		}
	}()

	// Same-account hit.
	hitA, err := apiA.FindRemoteMediaByHash(sum[:])
	if err != nil {
		t.Fatalf("postA: %v", err)
	}
	if hitA == "" {
		t.Fatal("postA hash miss on same account — dedup broken")
	}
	t.Logf("postA hash hit: %s", hitA)

	// Cross-account lookup — the whole point of the test.
	hitB, err := apiB.FindRemoteMediaByHash(sum[:])
	if err != nil {
		t.Fatalf("crossB: %v", err)
	}
	if hitB != "" {
		t.Errorf("CROSS-ACCOUNT DEDUP LEAK: apiB got mediaKey=%s for hash uploaded by A. Driver must NOT rely on FindRemoteMediaByHash across accounts.", hitB)
	} else {
		t.Log("crossB hash miss: OK — dedup confirmed per-account")
	}
}
