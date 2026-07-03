package changelog

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// withServer swaps the package-level baseURL/httpClient seams to point at a test
// server for the duration of the test, restoring them afterward. It mirrors
// update_test.go's installFakeRunner t.Cleanup pattern so real net/http code
// paths (status codes, JSON decode) run against a local server, no network.
func withServer(t *testing.T, h http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(h)
	prevBase, prevClient := baseURL, httpClient
	baseURL = srv.URL
	httpClient = srv.Client()
	t.Cleanup(func() {
		srv.Close()
		baseURL = prevBase
		httpClient = prevClient
	})
}

// releasesJSON renders a GitHub-releases-shaped JSON array from (tag,title,body)
// triples for the fake server to return.
func releasesJSON(triples ...[3]string) string {
	out := "["
	for i, tr := range triples {
		if i > 0 {
			out += ","
		}
		out += fmt.Sprintf(`{"tag_name":%q,"name":%q,"body":%q}`, tr[0], tr[1], tr[2])
	}
	return out + "]"
}

func TestFetchReleases_HappyPath(t *testing.T) {
	var gotPath, gotAuth string
	withServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.RequestURI()
		gotAuth = r.Header.Get("Authorization")
		fmt.Fprint(w, releasesJSON(
			[3]string{"v0.6.4", "fix: opencode session parsing", "body4"},
			[3]string{"v0.6.3", "feat: daily usage rollups", "body3"},
		))
	})

	rels, err := fetchReleases(context.Background(), "tu")
	if err != nil {
		t.Fatalf("fetchReleases err = %v, want nil", err)
	}
	// Request goes to the org/repo path with per_page=100, unauthenticated.
	wantPath := "/repos/sahil87/tu/releases?per_page=100"
	if gotPath != wantPath {
		t.Fatalf("request path = %q, want %q", gotPath, wantPath)
	}
	if gotAuth != "" {
		t.Fatalf("Authorization = %q, want empty (unauthenticated)", gotAuth)
	}
	if len(rels) != 2 || rels[0].Tag != "v0.6.4" || rels[0].Title != "fix: opencode session parsing" || rels[0].Body != "body4" {
		t.Fatalf("decoded releases = %+v, want tag/name/body populated", rels)
	}
}

func TestFetchReleases_RateLimitedIsUnavailable(t *testing.T) {
	withServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden) // 403 rate-limit
		fmt.Fprint(w, `{"message":"API rate limit exceeded"}`)
	})

	_, err := fetchReleases(context.Background(), "hop")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want wrapping ErrUnavailable", err)
	}
}

func TestFetchReleases_MalformedJSONIsUnavailable(t *testing.T) {
	withServer(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{not json`)
	})

	_, err := fetchReleases(context.Background(), "hop")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want wrapping ErrUnavailable", err)
	}
}

func TestFetchRange_UnavailableVsEmpty(t *testing.T) {
	// Unavailable: server 500s → Result.Unavailable true, Err set.
	withServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	res := FetchRange(context.Background(), RangeReq{Tool: "hop", Repo: "hop", Old: "0.1.16", New: "0.1.18"})
	if !res.Unavailable || res.Err == nil {
		t.Fatalf("Result = %+v, want Unavailable=true with Err set", res)
	}

	// Empty-in-range: server 200s with releases OUTSIDE the range → Unavailable
	// false, zero releases (distinct from unavailable).
	withServer(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, releasesJSON([3]string{"v0.1.10", "old", "b"}))
	})
	res = FetchRange(context.Background(), RangeReq{Tool: "hop", Repo: "hop", Old: "0.1.16", New: "0.1.18"})
	if res.Unavailable {
		t.Fatalf("Result.Unavailable = true, want false (fetch ok, just empty range)")
	}
	if len(res.Releases) != 0 {
		t.Fatalf("Releases = %+v, want empty (v0.1.10 is below the range)", res.Releases)
	}
}

func TestNormalizeVer(t *testing.T) {
	cases := map[string]string{
		"v0.6.4":   "0.6.4",
		"0.6.4":    "0.6.4",
		"0.6.4_1":  "0.6.4",
		"v0.6.4_2": "0.6.4",
		"  v1.2 ":  "1.2",
	}
	for in, want := range cases {
		if got := normalizeVer(in); got != want {
			t.Errorf("normalizeVer(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCompareVer(t *testing.T) {
	type c struct {
		a, b string
		want int
	}
	for _, tc := range []c{
		{"v0.6.4", "0.6.3", 1},
		{"0.6.3", "v0.6.4", -1},
		{"0.6.4", "0.6.4", 0},
		{"0.6", "0.6.0", 0},     // missing components treated as 0
		{"0.6.4_1", "0.6.4", 0}, // brew revision suffix stripped
		{"1.0.0", "0.9.9", 1},
		{"0.10.0", "0.9.0", 1}, // numeric, not lexical, compare
	} {
		if got := compareVer(tc.a, tc.b); got != tc.want {
			t.Errorf("compareVer(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestReleasesInRange_HalfOpenNewestFirst(t *testing.T) {
	rels := []Release{
		{Tag: "v0.6.2"}, {Tag: "v0.6.3"}, {Tag: "v0.6.4"},
	}
	got := releasesInRange(rels, "0.6.2", "0.6.4")
	// (0.6.2, 0.6.4]: excludes 0.6.2, includes 0.6.3 and 0.6.4, newest first.
	if len(got) != 2 || got[0].Tag != "v0.6.4" || got[1].Tag != "v0.6.3" {
		t.Fatalf("releasesInRange = %+v, want [v0.6.4 v0.6.3]", got)
	}

	// old == new → empty.
	if empty := releasesInRange(rels, "0.6.4", "0.6.4"); len(empty) != 0 {
		t.Fatalf("releasesInRange(old==new) = %+v, want empty", empty)
	}
}

func TestFetchAll_PreservesRequestOrder(t *testing.T) {
	// Return distinct release sets per repo; assert results align to the request
	// order regardless of concurrent completion.
	withServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/sahil87/tu/releases":
			fmt.Fprint(w, releasesJSON([3]string{"v0.6.4", "tu4", "b"}))
		case "/repos/sahil87/hop/releases":
			fmt.Fprint(w, releasesJSON([3]string{"v0.1.18", "hop18", "b"}))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	reqs := []RangeReq{
		{Tool: "tu", Repo: "tu", Old: "0.6.3", New: "0.6.4"},
		{Tool: "hop", Repo: "hop", Old: "0.1.17", New: "0.1.18"},
	}
	res := FetchAll(context.Background(), reqs)
	if len(res) != 2 || res[0].Tool != "tu" || res[1].Tool != "hop" {
		t.Fatalf("FetchAll order = [%s %s], want [tu hop]", res[0].Tool, res[1].Tool)
	}
}

func TestLatestTag(t *testing.T) {
	withServer(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, releasesJSON(
			[3]string{"v0.6.3", "t3", "b"},
			[3]string{"v0.6.4", "t4", "b"},
			[3]string{"v0.6.2", "t2", "b"},
		))
	})
	latest, rels, err := LatestTag(context.Background(), "tu")
	if err != nil {
		t.Fatalf("LatestTag err = %v", err)
	}
	if latest != "v0.6.4" {
		t.Fatalf("latest = %q, want v0.6.4 (highest by version, not list order)", latest)
	}
	if len(rels) != 3 {
		t.Fatalf("rels len = %d, want 3 (returned for caller reuse)", len(rels))
	}
}

func TestURLHelpers(t *testing.T) {
	if got := CompareURL("tu", "0.6.2", "0.6.4"); got != "https://github.com/sahil87/tu/compare/v0.6.2...v0.6.4" {
		t.Errorf("CompareURL = %q", got)
	}
	// v-prefix already present is not doubled.
	if got := CompareURL("hop", "v0.1.16", "v0.1.18"); got != "https://github.com/sahil87/hop/compare/v0.1.16...v0.1.18" {
		t.Errorf("CompareURL (v-prefixed) = %q", got)
	}
	// rk's repo slug is run-kit — the helper takes the repo, not the tool name.
	if got := ReleasesURL("run-kit"); got != "https://github.com/sahil87/run-kit/releases" {
		t.Errorf("ReleasesURL = %q", got)
	}
}
