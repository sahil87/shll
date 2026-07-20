// Package changelog is shll's isolated GitHub-releases fetch layer. It is the
// ONLY package in the repo that imports net/http (Constitution I spirit —
// mirroring internal/proc's isolation of os/exec): command code in
// src/cmd/shll never talks to net/http directly, it calls this package's
// exported API.
//
// The package fetches a repo's GitHub Releases (unauthenticated, stdlib
// net/http only — no new module dependency), filters them to a requested
// version range, and degrades any failure to a typed "unavailable" Result so
// callers render a compare-URL fallback and keep going (Constitution V —
// Graceful Degradation). It holds no state (Constitution II): every call
// re-fetches.
package changelog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// apiBaseDefault is the production GitHub REST API base. It is assigned to the
// package-level baseURL seam below, which tests override to point at a local
// httptest.Server. Named constant per code-quality.md (no magic strings).
const apiBaseDefault = "https://api.github.com"

// owner is the single source of truth for the sahil87 GitHub owner segment
// inside this package. Every repo path — the API path (repos/{owner}/{repo})
// and the browser-facing compare/releases URLs (github.com/{owner}/{repo}) —
// derives its owner from this one constant so the owner is never re-encoded
// (code-quality.md — no magic strings, no drift). It deliberately mirrors the
// value of cmd/shll's githubOrgBase rather than importing across the package
// boundary (this package must not depend on cmd/shll).
const owner = "sahil87"

// githubOrgBase is the GitHub org base URL for the shll toolkit, used to
// build the human-facing compare / releases-page URLs (NOT the API host —
// that is baseURL). Derived from owner so the org prefix has one source.
const githubOrgBase = "https://github.com/" + owner + "/"

// orgRepoPrefix is the owner segment of every sahil87 API repo path
// (repos/{owner}/{repo}). Derived from owner so the API path and the browser
// URLs share one source of truth for the owner.
const orgRepoPrefix = owner + "/"

// requestTimeout bounds each releases fetch. 10s is generous for a single
// unauthenticated GitHub API call while keeping a stalled request from hanging
// a `shll update` / `shll changelog` run. Named constant per code-quality.md.
const requestTimeout = 10 * time.Second

// perPage requests the maximum page size in one shot — the toolkit's repos have
// far fewer than 100 releases, so a single page always covers any real range and
// no pagination is needed in v1.
const perPage = 100

// baseURL is the package-level API-host seam (mirrors proc.Runner's
// package-level-swappable injection). Production uses apiBaseDefault; tests swap
// it for an httptest.Server URL so the real net/http code paths (status codes,
// JSON decode, timeout) are exercised without network access.
var baseURL = apiBaseDefault

// httpClient is the package-level client seam, swappable in tests. Requests
// carry their own context timeout (requestTimeout), so the client itself needs
// no Timeout field.
var httpClient = &http.Client{}

// SetTransportForTest points the package's API-host + client seams at a test
// server (typically an httptest.Server's URL and Client) and returns a restore
// func that reverts them. It exists so cross-package tests in cmd/shll can drive
// the changelog surface against a local server without network — the same
// package-level-swap seam this package's own tests use, exported for the one
// cross-package consumer. Not for production use.
func SetTransportForTest(base string, client *http.Client) (restore func()) {
	prevBase, prevClient := baseURL, httpClient
	baseURL = base
	if client != nil {
		httpClient = client
	}
	return func() {
		baseURL = prevBase
		httpClient = prevClient
	}
}

// ErrUnavailable is the sentinel wrapped by every fetch failure — network error,
// non-200 status (incl. 403 rate-limit), timeout, or JSON decode failure. It is
// the "degrade to a compare URL and continue" signal (Constitution V). Callers
// match it via errors.Is; FetchRange folds it into Result.Unavailable so call
// sites can branch on a bool without importing the error.
var ErrUnavailable = errors.New("changelog: release data unavailable")

// Release is one GitHub release, decoded from the Releases API JSON. Only the
// fields shll renders are decoded; the rest of the (large) payload is ignored.
type Release struct {
	// Tag is the git tag the release points at (JSON `tag_name`), e.g. `v0.6.4`.
	Tag string `json:"tag_name"`
	// Title is the release title (JSON `name`), e.g. `fix: opencode session parsing`.
	Title string `json:"name"`
	// Body is the auto-generated "What's Changed" markdown (JSON `body`),
	// printed as-is by `shll changelog`.
	Body string `json:"body"`
}

// RangeReq names one tool's fetch: the roster tool name (for result labelling),
// its GitHub repo slug (which is NOT always the name — rk's repo is run-kit),
// and the (old, new] version range to filter to.
type RangeReq struct {
	Tool string
	Repo string
	Old  string
	New  string
}

// Result is the outcome of fetching+filtering one tool's releases. Unavailable
// distinguishes "fetch failed → render a compare-URL fallback" from a successful
// fetch that simply had zero releases in range (Releases empty, Unavailable
// false). Err carries the failure detail when Unavailable is true.
type Result struct {
	Tool        string
	Repo        string
	Old         string
	New         string
	Releases    []Release
	Unavailable bool
	Err         error
}

// FetchRange fetches repo's releases and filters them to (old, new], newest
// first. Any fetch failure is folded into a Result with Unavailable=true and Err
// set (never a returned error) so callers render the compare-URL fallback and
// continue. A successful fetch with no releases in range returns Unavailable
// false and an empty Releases slice — the "no releases in range" case, distinct
// from unavailable.
func FetchRange(ctx context.Context, req RangeReq) Result {
	res := Result{Tool: req.Tool, Repo: req.Repo, Old: req.Old, New: req.New}
	rels, err := fetchReleases(ctx, req.Repo)
	if err != nil {
		res.Unavailable = true
		res.Err = err
		return res
	}
	res.Releases = releasesInRange(rels, req.Old, req.New)
	return res
}

// FetchAll fetches every request concurrently (read-only HTTP, mirroring
// update.go's probeRoster carve-out) and returns the Results in the SAME order
// as reqs — the caller assembles reqs in roster order, so output stays
// roster-ordered regardless of which fetch completes first. Indexing by position
// (not appending on completion) is what preserves the order.
func FetchAll(ctx context.Context, reqs []RangeReq) []Result {
	results := make([]Result, len(reqs))
	var wg sync.WaitGroup
	for i, req := range reqs {
		wg.Add(1)
		go func(i int, req RangeReq) {
			defer wg.Done()
			results[i] = FetchRange(ctx, req)
		}(i, req)
	}
	wg.Wait()
	return results
}

// LatestTag returns the newest release tag for a repo (by normalized-version
// order) and any releases fetched, degrading to ErrUnavailable on failure. It is
// used by the no-range forms of `shll changelog` (installed → latest) so the
// caller does not re-fetch: it returns the release list too, which the caller
// then range-filters. An empty release set returns "" with a nil error.
func LatestTag(ctx context.Context, repo string) (string, []Release, error) {
	rels, err := fetchReleases(ctx, repo)
	if err != nil {
		return "", nil, err
	}
	latest := ""
	for _, r := range rels {
		if latest == "" || compareVer(r.Tag, latest) > 0 {
			latest = r.Tag
		}
	}
	return latest, rels, nil
}

// CompareVer compares two versions by numeric dot-components (after stripping a
// `v` prefix and a brew `_N` suffix), returning -1, 0, or +1. Exported so cmd
// code can decide "installed is already at/after latest" for the up-to-date
// notice without re-implementing the toolkit's version-compare rules.
func CompareVer(a, b string) int { return compareVer(a, b) }

// NormalizeVer strips a leading `v` prefix and a brew `_N` revision suffix so a
// tag (`v0.6.4`) and a brew version (`0.6.4_1`) share one display form. Exported
// so cmd code renders ONE normalized version form on both sides of a transition
// (never mixing brew-form `old` with tag-form `new`, nor echoing a user's raw
// `v`-prefixed spec).
func NormalizeVer(s string) string { return normalizeVer(s) }

// FirstDiffComponent returns the index of the first dot-separated component at
// which a and b differ numerically (0 = major, 1 = minor, 2+ = patch), after
// NormalizeVer, or -1 when the versions compare equal. Exported so the notify-
// threshold policy in internal/versions can classify a pending bump (major vs
// minor vs patch) without re-implementing this package's version-component
// parsing rules (missing trailing components are 0; only a component's leading
// integer run is read).
func FirstDiffComponent(a, b string) int {
	ap := strings.Split(normalizeVer(a), ".")
	bp := strings.Split(normalizeVer(b), ".")
	n := len(ap)
	if len(bp) > n {
		n = len(bp)
	}
	for i := 0; i < n; i++ {
		if verComponent(ap, i) != verComponent(bp, i) {
			return i
		}
	}
	return -1
}

// ReleasesInRange filters an already-fetched release list to (old, new], newest
// first — the same filter FetchRange applies internally, exported so a caller
// that fetched once (via LatestTag) can range-filter the returned list WITHOUT a
// second network round-trip (the no-range `shll changelog` path: one GET per
// repo, not two).
func ReleasesInRange(rels []Release, old, new string) []Release {
	return releasesInRange(rels, old, new)
}

// CompareURL is the browser-facing "Full Changelog" compare link between two
// versions, e.g. https://github.com/sahil87/tu/compare/v0.6.2...v0.6.4. Tags are
// v-prefixed for the URL (GitHub compare refs use the tag form). Single source of
// truth so neither the command nor the update digest open-codes it.
func CompareURL(repo, old, new string) string {
	return fmt.Sprintf("%s%s/compare/%s...%s", githubOrgBase, repo, vTag(old), vTag(new))
}

// ReleasesURL is the browser-facing releases page for a repo, printed in the
// "up to date" notice and as a fallback anchor.
func ReleasesURL(repo string) string {
	return githubOrgBase + repo + "/releases"
}

// fetchReleases GETs repo's releases (unauthenticated, per_page=100) with a
// bounded context timeout. It returns ErrUnavailable (wrapped with detail) on a
// transport error, timeout, non-200 status, or JSON decode failure — the single
// degradation point (Constitution V). No retries in v1.
func fetchReleases(ctx context.Context, repo string) ([]Release, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	reqCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	url := fmt.Sprintf("%s/repos/%s%s/releases?per_page=%d", baseURL, orgRepoPrefix, repo, perPage)
	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: build request: %v", ErrUnavailable, err)
	}
	// Unauthenticated: no Authorization header. Ask for the v3 JSON media type.
	httpReq.Header.Set("Accept", "application/vnd.github+json")

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: request: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Treat ANY non-200 as unavailable (403 rate-limit, 404, 5xx alike).
		// The body is not read here — the status code alone is the degradation
		// signal — and the deferred Close above still releases the connection.
		return nil, fmt.Errorf("%w: status %d", ErrUnavailable, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: read body: %v", ErrUnavailable, err)
	}
	var rels []Release
	if err := json.Unmarshal(body, &rels); err != nil {
		return nil, fmt.Errorf("%w: decode: %v", ErrUnavailable, err)
	}
	return rels, nil
}

// releasesInRange selects the releases whose tag is in (old, new] and returns
// them newest-first (descending normalized version). A release equal to old is
// excluded; one equal to new is included. When old == new the range is empty.
func releasesInRange(rels []Release, old, new string) []Release {
	inRange := make([]Release, 0, len(rels))
	for _, r := range rels {
		if compareVer(r.Tag, old) > 0 && compareVer(r.Tag, new) <= 0 {
			inRange = append(inRange, r)
		}
	}
	sort.SliceStable(inRange, func(i, j int) bool {
		return compareVer(inRange[i].Tag, inRange[j].Tag) > 0
	})
	return inRange
}

// normalizeVer strips a leading `v` prefix and a brew revision suffix (`_N`)
// from a version string so tags (`v0.6.4`) and brew versions (`0.6.4_1`) compare
// on the same footing. It leaves any pre-release/build suffix beyond the numeric
// core untouched (compareVer only reads the numeric dot-components).
func normalizeVer(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "v")
	if i := strings.IndexByte(s, '_'); i >= 0 {
		s = s[:i]
	}
	return s
}

// compareVer compares two versions by their numeric dot-separated components,
// after normalizeVer. Missing trailing components are treated as 0 (so `0.6`
// == `0.6.0`). A non-numeric component compares as 0 (best-effort — the toolkit
// tags are plain numeric SemVer). Returns -1, 0, or +1.
func compareVer(a, b string) int {
	ap := strings.Split(normalizeVer(a), ".")
	bp := strings.Split(normalizeVer(b), ".")
	n := len(ap)
	if len(bp) > n {
		n = len(bp)
	}
	for i := 0; i < n; i++ {
		av := verComponent(ap, i)
		bv := verComponent(bp, i)
		if av != bv {
			if av < bv {
				return -1
			}
			return 1
		}
	}
	return 0
}

// verComponent returns the numeric value of the i-th dot-component of parts, or
// 0 when the component is absent or non-numeric. A component may carry a
// pre-release/build suffix (e.g. `4-rc1`); only its leading integer run is read.
func verComponent(parts []string, i int) int {
	if i >= len(parts) {
		return 0
	}
	c := parts[i]
	// Read the leading integer run (handles `4`, `4-rc1`, `4+build`).
	end := 0
	for end < len(c) && c[end] >= '0' && c[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0
	}
	n, err := strconv.Atoi(c[:end])
	if err != nil {
		return 0
	}
	return n
}

// vTag returns v as a v-prefixed tag for compare URLs (adds `v` if absent).
func vTag(v string) string {
	if strings.HasPrefix(v, "v") {
		return v
	}
	return "v" + v
}
