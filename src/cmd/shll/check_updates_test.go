package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sahil87/shll/internal/proc"
	"github.com/sahil87/shll/internal/versions"
)

// checkUpdatesManifestServer starts an httptest.Server answering every request
// with the given status/body and points the internal/versions manifest seams at
// it for the test (mirroring changelogServer for the GitHub seam).
func checkUpdatesManifestServer(t *testing.T, status int, body string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	}))
	restore := versions.SetTransportForTest(srv.URL, srv.Client())
	t.Cleanup(func() {
		srv.Close()
		restore()
	})
}

// checkUpdatesManifestGuard points the manifest seam at a server that FAILS the
// test if it is ever hit — for cases that must return before any manifest fetch
// (usage errors, the brew gate).
func checkUpdatesManifestGuard(t *testing.T) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("manifest endpoint was fetched — this path must return before any network access")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	restore := versions.SetTransportForTest(srv.URL, srv.Client())
	t.Cleanup(func() {
		srv.Close()
		restore()
	})
}

// checkUpdatesBrewFake returns a fakeRunner where brew is present and
// `brew list --formula --versions <formula>` answers from the installed map
// (formula → stdout line); formulas absent from the map are not installed.
func checkUpdatesBrewFake(installed map[string]string) *fakeRunner {
	return &fakeRunner{respond: func(req proc.Request) proc.Result {
		if req.Name != brewBinary {
			return proc.Result{Err: proc.ErrNotFound}
		}
		if len(req.Args) == 1 && req.Args[0] == "--version" {
			return proc.Result{Stdout: []byte("Homebrew 6.0.4\n")}
		}
		if len(req.Args) == 4 && req.Args[0] == "list" && req.Args[1] == "--formula" && req.Args[2] == "--versions" {
			if line, ok := installed[req.Args[3]]; ok {
				return proc.Result{Stdout: []byte(line)}
			}
			return proc.Result{Err: errors.New("exit 1: not installed")}
		}
		return proc.Result{Err: fmt.Errorf("unexpected brew invocation: %v", req.Args)}
	}}
}

// happyManifest is a schema-1 manifest where shll has a notable patch bump
// (notify: patch), wt is up to date, and run-kit has a non-notable patch bump
// (notify: minor — the intake's worked example).
const happyManifest = `{
	"schema": 1,
	"generated_at": "2026-07-19T09:32:11Z",
	"tools": {
		"shll":    {"latest": "0.1.6", "notify": "patch", "formula": "shll"},
		"wt":      {"latest": "0.1.3", "notify": "minor", "formula": "wt"},
		"run-kit": {"latest": "3.8.2", "notify": "minor", "formula": "run-kit"}
	}
}`

// happyBrew installs shll@0.1.5, wt@0.1.3, run-kit@3.8.1; everything else is
// missing.
func happyBrew() map[string]string {
	return map[string]string{
		shllFormula:               "shll 0.1.5\n",
		formulaPrefix + "wt":      "wt 0.1.3\n",
		formulaPrefix + "run-kit": "run-kit 3.8.1\n",
	}
}

func TestCheckUpdates_ReleasedHappyPathTable(t *testing.T) {
	checkUpdatesManifestServer(t, http.StatusOK, happyManifest)
	installFakeRunner(t, checkUpdatesBrewFake(happyBrew()))

	var stdout, stderr bytes.Buffer
	if err := runCheckUpdates(context.Background(), &stdout, &stderr, sourceReleased, false); err != nil {
		t.Fatalf("runCheckUpdates err = %v, want nil", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	out := stdout.String()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	// One row per tool: shll first, then the leaves-first roster — 7 rows total.
	if len(lines) != len(Roster)+1 {
		t.Fatalf("line count = %d, want %d. output:\n%s", len(lines), len(Roster)+1, out)
	}
	// shll first: notable patch bump under notify:patch. Non-TTY buffer → the
	// arrow ASCII-degrades to `->` and no ANSI is emitted.
	if !strings.HasPrefix(lines[0], "shll") || !strings.Contains(lines[0], "0.1.5 -> 0.1.6") || !strings.Contains(lines[0], checkStatusUpdate+checkStatusNotableSuffix) {
		t.Errorf("shll row = %q, want installed -> latest + %q", lines[0], checkStatusUpdate+checkStatusNotableSuffix)
	}
	// wt (roster position 1): up to date, single version shown.
	if !strings.HasPrefix(lines[1], "wt") || !strings.Contains(lines[1], "0.1.3") || !strings.Contains(lines[1], checkStatusUpToDate) {
		t.Errorf("wt row = %q, want up to date at 0.1.3", lines[1])
	}
	// idea (roster position 2): not installed, version column carries the label.
	if !strings.HasPrefix(lines[2], "idea") || !strings.Contains(lines[2], notInstalledLabel) {
		t.Errorf("idea row = %q, want %q", lines[2], notInstalledLabel)
	}
	// run-kit (roster position 4): patch bump under notify:minor → update
	// available but NOT notable (the intake's worked example).
	if !strings.HasPrefix(lines[4], "run-kit") || !strings.Contains(lines[4], "3.8.1 -> 3.8.2") || !strings.Contains(lines[4], checkStatusUpdate) {
		t.Errorf("run-kit row = %q, want 3.8.1 -> 3.8.2 update available", lines[4])
	}
	if strings.Contains(lines[4], checkStatusNotableSuffix) {
		t.Errorf("run-kit row = %q — a patch-only bump under notify:minor must NOT be notable", lines[4])
	}
	// A self-labeling aggregation: no per-tool headers, no ANSI.
	if strings.Contains(out, "==>") || strings.Contains(out, "▸") || strings.Contains(out, "\033[") {
		t.Errorf("output must carry no per-tool headers and no ANSI:\n%s", out)
	}
}

func TestCheckUpdates_JSONContractReleased(t *testing.T) {
	checkUpdatesManifestServer(t, http.StatusOK, happyManifest)
	installFakeRunner(t, checkUpdatesBrewFake(happyBrew()))

	var stdout, stderr bytes.Buffer
	if err := runCheckUpdates(context.Background(), &stdout, &stderr, sourceReleased, true); err != nil {
		t.Fatalf("runCheckUpdates err = %v, want nil", err)
	}
	raw := stdout.String()

	var report struct {
		Schema int    `json:"schema"`
		Source string `json:"source"`
		Tools  []struct {
			Name            string `json:"name"`
			Formula         string `json:"formula"`
			Installed       string `json:"installed"`
			Latest          string `json:"latest"`
			Notify          string `json:"notify"`
			UpdateAvailable bool   `json:"update_available"`
			Notable         *bool  `json:"notable"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, raw)
	}
	if report.Schema != checkUpdatesSchema || report.Source != sourceReleased {
		t.Errorf("envelope = schema %d source %q, want %d %q", report.Schema, report.Source, checkUpdatesSchema, sourceReleased)
	}
	// Unresolvable-row rule: only shll, wt, run-kit resolve (both sides known);
	// the four uninstalled tools are omitted. Order: shll first, then roster.
	if len(report.Tools) != 3 {
		t.Fatalf("tools len = %d, want 3 (unresolved rows omitted):\n%s", len(report.Tools), raw)
	}
	shll, wt, rk := report.Tools[0], report.Tools[1], report.Tools[2]
	if shll.Name != "shll" || wt.Name != "wt" || rk.Name != "run-kit" {
		t.Fatalf("row order = %s, %s, %s — want shll, wt, run-kit", shll.Name, wt.Name, rk.Name)
	}
	// The intake's worked example row, field by field.
	if rk.Formula != "run-kit" || rk.Installed != "3.8.1" || rk.Latest != "3.8.2" || rk.Notify != "minor" {
		t.Errorf("run-kit row = %+v", rk)
	}
	if !rk.UpdateAvailable || rk.Notable == nil || *rk.Notable {
		t.Errorf("run-kit verdicts = update_available %v notable %v, want true / false (patch bump, notify:minor)", rk.UpdateAvailable, rk.Notable)
	}
	if !shll.UpdateAvailable || shll.Notable == nil || !*shll.Notable {
		t.Errorf("shll verdicts = update_available %v notable %v, want true / true (patch bump, notify:patch)", shll.UpdateAvailable, shll.Notable)
	}
	if wt.UpdateAvailable || wt.Notable == nil || *wt.Notable {
		t.Errorf("wt verdicts = update_available %v notable %v, want false / false (up to date)", wt.UpdateAvailable, wt.Notable)
	}
	// Released rows carry an EXPLICIT "notable": false — never omitted (the
	// *bool + omitempty encoding must not drop the false value here).
	if !strings.Contains(raw, `"notable": false`) {
		t.Errorf("released output must carry an explicit \"notable\": false:\n%s", raw)
	}
	// list/doctor encoder precedent: 2-space indent, trailing newline.
	if !strings.HasPrefix(raw, "{\n  ") || !strings.HasSuffix(raw, "}\n") {
		t.Errorf("output must be 2-space-indented with a trailing newline:\n%q", raw)
	}
}

func TestCheckUpdates_GithubJSONOmitsNotifyNotable(t *testing.T) {
	// GitHub backend: latest tags come from the (test) GitHub API via the
	// internal/changelog seam. No notify policy exists, so rows omit the notify
	// and notable keys entirely — honest omission, not invented defaults.
	changelogServer(t, map[string]string{
		"shll": relJSON([3]string{"v0.1.6", "s6", "b"}),
		"wt":   relJSON([3]string{"v0.1.3", "w3", "b"}),
	})
	checkUpdatesManifestGuard(t) // the github backend must never fetch the manifest
	installFakeRunner(t, checkUpdatesBrewFake(happyBrew()))

	var stdout, stderr bytes.Buffer
	if err := runCheckUpdates(context.Background(), &stdout, &stderr, sourceGithub, true); err != nil {
		t.Fatalf("runCheckUpdates err = %v, want nil", err)
	}
	raw := stdout.String()
	var report checkUpdatesReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, raw)
	}
	if report.Source != sourceGithub {
		t.Errorf("source = %q, want %q", report.Source, sourceGithub)
	}
	// run-kit's repo is absent from the server map (404 → per-tool degrade), so
	// only shll and wt resolve.
	if len(report.Tools) != 2 || report.Tools[0].Name != "shll" || report.Tools[1].Name != "wt" {
		t.Fatalf("tools = %+v, want shll, wt", report.Tools)
	}
	if !report.Tools[0].UpdateAvailable {
		t.Errorf("shll update_available = false, want true (0.1.5 -> 0.1.6)")
	}
	if strings.Contains(raw, `"notify"`) || strings.Contains(raw, `"notable"`) {
		t.Errorf("github rows must omit the notify/notable keys entirely:\n%s", raw)
	}
}

func TestCheckUpdates_GithubPerToolFailureDegrades(t *testing.T) {
	// One repo's fetch fails (404) while another succeeds: the failed tool
	// degrades per-tool — `unavailable` in the human table, omitted from the
	// JSON — and the run still exits 0 (Constitution V, the changelog
	// degradation precedent).
	changelogServer(t, map[string]string{
		"tu": relJSON([3]string{"v0.6.4", "t4", "b"}),
	})
	installFakeRunner(t, checkUpdatesBrewFake(map[string]string{
		formulaPrefix + "wt": "wt 0.1.3\n",
		formulaPrefix + "tu": "tu 0.6.2\n",
	}))

	var stdout, stderr bytes.Buffer
	if err := runCheckUpdates(context.Background(), &stdout, &stderr, sourceGithub, false); err != nil {
		t.Fatalf("runCheckUpdates err = %v, want nil (per-tool degradation)", err)
	}
	out := stdout.String()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if !strings.HasPrefix(lines[1], "wt") || !strings.Contains(lines[1], "0.1.3") || !strings.Contains(lines[1], checkStatusUnavailable) {
		t.Errorf("wt row = %q, want installed version + %q", lines[1], checkStatusUnavailable)
	}
	if !strings.HasPrefix(lines[3], "tu") || !strings.Contains(lines[3], "0.6.2 -> 0.6.4") {
		t.Errorf("tu row = %q, want a resolved transition", lines[3])
	}

	// And the JSON run omits the failed row.
	stdout.Reset()
	if err := runCheckUpdates(context.Background(), &stdout, &stderr, sourceGithub, true); err != nil {
		t.Fatalf("runCheckUpdates (json) err = %v", err)
	}
	var report checkUpdatesReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("json: %v", err)
	}
	if len(report.Tools) != 1 || report.Tools[0].Name != "tu" {
		t.Errorf("tools = %+v, want just tu (wt's failed fetch omitted)", report.Tools)
	}
}

func TestCheckUpdates_UnknownSourceValueUsageError(t *testing.T) {
	checkUpdatesManifestGuard(t)
	f := checkUpdatesBrewFake(nil)
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	err := runCheckUpdates(context.Background(), &stdout, &stderr, "bogus", false)
	var ec *errExitCode
	if !errors.As(err, &ec) || ec.code != usageExitCode {
		t.Fatalf("err = %v, want errExitCode{code: %d}", err, usageExitCode)
	}
	if !strings.Contains(ec.msg, `"bogus"`) {
		t.Errorf("msg = %q, want the offending value named", ec.msg)
	}
	if !strings.Contains(ec.msg, sourceReleased) || !strings.Contains(ec.msg, sourceGithub) {
		t.Errorf("msg = %q, want the valid set named (%s, %s)", ec.msg, sourceReleased, sourceGithub)
	}
	// The usage error fires before any brew or network access.
	if calls := f.recordedCalls(); len(calls) != 0 {
		t.Errorf("recorded %d subprocess calls, want 0 (usage error must precede all work): %v", len(calls), calls)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
}

func TestCheckUpdates_ManifestFetchFailureExit1(t *testing.T) {
	checkUpdatesManifestServer(t, http.StatusInternalServerError, "boom")
	installFakeRunner(t, checkUpdatesBrewFake(happyBrew()))

	var stdout, stderr bytes.Buffer
	err := runCheckUpdates(context.Background(), &stdout, &stderr, sourceReleased, true)
	if !errors.Is(err, errSilent) {
		t.Fatalf("err = %v, want errSilent (whole check fails — one fetch)", err)
	}
	if !strings.Contains(stderr.String(), "shll check-updates:") {
		t.Errorf("stderr = %q, want a shll check-updates diagnostic", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty (no partial JSON on a failed check)", stdout.String())
	}
}

func TestCheckUpdates_UnsupportedSchemaFailsCheck(t *testing.T) {
	checkUpdatesManifestServer(t, http.StatusOK, `{"schema": 2, "tools": {}}`)
	installFakeRunner(t, checkUpdatesBrewFake(happyBrew()))

	var stdout, stderr bytes.Buffer
	if err := runCheckUpdates(context.Background(), &stdout, &stderr, sourceReleased, false); !errors.Is(err, errSilent) {
		t.Fatalf("err = %v, want errSilent (unsupported manifest schema)", err)
	}
	if stderr.Len() == 0 {
		t.Error("stderr empty, want a diagnostic")
	}
}

func TestCheckUpdates_BrewMissingHint(t *testing.T) {
	checkUpdatesManifestGuard(t) // the gate precedes any backend fetch
	installFakeRunner(t, &fakeRunner{respond: func(req proc.Request) proc.Result {
		return proc.Result{Err: proc.ErrNotFound}
	}})

	var stdout, stderr bytes.Buffer
	err := runCheckUpdates(context.Background(), &stdout, &stderr, sourceReleased, false)
	if !errors.Is(err, errSilent) {
		t.Fatalf("err = %v, want errSilent", err)
	}
	if got := strings.TrimSpace(stderr.String()); got != brewMissingHint {
		t.Errorf("stderr = %q, want %q", got, brewMissingHint)
	}
}

func TestCheckUpdates_NotInManifestRow(t *testing.T) {
	// A tool installed locally but absent from the manifest: the human row says
	// so (nothing hidden from humans); the JSON omits it (unresolvable-row rule).
	checkUpdatesManifestServer(t, http.StatusOK, `{
		"schema": 1,
		"tools": {"shll": {"latest": "0.1.5", "notify": "patch", "formula": "shll"}}
	}`)
	installFakeRunner(t, checkUpdatesBrewFake(happyBrew()))

	var stdout, stderr bytes.Buffer
	if err := runCheckUpdates(context.Background(), &stdout, &stderr, sourceReleased, false); err != nil {
		t.Fatalf("runCheckUpdates err = %v", err)
	}
	lines := strings.Split(strings.TrimRight(stdout.String(), "\n"), "\n")
	if !strings.HasPrefix(lines[1], "wt") || !strings.Contains(lines[1], "0.1.3") || !strings.Contains(lines[1], checkStatusNotInManifest) {
		t.Errorf("wt row = %q, want installed version + %q", lines[1], checkStatusNotInManifest)
	}

	stdout.Reset()
	if err := runCheckUpdates(context.Background(), &stdout, &stderr, sourceReleased, true); err != nil {
		t.Fatalf("runCheckUpdates (json) err = %v", err)
	}
	var report checkUpdatesReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("json: %v", err)
	}
	if len(report.Tools) != 1 || report.Tools[0].Name != "shll" {
		t.Errorf("tools = %+v, want just shll (wt not in manifest → omitted)", report.Tools)
	}
}

func TestCheckUpdates_EmptyResolvedSetEmitsEmptyArray(t *testing.T) {
	// Nothing installed: tools is [] — never null — so consumers index
	// unconditionally.
	checkUpdatesManifestServer(t, http.StatusOK, `{"schema": 1, "tools": {}}`)
	installFakeRunner(t, checkUpdatesBrewFake(nil))

	var stdout, stderr bytes.Buffer
	if err := runCheckUpdates(context.Background(), &stdout, &stderr, sourceReleased, true); err != nil {
		t.Fatalf("runCheckUpdates err = %v", err)
	}
	if !strings.Contains(stdout.String(), `"tools": []`) {
		t.Errorf("output must carry \"tools\": [] (never null):\n%s", stdout.String())
	}
}

func TestCheckUpdates_RegisteredInRoot(t *testing.T) {
	// The subcommand is wired into the root and listed in the user-facing
	// rootLong surface (thirteen subcommands; help-dump picks it up from the
	// same tree walk automatically).
	root := newRootCmd()
	found := false
	for _, sub := range root.Commands() {
		if sub.Name() == "check-updates" {
			found = true
			if sub.Hidden {
				t.Error("check-updates must be user-facing, not hidden")
			}
		}
	}
	if !found {
		t.Fatal("check-updates is not registered on the root command")
	}
	if !strings.Contains(rootLong, "shll check-updates") {
		t.Error("rootLong must list shll check-updates")
	}
}
