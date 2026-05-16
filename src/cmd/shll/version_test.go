package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/sahil87/shll/internal/proc"
)

// versionFake constructs a fakeRunner that simulates per-tool installation and
// version output. For a tool whose formula is absent from installedFormulas,
// the fake returns proc.ErrNotFound from `<tool> --version`, mirroring real
// exec.LookPath behavior when the binary is missing from PATH.
func versionFake(installedFormulas map[string]bool, versions map[string]string) *fakeRunner {
	formulaByName := map[string]string{}
	for _, t := range Roster {
		formulaByName[t.Name] = t.Formula
	}
	return &fakeRunner{respond: func(req proc.Request) proc.Result {
		// Simulate ErrNotFound for tools whose formula isn't installed.
		if formula, ok := formulaByName[req.Name]; ok && !installedFormulas[formula] {
			return proc.Result{Err: proc.ErrNotFound}
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
		installed[tool.Formula] = true
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
		installed[tool.Formula] = true
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
