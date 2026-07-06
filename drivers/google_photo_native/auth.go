package google_photo_native

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// gpsoauth flow — one-shot browser oauth_token → long-lived master token.
// Copied and trimmed from gotohp_rev/backend/gpsoauth.go, with the
// token-binding branch removed (rooted-device use case, needs tink-crypto).

const (
	gpsoauthURL           = "https://android.clients.google.com/auth"
	googlePhotosClientSig = "24bb24c05e47e0aefa68a58a766179d9b613a600"
	gpsoauthTimeout       = 30 * time.Second
	gpsoauthMaxBody       = 1 << 20 // 1 MiB
)

// RandomAndroidID returns a 16-hex-char device identifier. Bound to a
// storage's MasterToken for its lifetime; do not regenerate.
func RandomAndroidID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func gpsoauthPost(form url.Values) (map[string]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gpsoauthTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", gpsoauthURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "GoogleAuth/1.4")

	client, err := newHTTPClientWithProxy("")
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, gpsoauthMaxBody))
	if err != nil {
		return nil, err
	}

	out := make(map[string]string, 16)
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			continue
		}
		out[strings.TrimSpace(line[:eq])] = strings.TrimSpace(line[eq+1:])
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		detail := out["Error"]
		if detail == "" {
			detail = out["ErrorDetail"]
		}
		return nil, fmt.Errorf("auth endpoint returned %d: %s", resp.StatusCode, detail)
	}
	return out, nil
}

// ExchangeOAuthToken converts a one-time oauth_token cookie value (captured
// from an EmbeddedSetup browser login) into a long-lived aas_et master
// token. The returned email is authoritative — it is what Google resolves
// the cookie to, not what the user typed.
func ExchangeOAuthToken(oauthToken, androidID string) (email string, masterToken string, err error) {
	form := url.Values{}
	form.Set("accountType", "HOSTED_OR_GOOGLE")
	form.Set("Email", "")
	form.Set("has_permission", "1")
	form.Set("add_account", "1")
	form.Set("ACCESS_TOKEN", "1")
	form.Set("Token", oauthToken)
	form.Set("service", "ac2dm")
	form.Set("source", "android")
	form.Set("androidId", androidID)
	form.Set("device_country", "us")
	form.Set("operatorCountry", "us")
	form.Set("lang", "en")
	form.Set("sdk_version", "29")
	form.Set("google_play_services_version", "240913000")
	form.Set("client_sig", googlePhotosClientSig)
	form.Set("callerSig", googlePhotosClientSig)
	form.Set("droidguard_results", "dummy123")

	r, err := gpsoauthPost(form)
	if err != nil {
		return "", "", err
	}
	if r["Token"] == "" {
		return "", "", fmt.Errorf("exchange returned no master Token (got fields: %v)", sortedKeys(r))
	}
	respEmail := r["Email"]
	if respEmail == "" {
		return "", "", fmt.Errorf("exchange did not return an Email field")
	}
	return respEmail, r["Token"], nil
}

// BuildAuthString formats master credentials into the query-string blob the
// bearer-mint call expects. Same shape gotohp stores; kept internal because
// nothing outside the driver reads it.
func buildAuthString(email, masterToken, androidID, language string) string {
	values := url.Values{}
	values.Set("Email", email)
	values.Set("Token", masterToken)
	values.Set("androidId", androidID)
	values.Set("service", "oauth2:openid https://www.googleapis.com/auth/mobileapps.native https://www.googleapis.com/auth/photos.native")
	values.Set("source", "android")
	values.Set("app", "com.google.android.apps.photos")
	values.Set("callerPkg", "com.google.android.apps.photos")
	values.Set("client_sig", googlePhotosClientSig)
	values.Set("callerSig", googlePhotosClientSig)
	values.Set("google_play_services_version", "240913000")
	values.Set("has_permission", "1")
	values.Set("sdk_version", "29")
	if language != "" {
		values.Set("lang", language)
	}
	return values.Encode()
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
