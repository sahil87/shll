package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/sahil87/shll/internal/changelog"
	"github.com/sahil87/shll/internal/proc"
)

// changelogServer starts an httptest.Server serving the given per-repo release
// JSON and points the internal/changelog seams at it for the test. The handler
// keys on the repo slug from the request path (/repos/sahil87/<repo>/releases).
// jsonByRepo maps a repo slug to its releases-array JSON; a repo absent from the
// map is served 404 (→ unavailable).
func changelogServer(t *testing.T, jsonByRepo map[string]string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Path is /repos/sahil87/<repo>/releases.
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) < 4 {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		repo := parts[2]
		body, ok := jsonByRepo[repo]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		fmt.Fprint(w, body)
	}))
	restore := changelog.SetTransportForTest(srv.URL, srv.Client())
	t.Cleanup(func() {
		srv.Close()
		restore()
	})
}

// relJSON builds a releases-array JSON body from (tag,title,body) triples.
func relJSON(triples ...[3]string) string {
	out := "["
	for i, tr := range triples {
		if i > 0 {
			out += ","
		}
		out += fmt.Sprintf(`{"tag_name":%q,"name":%q,"body":%q}`, tr[0], tr[1], tr[2])
	}
	return out + "]"
}

func TestChangelog_ExplicitRangeHappyPath(t *testing.T) {
	changelogServer(t, map[string]string{
		"hop": relJSON(
			[3]string{"v0.1.18", "feat: agent support", "## What's Changed\n- agent"},
			[3]string{"v0.1.17", "fix: shim hardening", "## What's Changed\n- shim"},
			[3]string{"v0.1.16", "old", "old body"},
		),
	})
	// No proc fake needed — explicit ranges never consult brew. But installFakeRunner
	// guards against an accidental real brew spawn.
	installFakeRunner(t, &fakeRunner{})

	var stdout, stderr bytes.Buffer
	if err := runChangelog(context.Background(), &stdout, &stderr, []string{"hop@0.1.16..0.1.18"}); err != nil {
		t.Fatalf("runChangelog err = %v, want nil", err)
	}
	out := stdout.String()
	// Header with the transition + release count (2 in (0.1.16, 0.1.18]). Non-TTY
	// buffer → ASCII-degraded arrow (`->`).
	if !strings.Contains(out, "0.1.16 -> 0.1.18 (2 releases)") {
		t.Fatalf("out missing transition header:\n%s", out)
	}
	// Full bodies present, newest first.
	if !strings.Contains(out, "v0.1.18  feat: agent support") || !strings.Contains(out, "- agent") {
		t.Fatalf("out missing newest release + body:\n%s", out)
	}
	if !strings.Contains(out, "v0.1.17  fix: shim hardening") {
		t.Fatalf("out missing second release:\n%s", out)
	}
	// v0.1.16 == old is excluded.
	if strings.Contains(out, "old body") {
		t.Fatalf("out should exclude the release equal to old bound:\n%s", out)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRenderChangelogResult_ColorFormBoldAnchors(t *testing.T) {
	// Change 13k3: the changelog-surface navigational anchors — the transition line
	// and each release's tag/title line — are PLAIN BOLD when color is enabled
	// (bold-cyan is reserved for the per-tool header). The release body is NOT an
	// anchor and carries no styling. renderChangelogResult is driven directly with
	// color=true (bytes.Buffer runs are always non-TTY, so this is the only path
	// that exercises the color branch). renderReleases is exercised transitively.
	res := changelog.Result{
		Tool: "hop", Repo: "hop", Old: "0.1.16", New: "0.1.18",
		Releases: []changelog.Release{
			{Tag: "v0.1.18", Title: "feat: agent support", Body: "## What's Changed\n* agent"},
		},
	}
	var buf bytes.Buffer
	renderChangelogResult(&buf, res, true)
	got := buf.String()

	// Transition line bold-anchored (R3.3): the `{old} → {new} (N release[s])` run
	// is wrapped in ansiBold … ansiReset, with the Unicode arrow (color branch).
	if !strings.Contains(got, ansiBold+"0.1.16 → 0.1.18 (1 release)"+ansiReset) {
		t.Fatalf("transition line must be a bold anchor with the Unicode arrow:\n%q", got)
	}
	// Release tag/title line bold-anchored (R3.2).
	if !strings.Contains(got, ansiBold+"v0.1.18  feat: agent support"+ansiReset) {
		t.Fatalf("release tag/title line must be a bold anchor:\n%q", got)
	}
	// The anchors are plain bold, never bold-cyan (reserved for the header, R3.4).
	if strings.Contains(got, ansiBoldCyan) {
		t.Fatalf("changelog anchors must be plain bold, never bold-cyan:\n%q", got)
	}
	// The body is not an anchor — the body text itself carries no ANSI wrap.
	if !strings.Contains(got, "\n## What's Changed\n* agent\n") {
		t.Fatalf("release body must render unstyled inline:\n%q", got)
	}
}

func TestChangelog_UnknownTargetErrors(t *testing.T) {
	installFakeRunner(t, &fakeRunner{})
	var stdout, stderr bytes.Buffer
	err := runChangelog(context.Background(), &stdout, &stderr, []string{"nope@1..2"})
	if !errors.Is(err, errSilent) {
		t.Fatalf("err = %v, want errSilent", err)
	}
	if !strings.Contains(stderr.String(), `unknown target "nope"`) || !strings.Contains(stderr.String(), "valid targets:") {
		t.Fatalf("stderr = %q, want unknown-target diagnostic", stderr.String())
	}
}

func TestChangelog_InvalidRangeErrors(t *testing.T) {
	installFakeRunner(t, &fakeRunner{})
	var stdout, stderr bytes.Buffer
	err := runChangelog(context.Background(), &stdout, &stderr, []string{"hop@0.1.16"}) // no ..new
	if !errors.Is(err, errSilent) {
		t.Fatalf("err = %v, want errSilent", err)
	}
	if !strings.Contains(stderr.String(), "invalid range") {
		t.Fatalf("stderr = %q, want invalid-range diagnostic", stderr.String())
	}
}

func TestChangelog_RosterOrderRegardlessOfArgOrder(t *testing.T) {
	changelogServer(t, map[string]string{
		"tu":  relJSON([3]string{"v0.6.4", "tu4", "b"}),
		"hop": relJSON([3]string{"v0.1.18", "hop18", "b"}),
	})
	installFakeRunner(t, &fakeRunner{})

	var stdout, stderr bytes.Buffer
	// Args given hop before tu; roster order is tu before hop.
	if err := runChangelog(context.Background(), &stdout, &stderr, []string{"hop@0.1.17..0.1.18", "tu@0.6.3..0.6.4"}); err != nil {
		t.Fatalf("runChangelog err = %v", err)
	}
	out := stdout.String()
	tuIdx := strings.Index(out, "tu4")
	hopIdx := strings.Index(out, "hop18")
	if tuIdx < 0 || hopIdx < 0 || tuIdx > hopIdx {
		t.Fatalf("expected tu before hop (roster order), out:\n%s", out)
	}
}

func TestChangelog_NoRangeInstalledToLatest(t *testing.T) {
	changelogServer(t, map[string]string{
		"tu": relJSON(
			[3]string{"v0.6.4", "tu4", "body4"},
			[3]string{"v0.6.3", "tu3", "body3"},
		),
	})
	// tu installed at 0.6.2 (brew list returns `<formula> 0.6.2`); latest is 0.6.4.
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		if req.Name == brewBinary && len(req.Args) >= 4 && req.Args[0] == "list" && req.Args[3] == formulaPrefix+"tu" {
			return proc.Result{Stdout: []byte("tu 0.6.2\n")}
		}
		if req.Name == brewBinary && len(req.Args) >= 4 && req.Args[0] == "list" {
			return proc.Result{Err: errors.New("not installed")}
		}
		return proc.Result{}
	}}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runChangelog(context.Background(), &stdout, &stderr, []string{"tu"}); err != nil {
		t.Fatalf("runChangelog err = %v", err)
	}
	// Both sides of the transition are the normalized (v-stripped) form — no mixed
	// `0.6.2 → v0.6.4` — and the arrow ASCII-degrades in the non-TTY buffer.
	if !strings.Contains(stdout.String(), "0.6.2 -> 0.6.4 (2 releases)") {
		t.Fatalf("out missing installed→latest header (normalized both sides):\n%s", stdout.String())
	}
}

func TestChangelog_NoRangeSingleFetchPerRepo(t *testing.T) {
	// The no-range installed→latest path must fetch each repo's releases EXACTLY
	// ONCE (LatestTag returns the list; the caller filters locally) — not twice
	// (LatestTag then a discarded FetchAll re-fetch). Count GETs at the server.
	var gets int
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gets++
		mu.Unlock()
		fmt.Fprint(w, relJSON(
			[3]string{"v0.6.4", "tu4", "body4"},
			[3]string{"v0.6.3", "tu3", "body3"},
		))
	}))
	restore := changelog.SetTransportForTest(srv.URL, srv.Client())
	t.Cleanup(func() { srv.Close(); restore() })

	// tu installed at 0.6.2; latest 0.6.4.
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		if req.Name == brewBinary && len(req.Args) >= 4 && req.Args[0] == "list" && req.Args[3] == formulaPrefix+"tu" {
			return proc.Result{Stdout: []byte("tu 0.6.2\n")}
		}
		return proc.Result{Err: errors.New("not installed")}
	}}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runChangelog(context.Background(), &stdout, &stderr, []string{"tu"}); err != nil {
		t.Fatalf("runChangelog err = %v", err)
	}
	mu.Lock()
	got := gets
	mu.Unlock()
	if got != 1 {
		t.Fatalf("GET count = %d, want exactly 1 (no-range must fetch once per repo, not re-fetch)", got)
	}
}

func TestChangelog_VPrefixedSpecNormalizes(t *testing.T) {
	// R7: a `tu@v0.6.2..v0.6.4` spec (with v prefixes) parses and renders
	// identically to the unprefixed form — the raw `v`-prefixed bounds are NOT
	// echoed into the header; the displayed transition is the normalized form.
	changelogServer(t, map[string]string{
		"tu": relJSON(
			[3]string{"v0.6.4", "tu4", "body4"},
			[3]string{"v0.6.3", "tu3", "body3"},
			[3]string{"v0.6.2", "old", "old body"},
		),
	})
	installFakeRunner(t, &fakeRunner{})

	var stdout, stderr bytes.Buffer
	if err := runChangelog(context.Background(), &stdout, &stderr, []string{"tu@v0.6.2..v0.6.4"}); err != nil {
		t.Fatalf("runChangelog err = %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "0.6.2 -> 0.6.4 (2 releases)") {
		t.Fatalf("v-prefixed spec must render normalized (no raw v echo), out:\n%s", out)
	}
	if strings.Contains(out, "v0.6.2 ->") || strings.Contains(out, "-> v0.6.4") {
		t.Fatalf("displayed transition must not carry raw v prefixes, out:\n%s", out)
	}
}

func TestChangelog_UpToDateNotice(t *testing.T) {
	changelogServer(t, map[string]string{
		"tu": relJSON([3]string{"v0.6.4", "tu4", "body4"}),
	})
	// tu installed at 0.6.4 — already at latest → up-to-date notice + releases URL.
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		if req.Name == brewBinary && len(req.Args) >= 4 && req.Args[0] == "list" && req.Args[3] == formulaPrefix+"tu" {
			return proc.Result{Stdout: []byte("tu 0.6.4\n")}
		}
		return proc.Result{Err: errors.New("not installed")}
	}}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runChangelog(context.Background(), &stdout, &stderr, []string{"tu"}); err != nil {
		t.Fatalf("runChangelog err = %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "up to date at 0.6.4") {
		t.Fatalf("out missing up-to-date notice:\n%s", out)
	}
	if !strings.Contains(out, changelog.ReleasesURL("tu")) {
		t.Fatalf("out missing releases URL:\n%s", out)
	}
}

func TestChangelog_BareSweepIncludesShllSelf(t *testing.T) {
	// Bare `shll changelog` includes shll itself FIRST (symmetry with bare `shll
	// update`), anchored on shll's BREW formula version — not ldflags. Here shll is
	// brew-installed at 0.0.4 with a newer 0.0.5 release; every roster tool is
	// not installed (skipped). shll must render first.
	changelogServer(t, map[string]string{
		"shll": relJSON([3]string{"v0.0.5", "shll5", "b"}, [3]string{"v0.0.4", "old", "b"}),
	})
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		if req.Name == brewBinary && len(req.Args) > 0 && req.Args[0] == "--version" {
			return proc.Result{Stdout: []byte("Homebrew 4.0\n")}
		}
		if req.Name == brewBinary && len(req.Args) >= 4 && req.Args[0] == "list" && req.Args[3] == shllFormula {
			return proc.Result{Stdout: []byte("shll 0.0.4\n")}
		}
		return proc.Result{Err: errors.New("not installed")}
	}}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runChangelog(context.Background(), &stdout, &stderr, nil); err != nil {
		t.Fatalf("runChangelog err = %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "==> [1/1] shll\n") {
		t.Fatalf("bare sweep must render shll first, out:\n%s", out)
	}
	if !strings.Contains(out, "0.0.4 -> 0.0.5 (1 release)") {
		t.Fatalf("shll self entry must anchor on its brew formula version, out:\n%s", out)
	}
}

func TestChangelog_BareSweepZeroInstalledPrintsMessage(t *testing.T) {
	// Bare `shll changelog` with nothing installed (incl. shll) prints the same
	// nothing-to-do line as `shll update`, not silent empty output.
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		if req.Name == brewBinary && len(req.Args) > 0 && req.Args[0] == "--version" {
			return proc.Result{Stdout: []byte("Homebrew 4.0\n")}
		}
		return proc.Result{Err: errors.New("not installed")}
	}}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runChangelog(context.Background(), &stdout, &stderr, nil); err != nil {
		t.Fatalf("runChangelog err = %v", err)
	}
	if got := stdout.String(); got != noToolsInstalledMsg+"\n" {
		t.Fatalf("stdout = %q, want %q", got, noToolsInstalledMsg+"\n")
	}
}

func TestChangelog_NoRangeBrewMissingHint(t *testing.T) {
	// A no-range form needs brew to anchor the range; when brew is absent it prints
	// the same brew-missing hint as update/install and exits non-zero.
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		if req.Name == brewBinary {
			return proc.Result{Err: proc.ErrNotFound}
		}
		return proc.Result{}
	}}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	err := runChangelog(context.Background(), &stdout, &stderr, []string{"tu"})
	if !errors.Is(err, errSilent) {
		t.Fatalf("err = %v, want errSilent when brew missing for a no-range form", err)
	}
	if !strings.Contains(stderr.String(), brewMissingHint) {
		t.Fatalf("stderr = %q, want brew-missing hint", stderr.String())
	}
}

func TestChangelog_ExplicitRangeSkipsBrewCheck(t *testing.T) {
	// A run of ONLY explicit ranges never consults brew, so it must NOT gate on
	// brew presence — it works even when brew is absent.
	changelogServer(t, map[string]string{
		"hop": relJSON([3]string{"v0.1.18", "hop18", "b"}),
	})
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		if req.Name == brewBinary {
			return proc.Result{Err: proc.ErrNotFound} // brew absent
		}
		return proc.Result{}
	}}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runChangelog(context.Background(), &stdout, &stderr, []string{"hop@0.1.17..0.1.18"}); err != nil {
		t.Fatalf("explicit-range run must not gate on brew, err = %v", err)
	}
	if strings.Contains(stderr.String(), brewMissingHint) {
		t.Fatalf("explicit-range run must not print brew-missing hint, stderr = %q", stderr.String())
	}
	if !strings.Contains(stdout.String(), "hop18") {
		t.Fatalf("out missing hop release:\n%s", stdout.String())
	}
}

func TestChangelog_NamedNotInstalledErrorsNoRangeOnly(t *testing.T) {
	// run-kit NOT installed. No-range `shll changelog run-kit` → error.
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		return proc.Result{Err: errors.New("not installed")}
	}}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	err := runChangelog(context.Background(), &stdout, &stderr, []string{"run-kit"})
	if !errors.Is(err, errSilent) {
		t.Fatalf("err = %v, want errSilent for no-range not-installed", err)
	}
	if !strings.Contains(stderr.String(), "shll changelog: run-kit: not installed") {
		t.Fatalf("stderr = %q, want not-installed error", stderr.String())
	}
}

func TestChangelog_LegacyAliasResolvesToRunKit(t *testing.T) {
	// `shll changelog rk` resolves the alias to run-kit (canonicalized in
	// parseChangelogSpecs), so the not-installed error names the canonical run-kit.
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		return proc.Result{Err: errors.New("not installed")}
	}}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	err := runChangelog(context.Background(), &stdout, &stderr, []string{"rk"})
	if !errors.Is(err, errSilent) {
		t.Fatalf("err = %v, want errSilent", err)
	}
	if !strings.Contains(stderr.String(), "shll changelog: run-kit: not installed") {
		t.Fatalf("stderr = %q, want the alias to resolve to canonical run-kit", stderr.String())
	}
}

func TestChangelog_ExplicitRangeWorksUninstalled(t *testing.T) {
	// rk NOT installed, but an explicit range never consults brew and works.
	changelogServer(t, map[string]string{
		"run-kit": relJSON([3]string{"v0.2.0", "rk2", "b"}),
	})
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		return proc.Result{Err: errors.New("not installed")}
	}}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runChangelog(context.Background(), &stdout, &stderr, []string{"rk@0.1.0..0.2.0"}); err != nil {
		t.Fatalf("runChangelog err = %v, want nil (explicit range ignores install state)", err)
	}
	if !strings.Contains(stdout.String(), "rk2") {
		t.Fatalf("out missing rk release (repo slug run-kit must be used):\n%s", stdout.String())
	}
}

func TestChangelog_EmptyRange(t *testing.T) {
	changelogServer(t, map[string]string{
		"tu": relJSON([3]string{"v0.6.4", "tu4", "b"}),
	})
	installFakeRunner(t, &fakeRunner{})

	var stdout, stderr bytes.Buffer
	// old == new → no releases in range.
	if err := runChangelog(context.Background(), &stdout, &stderr, []string{"tu@0.6.4..0.6.4"}); err != nil {
		t.Fatalf("runChangelog err = %v", err)
	}
	if !strings.Contains(stdout.String(), "no releases in range") {
		t.Fatalf("out missing empty-range line:\n%s", stdout.String())
	}
}

func TestChangelog_CapOverflow(t *testing.T) {
	// 12 releases in range → capped at 10 + a cap notice + compare URL.
	triples := make([][3]string, 0, 13)
	for i := 13; i >= 1; i-- { // v0.0.13 .. v0.0.1, newest first in the payload
		triples = append(triples, [3]string{fmt.Sprintf("v0.0.%d", i), fmt.Sprintf("r%d", i), "b"})
	}
	changelogServer(t, map[string]string{"tu": relJSON(triples...)})
	installFakeRunner(t, &fakeRunner{})

	var stdout, stderr bytes.Buffer
	// (0.0.1, 0.0.13] → 12 releases, capped at 10.
	if err := runChangelog(context.Background(), &stdout, &stderr, []string{"tu@0.0.1..0.0.13"}); err != nil {
		t.Fatalf("runChangelog err = %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "(12 releases)") {
		t.Fatalf("out missing total count 12:\n%s", out)
	}
	if !strings.Contains(out, "2 more") || !strings.Contains(out, changelog.CompareURL("tu", "0.0.1", "0.0.13")) {
		t.Fatalf("out missing cap notice + compare URL:\n%s", out)
	}
	// The 11th-oldest (v0.0.2) should NOT appear as a shown release line.
	if strings.Contains(out, "v0.0.2  r2\n") {
		t.Fatalf("out should have capped v0.0.2:\n%s", out)
	}
}

func TestChangelog_UnavailableDegradesToCompareURL(t *testing.T) {
	// Server 404s for hop → unavailable → compare-URL fallback, exit 0.
	changelogServer(t, map[string]string{}) // no repos → 404
	installFakeRunner(t, &fakeRunner{})

	var stdout, stderr bytes.Buffer
	if err := runChangelog(context.Background(), &stdout, &stderr, []string{"hop@0.1.16..0.1.18"}); err != nil {
		t.Fatalf("runChangelog err = %v, want nil (fetch failure must not change exit code)", err)
	}
	out := stdout.String()
	// Non-TTY buffer → ASCII-degraded arrow + dash.
	if !strings.Contains(out, "0.1.16 -> 0.1.18 -- see "+changelog.CompareURL("hop", "0.1.16", "0.1.18")) {
		t.Fatalf("out missing compare-URL fallback:\n%s", out)
	}
}

// --- registration + net/http isolation (change r01z) ---

func TestChangelog_RegisteredOnRoot(t *testing.T) {
	root := newRootCmd()
	found := false
	for _, sub := range root.Commands() {
		if sub.Name() == "changelog" {
			found = true
		}
	}
	if !found {
		t.Error("changelog not registered on root")
	}
	if !strings.Contains(rootLong, "shll changelog") {
		t.Error("rootLong does not document shll changelog")
	}
}

// TestCmdShllNoNetHTTP guards the Constitution I-style isolation: net/http is
// confined to internal/changelog, exactly as os/exec is confined to
// internal/proc. No NON-TEST file in cmd/shll may import net/http — command code
// calls the internal/changelog API instead. (Test files legitimately import
// net/http/httptest to drive the changelog server.)
func TestCmdShllNoNetHTTP(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read cmd/shll dir: %v", err)
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if bytes.Contains(src, []byte(`"net/http"`)) {
			t.Errorf("%s imports net/http — it must be isolated in internal/changelog", name)
		}
		if bytes.Contains(src, []byte(`"os/exec"`)) {
			t.Errorf("%s imports os/exec — it must be isolated in internal/proc", name)
		}
	}
}
