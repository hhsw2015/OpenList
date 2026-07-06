# google_photo_native — Design Doc

Integrate xob0t/gotohp (`~/Dev/gotohp_rev`) into OpenList as a new
storage driver whose main selling point is **unlimited free storage** on
Google Photos via MP4 disguising.

- Status: proposal, not merged.
- Owner: wowdd1
- Target: OpenList `main`, backend + frontend
- Revision: 2 (post-review).

---

## 1. Motivation

OpenList already ships `drivers/google_photo/` (OAuth + Photos Library
API v1). It is `NoUpload:true`, only lists/downloads.

gotohp talks the private Android `photos.native` API. That API has
three properties OpenList users care about:

- **Zero-quota uploads.** Google Photos "original quality" ingest does
  not re-encode payloads. Any bytes appended after a valid MP4
  container survive the upload/download round-trip. gotohp disguises
  arbitrary files as a 1570-byte MP4 → Google stores them for free.
- **SHA-1 dedup / instant upload.** `FindRemoteMediaByHash` short-
  circuits a re-upload when the same content already exists in the
  account (per-account, not cross-account — safe).
- **Delete / album / thumbnail.** All missing in the OAuth driver.

Combined: turn a Google Photos account into an unbounded object store
mounted via OpenList (WebDAV / S3 / SMB / fs).

## 2. Non-goals

- Not replacing `drivers/google_photo/` (OAuth). Ship side-by-side.
- Not shipping the token-binding (rooted-device) auth path in v1
  (skips `tink-crypto` dep).
- Not shipping wash/autowash quota-migration tooling.
- Not exposing the private CLI/GUI of gotohp; only the storage surface.

## 3. Architecture

### 3.1 Directory layout

```
drivers/google_photo_native/
  meta.go              # driver.Config, Addition, init()
  driver.go            # Driver interface impls
  types.go             # obj adaptor, MediaItem, DownloadURLs
  auth.go              # gpsoauth: ExchangeOAuthToken, RandomAndroidID
  api_client.go        # Api struct, BearerToken (mutex-guarded), getAuthToken
  api_upload.go        # GetUploadToken, FindRemoteMediaByHash, doUpload, CommitUpload
  api_list.go          # GetMediaList + protobuf request builder + response parser
  api_download.go      # GetDownloadURLs, GetMediaInfo
  api_trash.go         # MoveToTrash, PermanentlyDelete
  disguise.go          # OpenDisguiseReader, DisguiseSize, disguise head parser
  disguise_range.go    # streaming disguise-stripping RangeReader for Link
  http.go              # HTTP client with proxy, retry, gzip
  filename.go          # isSupportedByGooglePhotos allow-list
  medialist_template.go # protobuf template for GetMediaList request
  generated/           # vendored *.pb.go from gotohp (~11.3k LOC, unmodified)
  DESIGN.md            # this file
```

Estimated hand-written: **~2200 LOC**. Vendored protobuf: **~11.3k
LOC** (no maintenance burden, generated).

Register in `drivers/all.go`:
```go
_ "github.com/OpenListTeam/OpenList/v4/drivers/google_photo_native"
```

### 3.2 New dependencies

- `google.golang.org/protobuf` — already indirect in OpenList `go.mod`.
- **No new modules** for v1. `tink-crypto/tink-go` skipped
  (no token-binding). `go.etcd.io/bbolt` not needed
  (see §7.3 for the no-sidecar decision).

## 4. Auth flow

### 4.1 Model

The driver holds three fields as long-term state:

| Field         | Source                                              | Lifetime      |
|---------------|-----------------------------------------------------|---------------|
| `Email`       | returned by `ExchangeOAuthToken`                    | until revoked |
| `MasterToken` | returned by `ExchangeOAuthToken` (aas_et)           | until revoked |
| `AndroidID`   | random 16-hex on first `Init`, then fixed           | forever       |

Short-lived bearer tokens are minted on demand from
`MasterToken + AndroidID` via `/auth`. Bearer expiry honored (`Expiry`
field), refreshed 30 s early. **`BearerToken()` must be mutex-guarded
— multiple OpenList requests hit the same `Api` concurrently.**

### 4.2 Bootstrap (one-time)

User captures a cookie from a signed-in browser session:

1. Open `https://accounts.google.com/EmbeddedSetup` in a private
   window.
2. Sign in normally (2FA / passkey all supported).
3. DevTools → Application → Cookies → `https://accounts.google.com`
   → copy the value of the cookie named `oauth_token`.
4. Paste it into the driver's `OAuthToken` field.

`Init` consumes `OAuthToken` exactly once:

```go
if d.MasterToken != "" && d.Email != "" && d.AndroidID != "" {
    return d.buildClient()
}
if d.OAuthToken == "" {
    return fmt.Errorf("provide oauth_token from https://accounts.google.com/EmbeddedSetup")
}
if d.AndroidID == "" {
    id, err := RandomAndroidID()
    if err != nil {
        return err
    }
    d.AndroidID = id
}
email, master, err := ExchangeOAuthToken(d.OAuthToken, d.AndroidID)
if err != nil {
    return fmt.Errorf("exchange oauth_token: %w", err)
}
d.Email, d.MasterToken = email, master
d.OAuthToken = ""                 // consumed, cookie already dead server-side
op.MustSaveDriverStorage(d)       // persist before anything else can run
return d.buildClient()
```

Rationale for each line documented at the code site.

### 4.3 Re-authentication

If `getAuthToken` returns a hard auth error, driver clears
`MasterToken` (keeps `Email` + `AndroidID`), persists, returns an
error. User re-edits storage, pastes a new `oauth_token`, saves, Init
re-runs the exchange branch.

## 5. Namespace / directory model

Google Photos has no folder tree. The driver synthesizes one:

```
/                    (fake root)
├── all/             all media items, paginated
├── albums/          (v2) list of album containers
│   └── <album>/     (v2) media in that album
└── trash/           (optional, RequestTrashItems flag)
```

- `dir.GetID()` encodes fetch mode: `all`, `albums`, `album:<key>`,
  `trash`.
- Root listing returns virtual dirs.

Rename / cross-album move are not supported (Google's model, no useful
mapping). Returned as `errs.NotSupport`.

## 6. Upload path

### 6.1 Media-native files

```
tmpFile, sha1Hex, err := stream.CacheFullAndHash(file, up, utils.SHA1)
sha1Bytes, _ := hex.DecodeString(sha1Hex)
sha1B64      := base64.StdEncoding.EncodeToString(sha1Bytes)

if !d.ForceUpload {
    if mediaKey, _ := api.FindRemoteMediaByHash(sha1Bytes); mediaKey != "" {
        return existingObj(mediaKey), nil          // instant upload
    }
}

uploadToken, _ := api.GetUploadToken(sha1B64, size)     // POST, returns X-GUploader-UploadID
commitToken, _ := api.doUploadRequest(ctx, uploadURL, tmpFile) // HTTP PUT chunked
mediaKey, _    := api.CommitUpload(commitToken, name, sha1Bytes, mtime)
```

Notes:

- `GetUploadToken` requires the SHA-1 **before** the PUT — hence
  the eager `CacheFullAndHash`. Big files spill to `TempDir`, same
  cost as every other hash-required driver.
- The PUT is single-request chunked-transfer, no `Content-Length`.
  Progress wrapped via `driver.ReaderUpdatingProgress`.
- Bearer refreshed inside each helper; a 401 during PUT retries once
  after refresh (gotohp already does this).
- `CommitUpload.Quality`:
  - `UseQuota=false && Saver=false` (default) → 3 = original,
    zero-quota on the disguise path.
  - `Saver=true` → 1 = "Storage saver".
  - `UseQuota=true` → force to count against quota (debug knob).

### 6.2 Non-media files (disguise)

```
if !IsMediaFilename(name) && d.DisguiseNonMedia {
    reader, total, _ := OpenDisguiseReader(tmpFile, name)
    // Streams: [mp4Template] [magic] [uint32 nameLen] [name] [payload]
    // total   = DisguiseSize(name, size)
    // The wire filename becomes <origName>.mp4, containing:
    //   - a 1570-byte valid MP4 cover
    //   - a magic separator ("FILE_DATA_BEGIN")
    //   - uint32 LE nameLen + name
    //   - the payload bytes
    // Hash for GetUploadToken is computed over the wrapper, not the
    // payload — that means SHA-1 dedup does NOT apply to disguised
    // uploads. Acceptable: point of disguise is to store arbitrary
    // unique bytes.
}
```

Hashing detail: `GetUploadToken` needs the SHA-1 of what will actually
be sent on the wire, i.e. the disguised bytes. Implementation:

1. `stream.CacheFullAndHash(file, up, utils.SHA1)` → payload cached
   on disk, `payloadSHA1` (unused for the upload, discarded).
2. `disguiseReader, total := OpenDisguiseReader(tmpFile, origName)`.
3. `wrapperHash := sha1.New()`; wrap `disguiseReader` with
   `io.TeeReader(disguiseReader, wrapperHash)`.
4. Do a **preflight pass** that only feeds the tee to `io.Discard` to
   compute `wrapperSHA1`. Reset `disguiseReader` by re-opening it
   (`OpenDisguiseReader` again on the same tmpFile).
5. `GetUploadToken(wrapperSHA1B64, total)` → PUT the fresh reader.

The preflight pass is one full read of the disguise stream (mostly
the payload). Cost: same order as the actual upload's local read.
Acceptable — disguise is opt-in, users understand the cost.

Simpler alternative for MVP: materialize the disguised bytes to a
second tmp file via `HideAsMP4`, hash it, upload it. Two disk writes
per disguised upload but zero clever streaming; we pick the simpler
path in v1 and revisit if benchmarks demand it. (`ponytail: two
writes for now, single-pass streaming when profiling proves it
matters`.)

## 7. Download path (Link)

### 7.1 Native media

```
d := api.GetDownloadURLs(mediaKey)
url := d.OriginalURL || d.EditedURL
return &model.Link{URL: url}, nil
```

Signed URL, no bearer needed, direct 302 to Google CDN.

### 7.2 Detecting disguised files

The driver cannot tell from a filename alone whether an item is a
real MP4 or a wrapped payload — the user's library may contain both.
Detection is **content-based**, done lazily inside `Link`:

1. Fetch the signed URL's first
   `len(mp4Template) + disguiseTailSearchWindow` (~66 KB) bytes via
   a `Range: bytes=0-…` GET.
2. Look for the `FILE_DATA_BEGIN` magic separator right after the
   MP4 cover (same logic as `disguise.go:TryExtractDisguised`).
3. **Miss** → real media. Return `Link{URL: signedURL}` for a 302.
4. **Hit** → parse the trailer, extract `origName` and
   `payloadStart`. Return a `RangeReader`-backed `Link` that shifts
   every subsequent `[off, off+len)` request to
   `[payloadStart+off, payloadStart+off+len)` upstream.

Consequences:

- Every disguised-file access pays one head-fetch (~66 KB) on first
  hit. Cached in-memory per mediaKey (LRU, 1024 entries) so
  subsequent range reads reuse the parse.
- The head-fetch is unavoidable to distinguish real MP4 from
  disguised MP4 without extra state.
- Display filename still ends in `.mp4` in List (because that is the
  wire name Google stored). Users see `foo.zip.mp4`, download it,
  and get `foo.zip` bytes — the extension is the only visible clue.
  Improving this needs List-time detection (a scanner Phase-2
  option) or per-item head-fetch during List (too expensive).

### 7.3 Why no sidecar cache

An earlier draft proposed a bolt sidecar for
`mediaKey → {origName, payloadStart, payloadSize}`. **Dropped**
because:

- The trailer itself carries `origName` and `payloadStart`. One
  ~66 KB head-fetch decodes it. In-memory LRU amortizes the cost.
- Sidecar introduces lifecycle plumbing (Drop cleanup, migration,
  backup) with no dividend when the head-fetch is already cheap.

Consequence for List: report the wire size (Google's total).
Disguised items are off by `~1600 + nameLen` bytes — visible in
`ls`, harmless in download (the RangeReader corrects
`ContentLength` on the returned `Link`). Users see slightly-inflated
sizes in the file browser; the bytes they download are correct.

## 8. Delete / trash

- OpenList `Remove(obj)` → `MoveToTrash([mediaKey])` (soft delete,
  60-day recovery). **Note the API is called with mediaKey values
  despite its `dedupKeys` parameter name — gotohp comment confirms
  this.**
- Driver option `PermanentDelete bool` (default false) switches to
  `PermanentlyDelete([dedupKey])`. This requires plumbing dedupKey
  through the internal obj type (List extracts both keys already).

## 9. Overwrite policy

`NoOverwriteUpload:false`. Rationale: OpenList's overwrite handling
with `NoOverwriteUpload:true` **renames the existing file** to a
`.openlist_to_delete` temp — Photos does not support rename, so that
path is unusable. Instead:

- Driver `Rename` returns `errs.NotSupport`.
- Google Photos natively allows duplicate names in the same "album";
  Put always creates a new item.
- SHA-1 dedup + `FindRemoteMediaByHash` collapses same-content
  re-uploads to the original mediaKey, so *native-file* content is
  not duplicated even if the client attempts an "overwrite".
- Disguised uploads never dedup (hash is computed over the wrapper,
  which varies with the embedded filename). Re-uploading the same
  disguised file produces a fresh mediaKey. Rare in practice
  (users don't re-upload identical archives), but worth stating.

## 10. Concurrency

- `Api.authResponseCache` is read/written by every request. gotohp
  is single-threaded; OpenList is not.
- Wrap all bearer reads and the `/auth` refresh in one `sync.Mutex`.
  Refresh is ~200 ms and rare; contention is a non-issue at
  OpenList's request rates.
- Holding the mutex across the whole refresh also solves in-flight
  coalescing: two racing requests serialize, and the second one
  sees the fresh token on unlock.

## 11. List pagination policy

Photos accounts routinely have 10 k – 500 k items. Full sync = 100 -
5000 pages of 100 items each.

- Full sync per List call is unusable (30 s – 30 min first request).
- OpenList's dir cache (`NoCache:false`) helps subsequent hits but
  the first is what users feel.

Trade-off:

- `MaxListItems` Addition, default `5000` (50 pages ≈ 5 s).
- `SyncToken` (incremental) is Phase 3.
- Users with big libraries edit the number or wait for v3.

Ponytail comment on the field: `// ponytail: full-sync fallback, add
syncToken-based incremental when users hit the ceiling`.

## 12. Addition (driver config)

```go
type Addition struct {
    driver.RootID

    // Auth
    OAuthToken  string `json:"oauth_token" help:"One-time cookie from https://accounts.google.com/EmbeddedSetup. Consumed on save."`
    MasterToken string `json:"master_token"` // auto-filled after first Init
    Email       string `json:"email"`        // auto-filled
    AndroidID   string `json:"android_id"`   // auto-generated
    Language    string `json:"language" default:"en"`
    Proxy       string `json:"proxy"`

    // Storage policy
    UseQuota    bool `json:"use_quota"    default:"false"` // false = free-unlimited path
    Saver       bool `json:"saver"        default:"false"`
    ForceUpload bool `json:"force_upload" default:"false"`

    // Namespace toggles
    RequestTrashItems bool `json:"request_trash_items" default:"false"`
    ShowAlbums        bool `json:"show_albums"         default:"false"` // v2

    // Behaviour
    DisguiseNonMedia bool `json:"disguise_non_media"  default:"true"`
    PermanentDelete  bool `json:"permanent_delete"    default:"false"`
    MaxListItems     int  `json:"max_list_items"      default:"5000"`
}

var config = driver.Config{
    Name:              "GooglePhotoNative",
    DefaultRoot:       "root",
    LocalSort:         true,
    OnlyProxy:         false,      // native: 302; disguised: RangeReader
    NoOverwriteUpload: false,      // see §9
    Alert:             "warning|Uses a reverse-engineered Google Photos API. May stop working without notice. Zero-quota disguise ships arbitrary bytes; do not rely on it for critical data.",
}
```

`Alert` uses OpenList's `severity|message` format
(see `drivers/aliyundrive/meta.go` for precedent).

## 13. Frontend touches (OpenList-Frontend)

In `src/lang/*/drivers.json` per locale:

1. Field-label bundle:
   ```json
   "GooglePhotoNative": {
     "oauth_token": "OAuth token (one-time)",
     "oauth_token-tips": "Sign in at https://accounts.google.com/EmbeddedSetup, then copy the 'oauth_token' cookie. Consumed on save.",
     "master_token": "Master token (auto-filled)",
     "email": "Email (auto-filled)",
     "android_id": "Device ID (auto-generated)",
     "language": "Language",
     "proxy": "Proxy",
     "use_quota": "Count against Google Photos quota",
     "saver": "Use storage-saver quality",
     "force_upload": "Skip SHA1 dedup lookup",
     "request_trash_items": "Show trashed items",
     "show_albums": "Show albums directory",
     "disguise_non_media": "Disguise non-media as MP4 (recommended for unlimited storage)",
     "permanent_delete": "Permanently delete (skip trash)",
     "max_list_items": "Max items per List (0 = unlimited, slower)"
   }
   ```
2. In each locale's driver map register `"GooglePhotoNative": {}` and
   in the display-name map `"GooglePhotoNative": "GooglePhotoNative"`.

Nothing else in the frontend needs changes — driver form is
schema-driven.

## 14. Risks

1. **API drift.** All endpoints are private; Google can change wire
   format at any time. Vendored protobuf definitions let a
   determined user rebuild against a captured schema. Disguise
   magic (`FILE_DATA_BEGIN`) is stable — matches xob0t/gp_disguise
   Python — so third-party payloads keep working.
2. **Account revocation.** Bulk upload of large opaque blobs may
   trip anti-abuse. `Alert:warning` says so. No automatic rate
   limiting in v1; OpenList global upload concurrency applies.
3. **`master_token` at rest.** Stored plaintext in OpenList's DB,
   same as every other driver's refresh_token. Acceptable v1.
4. **First-request List latency.** See §11. Mitigated by
   `MaxListItems` cap; incremental sync is v3.
5. **Disguised file size mismatch.** See §7.3. Cosmetic in file
   browsers; downloads are correct.
6. **Big-file disk usage on upload.** `stream.CacheFullAndHash`
   spills to `TempDir`. Standard for hash-required drivers. No new
   risk.

## 15. Implementation phases

- **v1 — MVP** (~2 days)
  - Vendor `generated/`, `disguise.go`, port `gpsoauth.go`.
  - `meta.go` + `driver.go` skeleton.
  - Init: one-shot OAuthToken exchange, persist master.
  - List: fake root + `all/` (paginated up to `MaxListItems`).
  - Put: native + disguise streaming.
  - Link: native URL + disguise streaming RangeReader (§7.2).
  - Remove: MoveToTrash.
  - Frontend locale entries.

- **v2 — Albums + thumbnails** (~1 day)
  - `MakeDir` → CreateAlbum; `Move` → AddMediaToAlbum.
  - `GetAlbumList` under `albums/`.
  - `model.Thumb` via `api.GetThumbnail`.

- **v3 — Optional**
  - Incremental sync via `syncToken`.
  - Token-binding auth (rooted-device users, `tink-crypto`).

## 16. Test plan

- Unit: disguise round-trip
  (`HideAsMP4` → `OpenDisguiseReader` → `TryExtractDisguised` bytes
  match). Already tested in gotohp; port the test.
- Unit: `ExchangeOAuthToken` with a mock `/auth` server (Google
  returns key=value form-encoded body).
- Manual: end-to-end upload of a `test.zip` → List shows
  `test.zip` → Link → download → decompresses cleanly.
- Manual: upload a native `.jpg` → dedup: re-upload returns same
  mediaKey without a second PUT.
- Manual: pop the tab, revoke master, re-Init prompts for a fresh
  `oauth_token`.
- Manual: mount via WebDAV, `ls -la` a directory of ~5000 items,
  time first vs cached response.
- Manual: cross-account dedup smoke test — upload a rare-hash file
  from account A, run hash lookup from account B; expect miss.
  gotohp assumes per-account dedup; verify before we depend on it.

## 17. Open questions

- **Photos anti-abuse thresholds.** Publish observed rate limits in
  README once we see them in practice.
- **Migration story for existing `google_photo` OAuth users.**
  Recommendation: no auto-migration — capabilities differ; keep
  both drivers, mention the choice in README.

## 18. Out of scope

- Multi-account inside one storage instance (users add the driver
  N times).
- Encryption of disguised payloads (users encrypt before OpenList
  sees the bytes if they care).
- Automatic album cover / metadata management.
- gotohp's wash / autowash quota-migration workflow.
