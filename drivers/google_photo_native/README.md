# google_photo_native

OpenList storage driver for Google Photos via the private Android
`photos.native` API. The selling point is **zero-quota storage of
arbitrary files** by wrapping non-media payloads in a minimal MP4
container — the same trick used by `xob0t/gp_disguise` and `gotohp`.

See [DESIGN.md](./DESIGN.md) for the full architecture, review history,
and risk log.

## When to use this driver

- You want to use a Google Photos account as an unbounded object store.
- You accept that this driver depends on reverse-engineered endpoints
  that Google can change without notice.
- You already have access to a Google account you can afford to lose
  if anti-abuse trips.

For a stable, sanctioned Google Photos integration (list + download
only, no upload, no disguise), use `drivers/google_photo`.

## Setup

1. Create a new storage of type `GooglePhotoNative`.
2. In a private browser window, open
   `https://accounts.google.com/EmbeddedSetup`. Sign in normally
   (2FA and passkeys work).
3. Open DevTools → Application → Cookies →
   `https://accounts.google.com`. Copy the value of the cookie named
   **`oauth_token`**.
4. Paste it into the driver's `OAuthToken` field and save.
   The first `Init` exchanges this cookie for a long-lived master
   token, fills in `MasterToken`, `Email`, `AndroidID`, and clears
   `OAuthToken`. Do not touch those three fields afterwards.

### Re-authentication

If the master token is revoked, Init returns an error and clears
`MasterToken`. Edit the storage, paste a fresh `oauth_token`, save,
and Init runs the exchange again.

## What works

| Feature | Status | Notes |
|---|---|---|
| List media | ✓ | Under `all/`. Capped at `MaxListItems` (default 5000). |
| Download native media | ✓ | 302 to Google's signed URL. |
| Download disguised file | ✓ | Streamed through a range-shifting reader. See §7 of DESIGN.md. |
| Upload native media | ✓ | With SHA-1 dedup (same-account only — cross-account safety not fully verified). |
| Upload arbitrary file (disguise) | ✓ | Enabled by default (`DisguiseNonMedia`). File appears as `<origName>.mp4`. |
| Delete | ✓ | Move to trash by default; `PermanentDelete` forces hard delete. |
| Create album | ✓ | `MakeDir` under `albums/`. |
| Add media to album | ✓ | `Move` from `all/` into `albums/<name>`. Google's model is many-to-many; the source is not removed. |
| List an album's contents | ✗ | Private API has no filtered endpoint; would require full-library scan. Deferred. |
| Thumbnail preview | ✗ | Requires bearer-authenticated URLs; not proxy-able to the browser without extra plumbing. |
| Rename | ✗ | Photos has no rename. |

## Fields

- `oauth_token` — one-time cookie, consumed on save.
- `master_token`, `email`, `android_id` — auto-populated. Never edit.
- `language` — passed to `/auth`. Default `en`.
- `upstream_proxy` — optional HTTP(S) proxy for Google API calls.
- `use_quota` — force uploads to count against Photos quota (debug).
- `saver` — use "Storage saver" quality.
- `force_upload` — bypass SHA-1 dedup lookup.
- `request_trash_items` — expose trashed items under `trash/`.
- `show_albums` — expose the `albums/` virtual dir (default on).
- `disguise_non_media` — wrap non-media files as MP4 (default on).
- `permanent_delete` — Remove sends to permanent delete, not trash.
- `max_list_items` — cap on List page count. Larger is slower on first
  request; caching amortizes.

## Known limitations

- **Google's signed download URLs ignore Range headers.** Our
  disguise-stripping reader compensates by fetching the full wire body
  and shifting it locally. This is fine for kilobyte-scale disguised
  files, painful for gigabyte-scale ones. See `shiftedReader` in
  `disguise_range.go`.
- Disguised uploads do not participate in SHA-1 dedup (hash is computed
  over the MP4 wrapper, which varies with the embedded filename).
- Cross-account dedup safety is unproven. Same-account dedup is
  verified. Set `force_upload=true` if you want zero dedup risk.
- First `List` on a large library is slow (one round-trip per 100
  items until `MaxListItems`). Cached for the next 5 minutes by OpenList.

## Tests

- `go test ./drivers/google_photo_native/` — unit test for the disguise
  round-trip parser.
- `GPN_SMOKE_CRED_A='<authData>' go test -run TestDisguiseE2E -v
  -timeout 5m ./drivers/google_photo_native/` — full end-to-end round
  trip: upload a random 1 MB disguised blob, verify byte-for-byte
  read-back at both full range and a mid-file range, trash on cleanup.
- `GPN_SMOKE_CRED_A=... GPN_SMOKE_CRED_B=... go test
  -run TestCrossAccountDedup -v ...` — verify per-account dedup with
  two Google accounts. Fresh master tokens required.
