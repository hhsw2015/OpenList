package google_photo_native

import (
	"bytes"
	"fmt"
	"net/http"
	"time"

	"github.com/OpenListTeam/OpenList/v4/drivers/google_photo_native/generated"
	"google.golang.org/protobuf/proto"
)

// AlbumItem is a parsed row from the album-list endpoint.
type AlbumItem struct {
	AlbumKey   string
	Title      string
	MediaCount int
}

// AlbumListResult is one page of albums.
type AlbumListResult struct {
	Albums        []AlbumItem
	NextPageToken string
}

// CreateAlbum creates a new album with an optional initial media set. Returns
// the album's own mediaKey (used as the "album folder" identifier).
func (a *Api) CreateAlbum(albumName string, mediaKeys []string) (string, error) {
	protoMediaKeys := make([]*generated.CreateAlbumField4Type, len(mediaKeys))
	for i, key := range mediaKeys {
		protoMediaKeys[i] = &generated.CreateAlbumField4Type{
			Field1: &generated.CreateAlbumField4TypeField1Type{MediaKey: key},
		}
	}
	body := generated.CreateAlbum{
		AlbumName: albumName,
		Timestamp: time.Now().Unix(),
		Field3:    1,
		MediaKeys: protoMediaKeys,
		Field6:    &generated.CreateAlbumField6Type{},
		Field7:    &generated.CreateAlbumField7Type{Field1: 3},
		DeviceInfo: &generated.CreateAlbumField8Type{
			Model:             a.model,
			Make:              a.make,
			AndroidApiVersion: a.androidAPIVersion,
		},
	}
	raw, err := proto.Marshal(&body)
	if err != nil {
		return "", fmt.Errorf("marshal CreateAlbum: %w", err)
	}
	respBytes, err := a.doProtobufPOST("https://photosdata-pa.googleapis.com/6439526531001121323/8386163679468898444", raw)
	if err != nil {
		return "", err
	}
	var pbResp generated.CreateAlbumResponse
	if err := proto.Unmarshal(respBytes, &pbResp); err != nil {
		return "", fmt.Errorf("unmarshal CreateAlbumResponse: %w", err)
	}
	if pbResp.GetField1() == nil {
		return "", fmt.Errorf("CreateAlbum: invalid response structure")
	}
	key := pbResp.GetField1().GetAlbumMediaKey()
	if key == "" {
		return "", fmt.Errorf("CreateAlbum: no album media key returned")
	}
	return key, nil
}

// AddMediaToAlbum appends one or more media items to an existing album.
func (a *Api) AddMediaToAlbum(albumMediaKey string, mediaKeys []string) error {
	body := generated.AddMediaToAlbum{
		MediaKeys:     mediaKeys,
		AlbumMediaKey: albumMediaKey,
		Field5:        &generated.AddMediaToAlbumField5Type{Field1: 2},
		DeviceInfo: &generated.AddMediaToAlbumField6Type{
			Model:             a.model,
			Make:              a.make,
			AndroidApiVersion: a.androidAPIVersion,
		},
		Timestamp: time.Now().Unix(),
	}
	raw, err := proto.Marshal(&body)
	if err != nil {
		return fmt.Errorf("marshal AddMediaToAlbum: %w", err)
	}
	_, err = a.doProtobufPOST("https://photosdata-pa.googleapis.com/6439526531001121323/484917746253879292", raw)
	return err
}

// GetAlbumList fetches one page of albums. pageToken threads response.field1.4
// from the previous call.
func (a *Api) GetAlbumList(pageToken string) (*AlbumListResult, error) {
	requestData := buildAlbumListRequest(pageToken)
	bearer, err := a.BearerToken()
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST",
		"https://photosdata-pa.googleapis.com/6439526531001121323/18047484249733410717",
		bytes.NewReader(requestData))
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
		return nil, fmt.Errorf("GetAlbumList status %d: %s", resp.StatusCode, readErrorBody(resp))
	}
	body, err := readResponseBody(resp)
	if err != nil {
		return nil, err
	}
	albums, token := extractAlbumsFromResponse(body)
	return &AlbumListResult{Albums: albums, NextPageToken: token}, nil
}
