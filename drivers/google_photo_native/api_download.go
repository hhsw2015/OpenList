package google_photo_native

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"

	"github.com/OpenListTeam/OpenList/v4/drivers/google_photo_native/generated"
	"google.golang.org/protobuf/proto"
)

// DownloadURLs is the parsed result of a GetDownloadURLs call.
type DownloadURLs struct {
	OriginalURL string
	EditedURL   string
	Filename    string
}

// GetDownloadURLs asks the private API for a signed download URL. The returned
// URL is fetchable without a bearer token (it is presigned).
func (a *Api) GetDownloadURLs(mediaKey string) (*DownloadURLs, error) {
	protoBody := generated.GetDownloadUrls{
		Field1: &generated.GetDownloadUrlsField1Type{
			Field1: &generated.GetDownloadUrlsField1Field1Type{MediaKey: mediaKey},
		},
		Field2: &generated.GetDownloadUrlsField2Type{
			Field1: &generated.GetDownloadUrlsField2Field1Type{
				Field7: &generated.GetDownloadUrlsField2Field1Field7Type{
					Field2: &generated.GetDownloadUrlsEmpty{},
				},
			},
			Field5: &generated.GetDownloadUrlsField2Field5Type{
				Field2: &generated.GetDownloadUrlsEmpty{},
				Field3: &generated.GetDownloadUrlsEmpty{},
				Field5: &generated.GetDownloadUrlsField2Field5Field5Type{
					Field1: &generated.GetDownloadUrlsEmpty{},
					Field3: 1,
				},
			},
		},
	}
	body, err := proto.Marshal(&protoBody)
	if err != nil {
		return nil, fmt.Errorf("marshal GetDownloadUrls: %w", err)
	}

	bearer, err := a.BearerToken()
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST",
		"https://photosdata-pa.googleapis.com/$rpc/social.frontend.photos.preparedownloaddata.v1.PhotosPrepareDownloadDataService/PhotosPrepareDownload",
		bytes.NewReader(body))
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
		return nil, fmt.Errorf("GetDownloadURLs status %d: %s", resp.StatusCode, readErrorBody(resp))
	}

	var reader io.Reader = resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gz, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("gzip reader: %w", err)
		}
		defer func() { _ = gz.Close() }()
		reader = gz
	}
	respBytes, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	var pbResp generated.GetDownloadUrlsResponse
	if err := proto.Unmarshal(respBytes, &pbResp); err != nil {
		return nil, fmt.Errorf("unmarshal GetDownloadUrlsResponse: %w", err)
	}

	out := &DownloadURLs{}
	f1 := pbResp.GetField1()
	if f1 == nil {
		return out, nil
	}
	if f2 := f1.GetField2(); f2 != nil {
		out.Filename = f2.GetField4()
	}
	f5 := f1.GetField5()
	if f5 == nil {
		return out, nil
	}
	// Videos: URL is under field3.field5.
	if f3 := f5.GetField3(); f3 != nil {
		if vid := f3.GetField5(); vid != "" {
			out.OriginalURL = vid
			return out, nil
		}
	}
	// Photos: original/edited URLs under field2.
	if f2 := f5.GetField2(); f2 != nil {
		out.EditedURL = f2.GetEditedUrl()
		out.OriginalURL = f2.GetOriginalUrl()
	}
	return out, nil
}
