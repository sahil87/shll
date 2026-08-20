package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/sahil87/shll/internal/proc"
)

// installKey is the key a fake builder's installed-map uses for a tool: its
// Formula for brew-managed tools, its Name for delegated (formula-less) tools.
func installKey(t Tool) string {
	if t.brewManaged() {
		return t.Formula
	}
	return t.Name
}

// versionFake constructs a fakeRunner that simulates per-tool installation and
// version output. For a tool whose installKey is absent from installedFormulas,
// the fake returns proc.ErrNotFound from `<tool> --version`, mirroring real
// exec.LookPath behavior when the binary is missing from PATH. The delegated
// (non-brew) rk-desktop entry has no formula: its simulated install state keys
// on its Name, and its probe (`rk desktop status`) answers with an `Installed:`
// line or the absent value.
func versionFake(installedFormulas map[string]bool, versions map[string]string) *fakeRunner {
	formulaByName := map[string]string{}
	var delegated *Tool
	for _, t := range Roster {
		if t.brewManaged() {
			formulaByName[t.Name] = t.Formula
		} else {
			t := t
			delegated = &t
		}
	}
	return &fakeRunner{respond: func(req proc.Request) proc.Result {
		// Simulate ErrNotFound for tools whose formula isn't installed.
		if formula, ok := formulaByName[req.Name]; ok && !installedFormulas[formula] {
			return proc.Result{Err: proc.ErrNotFound}
		}
		// Delegated probe: `rk desktop status`. rk itself stays on the simulated
		// PATH; the rk-desktop install state drives the `Installed:` line.
		if delegated != nil && req.Name == delegated.Probe.Argv[0] && strings.Join(req.Args, " ") == strings.Join(delegated.Probe.Argv[1:], " ") {
			if installedFormulas[delegated.Name] {
				return proc.Result{Stdout: []byte(delegated.Probe.LinePrefix + " v0.1.0\n")}
			}
			return proc.Result{Stdout: []byte(delegated.Probe.LinePrefix + " " + delegated.Probe.AbsentValue + "\n")}
		}
		// Match per-tool --version invocations: req.Name is the tool name,
		// args[0] is "--version".
		if len(req.Args) == 1 && req.Args[0] == "--version" {
			if v, ok := versions[req.Name]; ok {
				return proc.Result{Stdout: []byte(v)}
			}
		}
		return proc.Result{}
	}}
}

func TestVersion_AllInstalled(t *testing.T) {
	installed := map[string]bool{}
	versions := map[string]string{}
	for _, tool := range Roster {
		installed[installKey(tool)] = true
		versions[tool.Name] = tool.Name + " v0.1.0\n"
	}
	installFakeRunner(t, versionFake(installed, versions))

	prevVer := version
	version = "v9.9.9"
	t.Cleanup(func() { version = prevVer })

	var stdout bytes.Buffer
	if err := runVersion(context.Background(), &stdout); err != nil {
		t.Fatalf("runVersion err = %v", err)
	}
	out := stdout.String()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	want := 1 + len(Roster)
	if len(lines) != want {
		t.Fatalf("line count = %d, want %d. output:\n%s", len(lines), want, out)
	}
	if !strings.HasPrefix(lines[0], "shll") || !strings.Contains(lines[0], "v9.9.9") {
		t.Fatalf("first line = %q, want shll v9.9.9", lines[0])
	}
	for i, tool := range Roster {
		if !strings.HasPrefix(lines[i+1], tool.Name) {
			t.Errorf("line %d = %q, want to start with %q", i+1, lines[i+1], tool.Name)
		}
		if !strings.Contains(lines[i+1], "v0.1.0") {
			t.Errorf("line %d = %q, want to contain v0.1.0", i+1, lines[i+1])
		}
		// After normalization, the row MUST NOT contain the redundant
		// `<tool.Name> v0.1.0` substring — only the normalized token.
		if strings.Contains(lines[i+1], tool.Name+" v0.1.0") {
			t.Errorf("line %d = %q, must not contain raw %q after normalization", i+1, lines[i+1], tool.Name+" v0.1.0")
		}
	}
}

func TestVersion_SomeMissing(t *testing.T) {
	installed := map[string]bool{
		formulaPrefix + "hop": true,
		formulaPrefix + "wt":  true,
	}
	versions := map[string]string{
		"hop": "hop v0.1.0\n",
		"wt":  "wt v0.2.0\n",
	}
	installFakeRunner(t, versionFake(installed, versions))

	var stdout bytes.Buffer
	if err := runVersion(context.Background(), &stdout); err != nil {
		t.Fatalf("runVersion err = %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, notInstalledLabel) {
		t.Fatalf("expected %q somewhere in output, got:\n%s", notInstalledLabel, out)
	}
	// Idea is uninstalled — its row must say not installed.
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "idea") && !strings.Contains(line, notInstalledLabel) {
			t.Fatalf("idea row = %q, want %q", line, notInstalledLabel)
		}
	}
}

func TestVersion_LdflagsInjection(t *testing.T) {
	installFakeRunner(t, versionFake(nil, nil))
	prevVer := version
	version = "v1.2.3-test"
	t.Cleanup(func() { version = prevVer })

	var stdout bytes.Buffer
	if err := runVersion(context.Background(), &stdout); err != nil {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(stdout.String(), "v1.2.3-test") {
		t.Fatalf("output missing injected version, got:\n%s", stdout.String())
	}
}

func TestVersion_DefaultDev(t *testing.T) {
	installFakeRunner(t, versionFake(nil, nil))
	// Confirm `dev` is the default value of `version`. The package var starts
	// as "dev" — this test asserts that no init code clobbers it.
	if version != "dev" {
		t.Skipf("version was already overridden in this test run (= %q)", version)
	}
	var stdout bytes.Buffer
	if err := runVersion(context.Background(), &stdout); err != nil {
		t.Fatalf("err = %v", err)
	}
	first := strings.SplitN(stdout.String(), "\n", 2)[0]
	if !strings.Contains(first, "dev") {
		t.Fatalf("first row = %q, want to contain `dev`", first)
	}
}

func TestVersion_NoANSI(t *testing.T) {
	installed := map[string]bool{}
	versions := map[string]string{}
	for _, tool := range Roster {
		installed[installKey(tool)] = true
		versions[tool.Name] = tool.Name + " v0.1.0\n"
	}
	installFakeRunner(t, versionFake(installed, versions))
	var stdout bytes.Buffer
	if err := runVersion(context.Background(), &stdout); err != nil {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(stdout.String(), "\x1b[") {
		t.Fatalf("output contains ANSI escape, got:\n%s", stdout.String())
	}
}

func TestVersion_TimeoutHandling(t *testing.T) {
	// Simulate a hung tool by making the fake runner sleep past versionTimeout
	// when --version is invoked for hop. The captured-context (with timeout)
	// is honored by the fake — we manually return a deadline error.
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		if req.Name == brewBinary && len(req.Args) >= 4 && req.Args[0] == "list" {
			return proc.Result{} // installed
		}
		if req.Name == "hop" && len(req.Args) == 1 && req.Args[0] == "--version" {
			return proc.Result{Err: context.DeadlineExceeded}
		}
		if len(req.Args) == 1 && req.Args[0] == "--version" {
			return proc.Result{Stdout: []byte(req.Name + " v0.1.0\n")}
		}
		return proc.Result{}
	}}
	installFakeRunner(t, f)

	start := time.Now()
	var stdout bytes.Buffer
	if err := runVersion(context.Background(), &stdout); err != nil {
		t.Fatalf("err = %v", err)
	}
	elapsed := time.Since(start)
	// Sanity: even though we simulate the timeout error, the test itself must
	// finish quickly — this also verifies we did not actually wait for the
	// real timeout in the synthetic fake.
	if elapsed > versionTimeout {
		t.Fatalf("test elapsed = %s, expected fast (fake returned immediately)", elapsed)
	}
	for _, line := range strings.Split(stdout.String(), "\n") {
		if strings.HasPrefix(line, "hop") && !strings.Contains(line, notInstalledLabel) {
			t.Fatalf("hop row = %q, want %q for timeout", line, notInstalledLabel)
		}
	}
}

// --- root --version flag: toolkit `version` standard conformance (5ys1) -------

// TestRootVersionFlag_VersionStandardConformance pins the producer-side
// contract of `shll --version` against the toolkit `version` standard
// (docs/site/standards/version.md, "Verifying conformance"): exit 0, version
// written to stdout on the first non-empty line, matching the published shape
// — the RECOMMENDED canonical `<tool> version vX.Y.Z` — with nothing on
// stderr and no banner above the version. Assertions reuse the repo's own
// normalizeVersion (versionTokenRE/versionPrefixRE) so the test pins "shll
// parses its own output", not a hand-rolled regex.
//
// Clauses 3-4 of the standard (respond within 2 seconds, no network I/O on
// the version path) carry no in-process assertion: the path is a purely
// local read of the ldflags-injected package var via cobra's built-in
// version flag — there is no subprocess or I/O seam to fake. Those clauses
// are structural; this test protects them indirectly by pinning that the
// single-line template output comes straight from root.Version.
func TestRootVersionFlag_VersionStandardConformance(t *testing.T) {
	cases := []struct {
		name          string // subtest name
		version       string // value wired into root.Version (main.go seam)
		wantFirstLine string // exact first non-empty stdout line
		wantParsed    string // normalizeVersion round-trip of the output
	}{
		// Release builds inject the v-prefixed git tag verbatim.
		{"stamped", "v1.2.3", "shll version v1.2.3", "v1.2.3"},
		// Unstamped builds keep the `dev` default — the first line still
		// satisfies the `<word> version <rest>` prefix shape (versionPrefixRE),
		// so dev builds stay parseable, not just release builds.
		{"dev_default", "dev", "shll version dev", "dev"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := newRootCmd()
			root.Version = tc.version // mirrors main.go: rootCmd.Version = version
			var stdout, stderr bytes.Buffer
			root.SetOut(&stdout)
			root.SetErr(&stderr)
			root.SetArgs([]string{"--version"})

			// Clause: MUST support --version and exit 0. A nil Execute error
			// maps to exit 0 through translateExit (main.go).
			if err := root.Execute(); err != nil {
				t.Fatalf("Execute(--version) err = %v, want nil (exit 0)", err)
			}

			// Clause: version token on the first non-empty line, no banner /
			// copyright / update-check line above it, in the RECOMMENDED
			// canonical shape `<tool> version vX.Y.Z`.
			firstLine := ""
			for _, line := range strings.Split(stdout.String(), "\n") {
				if trimmed := strings.TrimSpace(line); trimmed != "" {
					firstLine = trimmed
					break
				}
			}
			if firstLine != tc.wantFirstLine {
				t.Errorf("first non-empty line = %q, want %q", firstLine, tc.wantFirstLine)
			}

			// Clause: matches the published shape — shll's own parse
			// (versionTokenRE / versionPrefixRE via normalizeVersion) extracts
			// the version from the output.
			if got := normalizeVersion(stdout.String()); got != tc.wantParsed {
				t.Errorf("normalizeVersion(output) = %q, want %q", got, tc.wantParsed)
			}

			// Clause: the version is written to stdout (stdout is data);
			// nothing lands on stderr.
			if stderr.Len() != 0 {
				t.Errorf("stderr = %q, want empty", stderr.String())
			}
		})
	}
}

// --- legacy-name PATH-probe fallback (rk→run-kit, change 9bak) ----------------

// runKitTool returns the live run-kit roster entry (which carries LegacyName "rk").
func runKitTool(t *testing.T) Tool {
	t.Helper()
	for _, tool := range Roster {
		if tool.Name == "run-kit" {
			return tool
		}
	}
	t.Fatal("run-kit not found in Roster")
	return Tool{}
}

func TestProbeToolVersion_LegacyNameFallbackOnErrNotFound(t *testing.T) {
	// The primary `run-kit --version` is ErrNotFound (binary not on PATH under the
	// new name), but the legacy `rk` binary IS present → the fallback finds it and
	// returns its output. Display name stays run-kit (the caller uses tool.Name).
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		if req.Name == "run-kit" && len(req.Args) == 1 && req.Args[0] == "--version" {
			return proc.Result{Err: proc.ErrNotFound}
		}
		if req.Name == "rk" && len(req.Args) == 1 && req.Args[0] == "--version" {
			return proc.Result{Stdout: []byte("rk v2.5.13\n")}
		}
		return proc.Result{}
	}}
	installFakeRunner(t, f)

	rk := runKitTool(t)
	out, err := probeToolVersion(context.Background(), rk)
	if err != nil {
		t.Fatalf("probeToolVersion err = %v, want nil (legacy fallback should succeed)", err)
	}
	if !strings.Contains(string(out), "2.5.13") {
		t.Fatalf("probe output = %q, want the legacy `rk` binary's version", string(out))
	}
	// toolInstalled sees it installed; toolVersion normalizes it — both with the
	// run-kit display identity.
	if !toolInstalled(context.Background(), rk) {
		t.Error("toolInstalled = false, want true via the legacy fallback")
	}
	if got := toolVersion(context.Background(), rk); got != "v2.5.13" {
		t.Errorf("toolVersion = %q, want v2.5.13", got)
	}
}

func TestProbeToolVersion_NoFallbackOnNonErrNotFound(t *testing.T) {
	// A present-but-broken run-kit (`--version` exits non-zero — NOT ErrNotFound)
	// must NOT silently defer to the legacy `rk` binary. The primary error is
	// returned; `rk` is never probed.
	rkProbed := false
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		if req.Name == "run-kit" && len(req.Args) == 1 && req.Args[0] == "--version" {
			return proc.Result{Err: errors.New("run-kit: boom")}
		}
		if req.Name == "rk" && len(req.Args) == 1 && req.Args[0] == "--version" {
			rkProbed = true
			return proc.Result{Stdout: []byte("rk v2.5.13\n")}
		}
		return proc.Result{}
	}}
	installFakeRunner(t, f)

	rk := runKitTool(t)
	if _, err := probeToolVersion(context.Background(), rk); err == nil {
		t.Fatal("probeToolVersion err = nil, want the primary non-ErrNotFound error (no fallback)")
	}
	if rkProbed {
		t.Fatal("the legacy `rk` binary must NOT be probed on a non-ErrNotFound primary error")
	}
	if toolInstalled(context.Background(), rk) {
		t.Error("toolInstalled = true, want false (present-but-broken run-kit is not installed via fallback)")
	}
}

// --- delegated (non-brew) probe seam (rk-desktop) ----------------------------

// rkDesktopTool returns the live rk-desktop roster entry (the delegated,
// formula-less tool carrying the `rk desktop status` Probe spec).
func rkDesktopTool(t *testing.T) Tool {
	t.Helper()
	tool, ok := rosterTool("rk-desktop")
	if !ok {
		t.Fatal("rk-desktop not found in Roster")
	}
	return tool
}

// rkDesktopFake builds a fakeRunner whose `rk desktop status` answers with the
// given stdout and exit code.
func rkDesktopFake(t *testing.T, tool Tool, statusOut string, statusCode int) *fakeRunner {
	t.Helper()
	return &fakeRunner{respond: func(req proc.Request) proc.Result {
		if req.Name == tool.Probe.Argv[0] && strings.Join(req.Args, " ") == strings.Join(tool.Probe.Argv[1:], " ") {
			return proc.Result{Stdout: []byte(statusOut), ExitCode: statusCode}
		}
		return proc.Result{}
	}}
}

func TestProbeToolVersion_DelegatedInstalled(t *testing.T) {
	tool := rkDesktopTool(t)
	installFakeRunner(t, rkDesktopFake(t, tool, "Run Kit Desktop\nInstalled: v1.2.3\nLatest: v1.2.3\n", 0))

	if !toolInstalled(context.Background(), tool) {
		t.Error("toolInstalled = false, want true (`Installed: v1.2.3`)")
	}
	if got := toolVersion(context.Background(), tool); got != "v1.2.3" {
		t.Errorf("toolVersion = %q, want v1.2.3 (parsed from the Installed: line)", got)
	}
}

func TestProbeToolVersion_DelegatedAbsent(t *testing.T) {
	tool := rkDesktopTool(t)
	installFakeRunner(t, rkDesktopFake(t, tool, "Installed: not installed\n", 0))

	if toolInstalled(context.Background(), tool) {
		t.Error("toolInstalled = true, want false (`Installed: not installed`)")
	}
	if got := toolVersion(context.Background(), tool); got != notInstalledLabel {
		t.Errorf("toolVersion = %q, want %q", got, notInstalledLabel)
	}
}

func TestProbeToolVersion_DelegatedRefusalIsNotInstalled(t *testing.T) {
	// An unsupported-platform refusal (non-zero exit + the errDesktopMacOnly
	// message) collapses to not-installed on the display surfaces — never a
	// crash, never a panic.
	tool := rkDesktopTool(t)
	installFakeRunner(t, rkDesktopFake(t, tool, "Error: rk desktop is macOS-only (the shell is packaged as a macOS .app)\n", 1))

	if toolInstalled(context.Background(), tool) {
		t.Error("toolInstalled = true, want false (platform refusal)")
	}
	if got := toolVersion(context.Background(), tool); got != notInstalledLabel {
		t.Errorf("toolVersion = %q, want %q", got, notInstalledLabel)
	}
}

func TestVersion_RkDesktopRow(t *testing.T) {
	// End-to-end through runVersion: the rk-desktop row reads its installed
	// version from the `rk desktop status` Installed: line, in roster position.
	tool := rkDesktopTool(t)
	installFakeRunner(t, &fakeRunner{respond: func(req proc.Request) proc.Result {
		if req.Name == tool.Probe.Argv[0] && strings.Join(req.Args, " ") == strings.Join(tool.Probe.Argv[1:], " ") {
			return proc.Result{Stdout: []byte("Installed: v3.1.4\n")}
		}
		return proc.Result{Err: proc.ErrNotFound}
	}})

	var stdout bytes.Buffer
	if err := runVersion(context.Background(), &stdout); err != nil {
		t.Fatalf("runVersion err = %v", err)
	}
	for _, line := range strings.Split(stdout.String(), "\n") {
		if strings.HasPrefix(line, "rk-desktop") && !strings.Contains(line, "v3.1.4") {
			t.Fatalf("rk-desktop row = %q, want to contain v3.1.4", line)
		}
	}
	if !strings.Contains(stdout.String(), "rk-desktop") {
		t.Fatalf("output missing the rk-desktop row, got:\n%s", stdout.String())
	}
}

// --- normalizeVersion unit tests ---------------------------------------------

func TestNormalizeVersion_NamePrefixedBare(t *testing.T) {
	got := normalizeVersion("fab-kit version 1.9.4\n")
	if got != "v1.9.4" {
		t.Fatalf("got %q, want %q", got, "v1.9.4")
	}
}

func TestNormalizeVersion_NamePrefixedV(t *testing.T) {
	got := normalizeVersion("hop version v0.1.5\n")
	if got != "v0.1.5" {
		t.Fatalf("got %q, want %q (no doubling)", got, "v0.1.5")
	}
}

func TestNormalizeVersion_Bare(t *testing.T) {
	got := normalizeVersion("0.4.10\n")
	if got != "v0.4.10" {
		t.Fatalf("got %q, want %q", got, "v0.4.10")
	}
}

func TestNormalizeVersion_BareDev(t *testing.T) {
	got := normalizeVersion("dev")
	if got != "dev" {
		t.Fatalf("got %q, want %q", got, "dev")
	}
}

func TestNormalizeVersion_NamePrefixedDev(t *testing.T) {
	got := normalizeVersion("shll version dev\n")
	if got != "dev" {
		t.Fatalf("got %q, want %q (prefix-strip)", got, "dev")
	}
}

func TestNormalizeVersion_Unparseable(t *testing.T) {
	got := normalizeVersion("some unparseable banner")
	if got != "some unparseable banner" {
		t.Fatalf("got %q, want raw passthrough", got)
	}
}

func TestNormalizeVersion_Empty(t *testing.T) {
	if got := normalizeVersion(""); got != "" {
		t.Fatalf("empty: got %q, want \"\"", got)
	}
	if got := normalizeVersion("\n\n  \n"); got != "" {
		t.Fatalf("whitespace-only: got %q, want \"\"", got)
	}
}

func TestNormalizeVersion_FirstLineOnly(t *testing.T) {
	got := normalizeVersion("MyTool — the swiss army knife\n0.4.10\n")
	if got != "MyTool — the swiss army knife" {
		t.Fatalf("got %q, want first line verbatim (line 2 must NOT be searched)", got)
	}
}

func TestNormalizeVersion_BlankLeadingLines(t *testing.T) {
	got := normalizeVersion("\n\nfab-kit version 1.9.4\n")
	if got != "v1.9.4" {
		t.Fatalf("got %q, want %q", got, "v1.9.4")
	}
}

func TestNormalizeVersion_PermissiveSemVer(t *testing.T) {
	if got := normalizeVersion("mytool version 1.2"); got != "v1.2" {
		t.Fatalf("2-component: got %q, want %q", got, "v1.2")
	}
	if got := normalizeVersion("mytool version 1.2.3-rc1+build.42"); got != "v1.2.3-rc1+build.42" {
		t.Fatalf("rich suffix: got %q, want %q", got, "v1.2.3-rc1+build.42")
	}
}

func TestNormalizeVersion_CaseInsensitiveVersionWord(t *testing.T) {
	// The version-token regex matches `1.0` first; the prefix-strip path is
	// not exercised here. This test confirms the version-token branch wins
	// when both could apply.
	got := normalizeVersion("MyTool Version 1.0")
	if got != "v1.0" {
		t.Fatalf("got %q, want %q", got, "v1.0")
	}
}

func TestNormalizeVersion_PrefixStripCase(t *testing.T) {
	// `dev` has no version-shaped token, so the prefix-strip fallback runs.
	// The literal word `Version` is capitalized — the regex MUST match it
	// case-insensitively.
	got := normalizeVersion("shll Version dev")
	if got != "dev" {
		t.Fatalf("got %q, want %q (case-insensitive prefix-strip)", got, "dev")
	}
}
