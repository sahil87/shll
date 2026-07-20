// Package versions is shll's "latest version per tool" resolver seam — the
// single surface behind `shll check-updates` (both backends) and the GitHub
// anchor of `shll changelog`'s no-range resolution.
//
// It owns the shll.ai versions-manifest fetch (schema decode + notify policy)
// and the notify-threshold ("notable") computation, and delegates the GitHub
// backend to internal/changelog's existing fetch — no duplicated GitHub code.
// Together with internal/changelog it keeps net/http isolated in internal
// packages (Constitution I spirit): command code in src/cmd/shll never talks
// to net/http directly. It holds no state (Constitution II): every call
// re-fetches. Future versions.json schema evolution is absorbed HERE, so
// consumers (run-kit exec'ing `shll check-updates --json`, `shll changelog`)
// never compile in manifest or version-comparison policy.
package versions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/sahil87/shll/internal/changelog"
)

// manifestURLDefault is the production shll.ai versions manifest — the roster +
// policy authority for the `--released` backend. Named constant per
// code-quality.md (no magic strings); assigned to the manifestURL seam below.
const manifestURLDefault = "https://shll.ai/versions.json"

// manifestSchema is the versions.json schema this binary understands. A
// manifest reporting any other schema is treated as unavailable — shll is the
// single surface that absorbs schema evolution, so failing loudly on an
// unknown schema beats silently misreading policy (a schema bump ships with a
// shll release that raises this constant).
const manifestSchema = 1

// requestTimeout bounds the manifest fetch, mirroring internal/changelog's
// per-request timeout: generous for one HTTPS GET while keeping a stall from
// hanging a `shll check-updates` run. Named constant per code-quality.md.
const requestTimeout = 10 * time.Second

// Notify policy values as published in the manifest's per-tool `notify` field.
// An unknown/future value is treated as NotifyMinor by Notable (forward-compat
// conservatism: mild over-notification beats silent suppression on a notify
// surface). Named constants per code-quality.md.
const (
	NotifyNever = "never"
	NotifyPatch = "patch"
	NotifyMinor = "minor"
)

// manifestURL is the package-level URL seam (mirrors internal/changelog's
// baseURL and proc.Runner's package-level-swappable injection). Production
// uses manifestURLDefault; tests swap it for an httptest.Server URL so the
// real net/http code paths (status codes, JSON decode, timeout) are exercised
// without network access.
var manifestURL = manifestURLDefault

// httpClient is the package-level client seam, swappable in tests. Requests
// carry their own context timeout (requestTimeout), so the client itself
// needs no Timeout field.
var httpClient = &http.Client{}

// SetTransportForTest points the package's manifest-URL + client seams at a
// test server and returns a restore func that reverts them — the same
// package-level-swap seam internal/changelog exports, for the one
// cross-package consumer (cmd/shll's check_updates_test.go). Not for
// production use.
func SetTransportForTest(url string, client *http.Client) (restore func()) {
	prevURL, prevClient := manifestURL, httpClient
	manifestURL = url
	if client != nil {
		httpClient = client
	}
	return func() {
		manifestURL = prevURL
		httpClient = prevClient
	}
}

// ErrUnavailable is the sentinel wrapped by every manifest-fetch failure —
// network error, timeout, non-200 status, JSON decode failure, or an
// unsupported schema. For the `--released` backend there is exactly one fetch,
// so unavailability fails the whole check (unlike the per-tool GitHub
// degradation) — the caller writes a diagnostic and exits 1.
var ErrUnavailable = errors.New("versions: manifest unavailable")

// ManifestTool is one tool's entry in the versions manifest: its latest
// released version, its notify policy (see the Notify* constants), and its
// tap-relative formula leaf. Only the fields shll consumes are decoded.
type ManifestTool struct {
	Latest  string `json:"latest"`
	Notify  string `json:"notify"`
	Formula string `json:"formula"`
}

// Manifest is the decoded shll.ai versions.json (schema 1): the schema tag,
// the generation timestamp, and the per-tool map keyed by tool NAME (the
// manifest carries shll itself plus every roster tool).
type Manifest struct {
	Schema      int                     `json:"schema"`
	GeneratedAt string                  `json:"generated_at"`
	Tools       map[string]ManifestTool `json:"tools"`
}

// FetchManifest GETs the shll.ai versions manifest with a bounded context
// timeout and decodes it. It returns an error wrapping ErrUnavailable on a
// transport error, timeout, non-200 status, decode failure, or an unsupported
// schema value — the single degradation point for the `--released` backend.
// No retries, no caching (Constitution II): every call re-fetches.
func FetchManifest(ctx context.Context) (Manifest, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	reqCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodGet, manifestURL, nil)
	if err != nil {
		return Manifest{}, fmt.Errorf("%w: build request: %v", ErrUnavailable, err)
	}

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return Manifest{}, fmt.Errorf("%w: request: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Any non-200 is unavailable; the status code alone is the signal (the
		// body is not read — the deferred Close still releases the connection).
		return Manifest{}, fmt.Errorf("%w: status %d", ErrUnavailable, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Manifest{}, fmt.Errorf("%w: read body: %v", ErrUnavailable, err)
	}
	var m Manifest
	if err := json.Unmarshal(body, &m); err != nil {
		return Manifest{}, fmt.Errorf("%w: decode: %v", ErrUnavailable, err)
	}
	if m.Schema != manifestSchema {
		return Manifest{}, fmt.Errorf("%w: unsupported schema %d (want %d)", ErrUnavailable, m.Schema, manifestSchema)
	}
	return m, nil
}

// Notable reports whether the pending installed→latest bump crosses the
// tool's notify threshold:
//
//   - NotifyNever → never notable.
//   - NotifyPatch → any pending bump is notable.
//   - NotifyMinor → notable iff a minor-or-higher component increases (a
//     patch-only bump is not notable); a major bump crosses every non-never
//     threshold.
//   - any other value (unknown/future policy, empty) → treated as NotifyMinor:
//     forward-compat conservatism — mild over-notification beats silently
//     suppressing notifications a future policy meant to allow.
//
// When no update is pending (installed >= latest by changelog.CompareVer, or
// either side empty/unresolved) it returns false regardless of policy. Version
// parsing delegates to internal/changelog (CompareVer + FirstDiffComponent) so
// the toolkit's version-comparison rules live in exactly one place.
func Notable(notify, installed, latest string) bool {
	if installed == "" || latest == "" || changelog.CompareVer(latest, installed) <= 0 {
		return false
	}
	switch notify {
	case NotifyNever:
		return false
	case NotifyPatch:
		return true
	default: // NotifyMinor and unknown/future values
		idx := changelog.FirstDiffComponent(installed, latest)
		return idx >= 0 && idx <= 1
	}
}

// LatestGitHub resolves a repo's latest release tag via the GitHub backend —
// a thin delegation to internal/changelog's LatestTag (no duplicated GitHub
// fetch code). It preserves LatestTag's single-fetch contract by returning the
// fetched release list too, so a caller that also needs the releases (the
// no-range `shll changelog` anchor) never fetches twice. Failures degrade to
// changelog.ErrUnavailable per that package's contract; the check-updates
// `--github` backend degrades them per-tool (row omitted), never failing the
// run (Constitution V).
func LatestGitHub(ctx context.Context, repo string) (latest string, rels []changelog.Release, err error) {
	return changelog.LatestTag(ctx, repo)
}
