package google_photo_native

import (
	"time"

	"github.com/OpenListTeam/OpenList/v4/internal/model"
)

// PhotoObj is a wrapper around model.Object that carries the two identifier
// forms Google Photos uses: mediaKey (for Link, MoveToTrash) and dedupKey
// (for PermanentlyDelete). model.Obj.GetID() returns the mediaKey; DedupKey
// is accessible via a type assertion inside the driver.
type PhotoObj struct {
	model.Object
	DedupKey string
}

// -----------------------------------------------------------------------------
// Fake-root helpers
// -----------------------------------------------------------------------------

const (
	rootPath      = "root"
	allID         = "all"
	trashID       = "trash"
	albumsID      = "albums"
	albumIDPrefix = "album:"
)

// AlbumDir is a virtual folder representing one Google Photos album.
// GetID() returns "album:<albumMediaKey>"; the album's own key is under
// AlbumKey so driver methods can distinguish it from a plain media obj.
type AlbumDir struct {
	model.Object
	AlbumKey string
}

func rootDirs(includeTrash, includeAlbums bool) []model.Obj {
	dirs := []model.Obj{
		&model.Object{
			ID:       allID,
			Name:     allID,
			IsFolder: true,
			Modified: time.Unix(0, 0),
		},
	}
	if includeAlbums {
		dirs = append(dirs, &model.Object{
			ID:       albumsID,
			Name:     albumsID,
			IsFolder: true,
			Modified: time.Unix(0, 0),
		})
	}
	if includeTrash {
		dirs = append(dirs, &model.Object{
			ID:       trashID,
			Name:     trashID,
			IsFolder: true,
			Modified: time.Unix(0, 0),
		})
	}
	return dirs
}
