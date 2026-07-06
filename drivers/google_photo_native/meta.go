package google_photo_native

import (
	"github.com/OpenListTeam/OpenList/v4/internal/driver"
	"github.com/OpenListTeam/OpenList/v4/internal/op"
)

// Addition holds every persistent knob for a single storage instance.
type Addition struct {
	driver.RootID

	// One-time OAuth cookie value from the EmbeddedSetup browser login.
	// Consumed on first Init: exchanged for MasterToken + Email, then cleared
	// and persisted via op.MustSaveDriverStorage.
	OAuthToken string `json:"oauth_token" help:"One-time cookie. Sign in at https://accounts.google.com/EmbeddedSetup, DevTools > Application > Cookies, copy the value of 'oauth_token'. Consumed on save."`

	// Long-lived credentials, auto-filled after the first exchange.
	MasterToken string `json:"master_token" help:"Auto-filled after first save. Do not edit."`
	Email       string `json:"email" help:"Auto-filled."`
	AndroidID   string `json:"android_id" help:"Auto-generated on first save."`

	Language      string `json:"language" default:"en"`
	UpstreamProxy string `json:"upstream_proxy" help:"HTTP(S) proxy URL for Google API calls, e.g. http://127.0.0.1:8080"`

	// Storage policy
	UseQuota    bool `json:"use_quota"    default:"false"`
	Saver       bool `json:"saver"        default:"false"`
	ForceUpload bool `json:"force_upload" default:"false"`

	// Namespace / behaviour
	RequestTrashItems bool `json:"request_trash_items" default:"false"`
	ShowAlbums        bool `json:"show_albums"         default:"true"`
	DisguiseNonMedia  bool `json:"disguise_non_media"  default:"true"`
	PermanentDelete   bool `json:"permanent_delete"    default:"false"`
	MaxListItems      int  `json:"max_list_items"      default:"5000"`
}

var config = driver.Config{
	Name:        "GooglePhotoNative",
	DefaultRoot: rootPath,
	LocalSort:   true,
	Alert:       "warning|Uses a reverse-engineered Google Photos API. May stop working without notice. The MP4 disguise feature ships arbitrary bytes as fake video; do not rely on it for critical data.",
}

func init() {
	op.RegisterDriver(func() driver.Driver {
		return &GooglePhotoNative{}
	})
}
