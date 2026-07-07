package google_photo_native

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/OpenListTeam/OpenList/v4/internal/driver"
	"github.com/OpenListTeam/OpenList/v4/internal/errs"
	"github.com/OpenListTeam/OpenList/v4/internal/model"
	"github.com/OpenListTeam/OpenList/v4/internal/op"
	streamPkg "github.com/OpenListTeam/OpenList/v4/internal/stream"
	"github.com/OpenListTeam/OpenList/v4/pkg/http_range"
	"github.com/OpenListTeam/OpenList/v4/pkg/utils"
	log "github.com/sirupsen/logrus"
)

// needsSizeProbe reports whether Link should HEAD the signed URL to
// discover the file's true size. Only media types that OpenList's
// server-side ProxyRange handler needs to seek into are worth the
// extra round trip — currently video formats when web_proxy is on.
func needsSizeProbe(lowerName string, storage *model.Storage) bool {
	if storage == nil || (!storage.WebProxy && !storage.ProxyRange) {
		return false
	}
	switch {
	case strings.HasSuffix(lowerName, ".mov"),
		strings.HasSuffix(lowerName, ".m4v"),
		strings.HasSuffix(lowerName, ".mkv"),
		strings.HasSuffix(lowerName, ".webm"),
		strings.HasSuffix(lowerName, ".avi"),
		strings.HasSuffix(lowerName, ".mpg"),
		strings.HasSuffix(lowerName, ".mpeg"),
		strings.HasSuffix(lowerName, ".3gp"),
		strings.HasSuffix(lowerName, ".3g2"),
		strings.HasSuffix(lowerName, ".wmv"),
		strings.HasSuffix(lowerName, ".ts"),
		strings.HasSuffix(lowerName, ".m2ts"),
		strings.HasSuffix(lowerName, ".mts"):
		return true
	}
	return false
}

type GooglePhotoNative struct {
	model.Storage
	Addition
	api *Api
}

func (d *GooglePhotoNative) Config() driver.Config {
	return config
}

func (d *GooglePhotoNative) GetAddition() driver.Additional {
	return &d.Addition
}

// Init runs the auth bootstrap or reconnects with cached credentials.
// See DESIGN.md §4 for the state machine.
func (d *GooglePhotoNative) Init(ctx context.Context) error {
	if d.MasterToken != "" && d.Email != "" && d.AndroidID != "" {
		return d.buildClient()
	}
	if d.OAuthToken == "" {
		return fmt.Errorf("no credentials. Sign in at https://accounts.google.com/EmbeddedSetup, copy the 'oauth_token' cookie into the OAuthToken field, and save.")
	}
	if d.AndroidID == "" {
		id, err := RandomAndroidID()
		if err != nil {
			return fmt.Errorf("generate android id: %w", err)
		}
		d.AndroidID = id
	}
	email, master, err := ExchangeOAuthToken(d.OAuthToken, d.AndroidID)
	if err != nil {
		return fmt.Errorf("exchange oauth_token: %w", err)
	}
	d.Email = email
	d.MasterToken = master
	// Build the client before clearing OAuthToken. If buildClient fails
	// (bad proxy config, etc.) the caller can retry Init without needing
	// a fresh EmbeddedSetup cookie — the oauth_token would already be
	// dead server-side but at least the storage stays fixable.
	if err := d.buildClient(); err != nil {
		return err
	}
	d.OAuthToken = "" // one-shot; the cookie is dead server-side too
	op.MustSaveDriverStorage(d)
	return nil
}

func (d *GooglePhotoNative) buildClient() error {
	client, err := newHTTPClientWithProxy(d.UpstreamProxy)
	if err != nil {
		return fmt.Errorf("http client: %w", err)
	}
	lang := d.Language
	if lang == "" {
		lang = "en"
	}
	auth := buildAuthString(d.Email, d.MasterToken, d.AndroidID, lang)
	d.api = newApi(auth, d.Email, lang, client)
	return nil
}

func (d *GooglePhotoNative) Drop(ctx context.Context) error {
	d.api = nil
	return nil
}

// List renders a fake namespace: a virtual root with all/ (and optionally
// albums/, trash/), plus the media list under all/ paginated up to
// MaxListItems, and album contents under albums/<album>.
func (d *GooglePhotoNative) List(ctx context.Context, dir model.Obj, args model.ListArgs) ([]model.Obj, error) {
	if d.api == nil {
		return nil, fmt.Errorf("driver not initialized")
	}
	id := dir.GetID()
	switch {
	case id == rootPath, id == "", id == d.RootFolderID:
		return rootDirs(d.RequestTrashItems, d.ShowAlbums), nil
	case id == allID, id == trashID:
		return d.listAll(ctx)
	case id == albumsID:
		return d.listAlbums(ctx)
	case strings.HasPrefix(id, albumIDPrefix):
		return d.listAlbumMedia(ctx, strings.TrimPrefix(id, albumIDPrefix))
	default:
		return nil, errs.ObjectNotFound
	}
}

// listAlbums fetches one page of albums under albums/. Currently caps at
// the first page (~30 albums). Multi-page would follow the same pattern as
// listAll but is unlikely to hit users with small collections.
func (d *GooglePhotoNative) listAlbums(ctx context.Context) ([]model.Obj, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	result, err := d.api.GetAlbumList("")
	if err != nil {
		return nil, fmt.Errorf("GetAlbumList: %w", err)
	}
	out := make([]model.Obj, 0, len(result.Albums))
	for i := range result.Albums {
		a := &result.Albums[i]
		if a.AlbumKey == "" {
			continue
		}
		name := a.Title
		if name == "" {
			name = a.AlbumKey
		}
		out = append(out, &AlbumDir{
			Object: model.Object{
				ID:       albumIDPrefix + a.AlbumKey,
				Name:     name,
				IsFolder: true,
				Modified: time.Unix(0, 0),
			},
			AlbumKey: a.AlbumKey,
		})
	}
	return out, nil
}

// listAlbumMedia: an album's own contents are not exposed via a distinct
// list endpoint in the private API; the only reliable path is to filter
// the full media list by album membership, which is O(library). For v2
// we return an empty listing and rely on album creation via MakeDir +
// Move (AddMediaToAlbum) to give users a functional workflow. Full
// per-album contents view is deferred to v3.
//
// ponytail: empty list keeps UX honest; add real filtering when there is
// a demand signal.
func (d *GooglePhotoNative) listAlbumMedia(ctx context.Context, albumKey string) ([]model.Obj, error) {
	_ = albumKey
	return []model.Obj{}, nil
}

func (d *GooglePhotoNative) listAll(ctx context.Context) ([]model.Obj, error) {
	limit := d.MaxListItems
	if limit <= 0 {
		limit = 5000
	}
	pageSize := 100
	if limit < pageSize {
		pageSize = limit
	}

	var out []model.Obj
	seen := make(map[string]struct{})
	var pageToken, prevToken string
	for len(out) < limit {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		remaining := limit - len(out)
		curPage := pageSize
		if remaining < curPage {
			curPage = remaining
		}
		result, err := d.api.GetMediaList(pageToken, "", 2, curPage, d.RequestTrashItems)
		if err != nil {
			return nil, fmt.Errorf("GetMediaList: %w", err)
		}
		for i := range result.Items {
			item := &result.Items[i]
			if item.MediaKey == "" {
				continue
			}
			if _, dup := seen[item.MediaKey]; dup {
				continue
			}
			seen[item.MediaKey] = struct{}{}
			obj := &PhotoObj{
				Object: model.Object{
					ID:       item.MediaKey,
					Name:     photoName(item),
					Size:     0, // unknown until Link resolves it; harmless
					Modified: time.Unix(item.Timestamp, 0),
					IsFolder: false,
				},
				DedupKey: item.DedupKey,
			}
			out = append(out, obj)
		}
		if result.NextPageToken == "" || len(result.Items) == 0 {
			break
		}
		// Break out of a pagination loop that keeps yielding the same
		// token — misbehaving endpoint or stale cursor.
		if result.NextPageToken == prevToken {
			break
		}
		prevToken = pageToken
		pageToken = result.NextPageToken
	}
	return out, nil
}

func photoName(item *MediaItem) string {
	if item.Filename != "" {
		return item.Filename
	}
	// Fallback: mediaKey prefix + extension guess. Better than empty.
	key := item.MediaKey
	if len(key) > 12 {
		key = key[:12]
	}
	ext := ".bin"
	switch item.MediaType {
	case "video":
		ext = ".mp4"
	case "photo":
		ext = ".jpg"
	}
	return key + ext
}

// Link returns a signed direct URL for real media, or a disguise-aware
// RangeReader when the on-wire item carries a disguise trailer. Detection is
// content-based (see DESIGN.md §7).
func (d *GooglePhotoNative) Link(ctx context.Context, file model.Obj, args model.LinkArgs) (*model.Link, error) {
	if d.api == nil {
		return nil, fmt.Errorf("driver not initialized")
	}
	urls, err := d.api.GetDownloadURLs(file.GetID())
	if err != nil {
		return nil, err
	}
	url := urls.OriginalURL
	if url == "" {
		url = urls.EditedURL
	}
	if url == "" {
		return nil, fmt.Errorf("no download URL for mediaKey %s", file.GetID())
	}

	name := strings.ToLower(file.GetName())

	// Probe the signed URL for its true size when the server-side proxy
	// or range emulation actually needs it. Skip the round-trip for pure
	// image formats served from the direct URL — they don't need a size
	// hint to render, and probing every gallery thumbnail is wasteful.
	if !strings.HasSuffix(name, ".mp4") {
		if needsSizeProbe(name, d.GetStorage()) {
			size, err := probeContentLength(ctx, url, d.api.client)
			if err != nil {
				log.Warnf("google_photo_native: probe %s: %v (Link returned without ContentLength; seeking may not work)", name, err)
				return &model.Link{URL: url}, nil
			}
			return &model.Link{URL: url, ContentLength: size}, nil
		}
		return &model.Link{URL: url}, nil
	}

	rr := newDisguiseRangeReader(url, file.GetSize(), d.api.client)
	if err := rr.inspect(ctx); err != nil {
		// Head-fetch itself failed (network / auth). Propagate rather
		// than silently returning the wrapped URL — a disguised file
		// downloaded as `.mp4 with trailer` looks like data corruption.
		return nil, fmt.Errorf("disguise probe failed: %w", err)
	}
	if !rr.IsDisguised() {
		// Real MP4: no wrapper. Return the signed URL with the wire size
		// so proxy_range can emulate seek.
		return &model.Link{URL: url, ContentLength: rr.PayloadSize()}, nil
	}
	// Disguised: proxy via RangeReader so the client sees the payload only.
	return &model.Link{
		RangeReader:   rr,
		ContentLength: rr.PayloadSize(),
	}, nil
}

// Put uploads a file. Non-media files are wrapped in an MP4 disguise when
// DisguiseNonMedia is on. Every upload writes the payload (or wrapper) to a
// tmp file, hashes it with SHA-1, then GetUploadToken → PUT → CommitUpload.
func (d *GooglePhotoNative) Put(ctx context.Context, dstDir model.Obj, file model.FileStreamer, up driver.UpdateProgress) error {
	if d.api == nil {
		return fmt.Errorf("driver not initialized")
	}

	origName := file.GetName()
	disguise := d.DisguiseNonMedia && !IsMediaFilename(origName)

	// Materialize to tmp with SHA-1 in one pass.
	tmpFile, sha1Hex, err := streamPkg.CacheFullAndHash(file, &up, utils.SHA1)
	if err != nil {
		return fmt.Errorf("cache and hash: %w", err)
	}
	payloadPath := ""
	if f, ok := tmpFile.(*os.File); ok {
		payloadPath = f.Name()
	}
	size := file.GetSize()

	uploadName := origName
	uploadSize := size
	uploadSHA1Hex := sha1Hex
	if disguise {
		if payloadPath == "" {
			return fmt.Errorf("disguise requires a tmpfile-backed stream")
		}
		disguisedPath, err := HideAsMP4(payloadPath, "")
		if err != nil {
			return fmt.Errorf("disguise: %w", err)
		}
		defer func() { _ = os.Remove(disguisedPath) }()
		info, err := os.Stat(disguisedPath)
		if err != nil {
			return fmt.Errorf("disguise stat: %w", err)
		}
		hexHash, err := sha1OfFile(disguisedPath)
		if err != nil {
			return fmt.Errorf("hash disguise: %w", err)
		}
		uploadName = origName + disguiseSuffix
		uploadSize = info.Size()
		uploadSHA1Hex = hexHash
		payloadPath = disguisedPath
	}

	sha1Bytes, err := hex.DecodeString(uploadSHA1Hex)
	if err != nil {
		return fmt.Errorf("decode sha1: %w", err)
	}
	sha1B64 := base64.StdEncoding.EncodeToString(sha1Bytes)

	// Dedup only helps native uploads. Skipping for disguised paths is a
	// deliberate no-op: disguise SHA-1 varies with the embedded filename.
	if !d.ForceUpload && !disguise {
		if mediaKey, err := d.api.FindRemoteMediaByHash(sha1Bytes); err == nil && mediaKey != "" {
			// Signal completion so the caller's progress bar reaches 100%.
			up(100)
			_ = mediaKey
			return nil
		}
	}

	// Reopen the payload for the PUT — CacheFullAndHash left it seeked at
	// EOF, and HideAsMP4's disguisedPath is a plain file too.
	reader, err := os.Open(payloadPath)
	if err != nil {
		return fmt.Errorf("reopen payload: %w", err)
	}
	defer func() { _ = reader.Close() }()

	uploadID, err := d.api.GetUploadToken(sha1B64, uploadSize)
	if err != nil {
		return fmt.Errorf("GetUploadToken: %w", err)
	}
	tok, err := d.api.UploadStream(ctx, uploadID, driver.NewLimitedUploadStream(ctx, reader), uploadSize)
	if err != nil {
		return fmt.Errorf("UploadStream: %w", err)
	}

	quality := QualityOriginal
	if d.Saver {
		quality = QualitySaver
	} else if d.UseQuota {
		quality = QualityQuota
	}
	mtime := file.ModTime().Unix()
	if mtime <= 0 {
		mtime = time.Now().Unix()
	}
	_, err = d.api.CommitUpload(tok, uploadName, sha1Bytes, mtime, quality)
	if err != nil {
		return fmt.Errorf("CommitUpload: %w", err)
	}
	return nil
}

func (d *GooglePhotoNative) Remove(ctx context.Context, obj model.Obj) error {
	if d.api == nil {
		return fmt.Errorf("driver not initialized")
	}
	if d.PermanentDelete {
		if po, ok := obj.(*PhotoObj); ok && po.DedupKey != "" {
			return d.api.PermanentlyDelete([]string{po.DedupKey})
		}
		// Fallback: no dedupKey → trash instead of permanent.
	}
	return d.api.MoveToTrash([]string{obj.GetID()})
}

// MakeDir under albums/ creates a new album with the given name.
// Anywhere else, unsupported.
func (d *GooglePhotoNative) MakeDir(ctx context.Context, parentDir model.Obj, dirName string) error {
	if d.api == nil {
		return fmt.Errorf("driver not initialized")
	}
	if parentDir.GetID() != albumsID {
		return errs.NotSupport
	}
	_, err := d.api.CreateAlbum(dirName, nil)
	return err
}

// Move from any media obj into an album adds it to that album. Google
// Photos allows one media item to belong to many albums, so this is an
// addition — the source is not removed. This is the closest mapping to
// filesystem semantics we can offer.
func (d *GooglePhotoNative) Move(ctx context.Context, srcObj, dstDir model.Obj) error {
	if d.api == nil {
		return fmt.Errorf("driver not initialized")
	}
	album, ok := dstDir.(*AlbumDir)
	if !ok {
		return errs.NotSupport
	}
	return d.api.AddMediaToAlbum(album.AlbumKey, []string{srcObj.GetID()})
}

func (d *GooglePhotoNative) Rename(ctx context.Context, srcObj model.Obj, newName string) error {
	return errs.NotSupport
}
func (d *GooglePhotoNative) Copy(ctx context.Context, srcObj, dstDir model.Obj) error {
	if d.api == nil {
		return fmt.Errorf("driver not initialized")
	}
	album, ok := dstDir.(*AlbumDir)
	if !ok {
		return errs.NotSupport
	}
	// Photos has no true copy; adding to album is idempotent duplication
	// with a filesystem-like read.
	return d.api.AddMediaToAlbum(album.AlbumKey, []string{srcObj.GetID()})
}

var _ driver.Driver = (*GooglePhotoNative)(nil)

// RangeRead adapter for the disguise range reader is exposed via the
// standard model.RangeReaderIF interface directly; no additional glue needed.
var _ model.RangeReaderIF = (*disguiseRangeReader)(nil)

// unused: pinned for compile clarity
var _ = http_range.Range{}
