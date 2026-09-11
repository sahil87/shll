package versions

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/sahil87/shll/internal/changelog"
)

// manifestServer starts an httptest.Server answering every request with the
// given status and body, and points the package seams at it for the test.
func manifestServer(t *testing.T, status int, body string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	}))
	restore := SetTransportForTest(srv.URL, srv.Client())
	t.Cleanup(func() {
		srv.Close()
		restore()
	})
}

func TestFetchManifest_HappyPath(t *testing.T) {
	manifestServer(t, http.StatusOK, `{
		"schema": 1,
		"generated_at": "2026-07-19T09:32:11Z",
		"tools": {
			"run-kit": {"latest": "3.8.2", "notify": "minor", "formula": "run-kit"},
			"shll":    {"latest": "0.1.6", "notify": "patch", "formula": "shll"}
		}
	}`)

	m, err := FetchManifest(context.Background())
	if err != nil {
		t.Fatalf("FetchManifest err = %v, want nil", err)
	}
	if m.Schema != 1 {
		t.Errorf("Schema = %d, want 1", m.Schema)
	}
	if m.GeneratedAt != "2026-07-19T09:32:11Z" {
		t.Errorf("GeneratedAt = %q", m.GeneratedAt)
	}
	rk, ok := m.Tools["run-kit"]
	if !ok {
		t.Fatalf("Tools missing run-kit: %+v", m.Tools)
	}
	if rk.Latest != "3.8.2" || rk.Notify != "minor" || rk.Formula != "run-kit" {
		t.Errorf("run-kit entry = %+v", rk)
	}
	if got := m.Tools["shll"].Notify; got != "patch" {
		t.Errorf("shll notify = %q, want patch", got)
	}
}

func TestFetchManifest_Non200IsUnavailable(t *testing.T) {
	manifestServer(t, http.StatusInternalServerError, "boom")
	_, err := FetchManifest(context.Background())
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
}

func TestFetchManifest_MalformedJSONIsUnavailable(t *testing.T) {
	manifestServer(t, http.StatusOK, `{"schema": 1, "tools": [`)
	_, err := FetchManifest(context.Background())
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
}

func TestFetchManifest_UnsupportedSchemaIsUnavailable(t *testing.T) {
	// A future schema bump ships with a shll release that understands it; this
	// binary must fail loudly rather than misread policy from an unknown shape.
	manifestServer(t, http.StatusOK, `{"schema": 2, "tools": {}}`)
	_, err := FetchManifest(context.Background())
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable (unsupported schema)", err)
	}
}

func TestNotable(t *testing.T) {
	type c struct {
		notify, installed, latest string
		want                      bool
	}
	for _, tc := range []c{
		// never → never notable, whatever the bump size.
		{NotifyNever, "3.8.1", "3.8.2", false},
		{NotifyNever, "3.8.1", "4.0.0", false},
		// patch → any pending bump is notable.
		{NotifyPatch, "3.8.1", "3.8.2", true},
		{NotifyPatch, "3.8.1", "3.9.0", true},
		{NotifyPatch, "3.8.1", "4.0.0", true},
		// minor → patch-only bump NOT notable; minor and major are.
		{NotifyMinor, "3.8.1", "3.8.2", false}, // the intake's worked example
		{NotifyMinor, "3.8.1", "3.9.0", true},
		{NotifyMinor, "3.8.1", "4.0.0", true},
		// unknown/future policy values are treated as minor.
		{"weird", "3.8.1", "3.8.2", false},
		{"weird", "3.8.1", "3.9.0", true},
		{"", "3.8.1", "4.0.0", true},
		// no pending bump (equal or ahead, or unresolved sides) → false.
		{NotifyPatch, "3.8.2", "3.8.2", false},
		{NotifyPatch, "3.9.0", "3.8.2", false},
		{NotifyPatch, "", "3.8.2", false},
		{NotifyPatch, "3.8.1", "", false},
		// normalization: v prefix + brew revision suffix stripped before compare.
		{NotifyMinor, "v0.1.5_1", "0.2.0", true},
	} {
		if got := Notable(tc.notify, tc.installed, tc.latest); got != tc.want {
			t.Errorf("Notable(%q, %q, %q) = %v, want %v", tc.notify, tc.installed, tc.latest, got, tc.want)
		}
	}
}

func TestLatestGitHub_DelegatesSingleFetch(t *testing.T) {
	// LatestGitHub must be a single delegation to changelog.LatestTag — exactly
	// one GET per call — and must return the fetched release list so a caller
	// that also needs the releases never fetches twice (the single-GET contract
	// `shll changelog`'s no-range anchor relies on).
	var gets int
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		gets++
		mu.Unlock()
		fmt.Fprint(w, `[{"tag_name":"v0.6.4","name":"t4","body":"b4"},{"tag_name":"v0.6.3","name":"t3","body":"b3"}]`)
	}))
	restore := changelog.SetTransportForTest(srv.URL, srv.Client())
	t.Cleanup(func() { srv.Close(); restore() })

	latest, rels, err := LatestGitHub(context.Background(), "tu")
	if err != nil {
		t.Fatalf("LatestGitHub err = %v", err)
	}
	if latest != "v0.6.4" {
		t.Errorf("latest = %q, want v0.6.4", latest)
	}
	if len(rels) != 2 {
		t.Errorf("len(rels) = %d, want 2 (release list must be returned)", len(rels))
	}
	mu.Lock()
	got := gets
	mu.Unlock()
	if got != 1 {
		t.Errorf("GET count = %d, want exactly 1", got)
	}
}

func TestLatestGitHub_UnavailableDegrades(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	restore := changelog.SetTransportForTest(srv.URL, srv.Client())
	t.Cleanup(func() { srv.Close(); restore() })

	_, _, err := LatestGitHub(context.Background(), "tu")
	if !errors.Is(err, changelog.ErrUnavailable) {
		t.Fatalf("err = %v, want changelog.ErrUnavailable", err)
	}
}

// validManifest is a minimal schema-1 body whose single tool entry lets a test
// tell which server answered.
func validManifest(tool string) string {
	return fmt.Sprintf(`{"schema": 1, "generated_at": "2026-09-11T00:00:00Z", "tools": {%q: {"latest": "1.0.0", "notify": "minor", "formula": %q}}}`, tool, tool)
}

// manifestServers starts a primary and a fallback httptest.Server with the given
// handlers and points the ordered manifestURLs seam at them (primary first) for
// the test — the fallback-ordering counterpart of manifestServer's single-URL
// collapse.
func manifestServers(t *testing.T, primary, fallback http.HandlerFunc) {
	t.Helper()
	p := httptest.NewServer(primary)
	f := httptest.NewServer(fallback)
	prevURLs, prevClient := manifestURLs, httpClient
	manifestURLs = []string{p.URL, f.URL}
	httpClient = p.Client()
	t.Cleanup(func() {
		p.Close()
		f.Close()
		manifestURLs, httpClient = prevURLs, prevClient
	})
}

func serve(status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	}
}

func TestFetchManifest_PrimaryFailureFallsThrough(t *testing.T) {
	// Every failure class the single-URL path degrades on must move the fetch
	// to the next URL rather than end it: the fallback host is the whole point
	// of the ordered list during the shll.ai → hexokit.com cutover.
	for name, primary := range map[string]http.HandlerFunc{
		"non-200":            serve(http.StatusInternalServerError, "boom"),
		"malformed JSON":     serve(http.StatusOK, `{"schema": 1, "tools": [`),
		"unsupported schema": serve(http.StatusOK, `{"schema": 2, "tools": {}}`),
	} {
		t.Run(name, func(t *testing.T) {
			manifestServers(t, primary, serve(http.StatusOK, validManifest("from-fallback")))
			m, err := FetchManifest(context.Background())
			if err != nil {
				t.Fatalf("FetchManifest err = %v, want nil (fallback should have answered)", err)
			}
			if _, ok := m.Tools["from-fallback"]; !ok {
				t.Errorf("Tools = %+v, want the fallback server's manifest", m.Tools)
			}
		})
	}
}

func TestFetchManifest_PrimaryRefusedFallsThrough(t *testing.T) {
	// A transport error (connection refused — the host is down, not merely
	// erroring) is the fallback's headline case.
	dead := httptest.NewServer(http.NotFoundHandler())
	deadURL := dead.URL
	dead.Close()
	f := httptest.NewServer(serve(http.StatusOK, validManifest("from-fallback")))
	prevURLs, prevClient := manifestURLs, httpClient
	manifestURLs = []string{deadURL, f.URL}
	httpClient = f.Client()
	t.Cleanup(func() {
		f.Close()
		manifestURLs, httpClient = prevURLs, prevClient
	})

	m, err := FetchManifest(context.Background())
	if err != nil {
		t.Fatalf("FetchManifest err = %v, want nil", err)
	}
	if _, ok := m.Tools["from-fallback"]; !ok {
		t.Errorf("Tools = %+v, want the fallback server's manifest", m.Tools)
	}
}

func TestFetchManifest_PrimarySuccessSkipsFallback(t *testing.T) {
	// The happy path must cost exactly one request: a reachable primary means
	// the fallback host is never contacted.
	var fallbackHits int
	var mu sync.Mutex
	manifestServers(t,
		serve(http.StatusOK, validManifest("from-primary")),
		func(w http.ResponseWriter, _ *http.Request) {
			mu.Lock()
			fallbackHits++
			mu.Unlock()
			fmt.Fprint(w, validManifest("from-fallback"))
		})

	m, err := FetchManifest(context.Background())
	if err != nil {
		t.Fatalf("FetchManifest err = %v, want nil", err)
	}
	if _, ok := m.Tools["from-primary"]; !ok {
		t.Errorf("Tools = %+v, want the primary server's manifest", m.Tools)
	}
	mu.Lock()
	hits := fallbackHits
	mu.Unlock()
	if hits != 0 {
		t.Errorf("fallback hits = %d, want 0 (primary succeeded)", hits)
	}
}

func TestFetchManifest_AllUnavailable(t *testing.T) {
	manifestServers(t, serve(http.StatusInternalServerError, "boom"), serve(http.StatusBadGateway, "worse"))
	_, err := FetchManifest(context.Background())
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable when every URL fails", err)
	}
}

func TestSetTransportForTest_CollapsesToSingleURL(t *testing.T) {
	// The exported seam collapses the ordered list to the one test URL — the
	// contract cmd/shll's check_updates_test.go relies on — and the restore
	// func puts the production list back.
	prev := append([]string(nil), manifestURLs...)
	restore := SetTransportForTest("http://127.0.0.1:1/versions.json", nil)
	if len(manifestURLs) != 1 || manifestURLs[0] != "http://127.0.0.1:1/versions.json" {
		t.Errorf("manifestURLs during test = %v, want the single test URL", manifestURLs)
	}
	restore()
	if len(manifestURLs) != len(prev) || manifestURLs[0] != prev[0] || manifestURLs[1] != prev[1] {
		t.Errorf("manifestURLs after restore = %v, want %v", manifestURLs, prev)
	}
	if manifestURLs[0] != manifestURLDefault || manifestURLs[1] != manifestURLFallback {
		t.Errorf("production list = %v, want [%s %s]", manifestURLs, manifestURLDefault, manifestURLFallback)
	}
}

func TestFetchManifest_PrimaryBodyReadFailureFallsThrough(t *testing.T) {
	// A response that dies mid-body (Content-Length promises more bytes than
	// arrive) is the read-body failure class: io.ReadAll returns an unexpected
	// EOF, which must fall through to the fallback exactly like a non-200.
	truncated := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "4096")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"schema": 1,`)
		// Returning here closes the connection short of the declared length.
	}
	var fallbackHits int
	var mu sync.Mutex
	manifestServers(t, truncated, func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		fallbackHits++
		mu.Unlock()
		fmt.Fprint(w, validManifest("from-fallback"))
	})

	m, err := FetchManifest(context.Background())
	if err != nil {
		t.Fatalf("FetchManifest err = %v, want nil (fallback should have answered)", err)
	}
	if _, ok := m.Tools["from-fallback"]; !ok {
		t.Errorf("Tools = %+v, want the fallback server's manifest", m.Tools)
	}
	mu.Lock()
	hits := fallbackHits
	mu.Unlock()
	if hits != 1 {
		t.Errorf("fallback hits = %d, want exactly 1", hits)
	}
}
