package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestRosterOrder pins the exact roster contract: importance-descending with
// dependency adjacency — run-kit first, rk-desktop immediately after it (its
// `rk desktop …` delegations need the run-kit binary), then the rest in the
// order the change resolved. Every roster-driven surface (install/update/
// uninstall walk order, list/doctor/version row order, shell-init composition)
// inherits this order from the single slice, so the exact sequence IS the
// contract — a comment cannot fail CI, so this test guards against an
// accidental reorder.
func TestRosterOrder(t *testing.T) {
	want := []string{"run-kit", "rk-desktop", "fab-kit", "wt", "idea", "tu", "hop"}
	if len(Roster) != len(want) {
		t.Fatalf("len(Roster) = %d, want %d", len(Roster), len(want))
	}
	for i, name := range want {
		if Roster[i].Name != name {
			t.Errorf("Roster[%d].Name = %q, want %q", i, Roster[i].Name, name)
		}
	}
}

// TestRosterBrewManagedShape pins the brew-vs-delegated split: every roster
// entry except rk-desktop is brew-managed (Formula set, no Install argv, no
// Probe), and the invariants the brew helpers rely on hold (a brew-managed tool
// always has a tap-qualified formula).
func TestRosterBrewManagedShape(t *testing.T) {
	for _, tool := range Roster {
		if !tool.brewManaged() {
			continue // the delegated seam is pinned by TestRkDesktopEntry
		}
		if !strings.HasPrefix(tool.Formula, formulaPrefix) {
			t.Errorf("%s: Formula = %q, want tap-qualified (%s<name>)", tool.Name, tool.Formula, formulaPrefix)
		}
		if len(tool.Install) != 0 {
			t.Errorf("%s: brew-managed tool must not carry a delegated Install argv", tool.Name)
		}
		if tool.Probe != nil {
			t.Errorf("%s: brew-managed tool must not carry a Probe", tool.Name)
		}
	}
}

// TestRkDesktopEntry pins the roster's first delegated (non-brew) entry: no
// Formula (no brew helper may touch it), install/update delegating to
// `rk desktop install`/`rk desktop update`, the installed-probe parsing the
// `Installed:` line of `rk desktop status`, Repo pointing at run-kit (it ships
// with run-kit — no repo of its own), no shell-init, and a SkillHint (required
// of every entry by TestRosterSkillHints).
func TestRkDesktopEntry(t *testing.T) {
	tool, ok := rosterTool("rk-desktop")
	if !ok {
		t.Fatal("rk-desktop missing from Roster")
	}
	if tool.brewManaged() {
		t.Errorf("rk-desktop Formula = %q, want empty (delegated, non-brew)", tool.Formula)
	}
	if got := strings.Join(tool.Install, " "); got != "rk desktop install" {
		t.Errorf("rk-desktop Install = %q, want %q", got, "rk desktop install")
	}
	if got := strings.Join(tool.Update, " "); got != "rk desktop update" {
		t.Errorf("rk-desktop Update = %q, want %q", got, "rk desktop update")
	}
	if tool.Probe == nil {
		t.Fatal("rk-desktop Probe = nil, want the `rk desktop status` probe spec")
	}
	if got := strings.Join(tool.Probe.Argv, " "); got != "rk desktop status" {
		t.Errorf("rk-desktop Probe.Argv = %q, want %q", got, "rk desktop status")
	}
	if tool.Probe.LinePrefix != "Installed:" || tool.Probe.AbsentValue != "not installed" {
		t.Errorf("rk-desktop Probe = %+v, want LinePrefix %q / AbsentValue %q", tool.Probe, "Installed:", "not installed")
	}
	if tool.Repo != "run-kit" {
		t.Errorf("rk-desktop Repo = %q, want %q (it ships with run-kit)", tool.Repo, "run-kit")
	}
	if len(tool.ShellInit) != 0 {
		t.Errorf("rk-desktop must not carry ShellInit (no shell integration)")
	}
	if tool.SkillHint == "" {
		t.Error("rk-desktop SkillHint must be non-empty (TestRosterSkillHints contract)")
	}
}

// TestIsRkDesktopRefusal pins the unsupported-platform refusal matcher: it keys
// on run-kit's errDesktopMacOnly message substring — never a hardcoded darwin
// check — so a platform refusal is distinguishable from a real failure.
func TestIsRkDesktopRefusal(t *testing.T) {
	if !isRkDesktopRefusal([]byte("Error: rk desktop is macOS-only (the shell is packaged as a macOS .app)\n")) {
		t.Error("refusal message not detected")
	}
	if isRkDesktopRefusal([]byte("Installed: v1.2.3\n")) {
		t.Error("ordinary status output misdetected as a refusal")
	}
	if isRkDesktopRefusal(nil) {
		t.Error("empty output misdetected as a refusal")
	}
}

// --- shllSelf shared descriptor (change bb7r) ---

func TestShllSelf_Descriptor(t *testing.T) {
	// The shared shll-self descriptor's field contract — single-sourced and reused
	// by list/doctor/install.
	if shllSelf.Name != shllTargetToken {
		t.Errorf("shllSelf.Name = %q, want %q", shllSelf.Name, shllTargetToken)
	}
	if shllSelf.Description != shllSelfDescription {
		t.Errorf("shllSelf.Description = %q, want %q", shllSelf.Description, shllSelfDescription)
	}
	if shllSelf.Repo != shllTargetToken {
		t.Errorf("shllSelf.Repo = %q, want %q (→ github.com/sahil87/shll)", shllSelf.Repo, shllTargetToken)
	}
	// shll has no managed Formula/ShellInit/Update — it is not a Roster tool.
	if shllSelf.Formula != "" || len(shllSelf.ShellInit) != 0 || len(shllSelf.Update) != 0 {
		t.Errorf("shllSelf must not carry Formula/ShellInit/Update, got %+v", shllSelf)
	}
}

func TestShllSelf_NotInRoster(t *testing.T) {
	// The descriptor must NOT have leaked into Roster (Constitution III +
	// roster-order invariant). Roster stays exactly the 7 managed sub-tools
	// (6 brew-managed + the delegated rk-desktop).
	if len(Roster) != 7 {
		t.Errorf("len(Roster) = %d, want 7 (managed sub-tools only)", len(Roster))
	}
	if rosterHas(shllTargetToken) {
		t.Error("shll must NOT be a Roster entry")
	}
}

func TestShllSelfVersion_FromPackageVar(t *testing.T) {
	// shllSelfVersion reads the package version var (normalized), never a
	// subprocess. Swap the var to confirm the source.
	prev := version
	t.Cleanup(func() { version = prev })
	version = "9.9.9"
	if got := shllSelfVersion(); got != "v9.9.9" {
		t.Errorf("shllSelfVersion() = %q, want %q (normalized package var)", got, "v9.9.9")
	}
}

// --- resolveTargets (shared subset resolver, change b2vg) ---

func toolNames(tools []Tool) []string {
	names := make([]string, len(tools))
	for i, t := range tools {
		names[i] = t.Name
	}
	return names
}

func TestResolveTargets_RosterOrderRegardlessOfArgOrder(t *testing.T) {
	// Args in reverse roster order must still resolve to roster order: fab-kit
	// precedes wt in the new importance-descending roster.
	selected, self, aliased, err := resolveTargets([]string{"wt", "fab-kit"}, true)
	if err != nil {
		t.Fatalf("resolveTargets err = %v, want nil", err)
	}
	if self {
		t.Error("selfSelected should be false when shll is not named")
	}
	if len(aliased) != 0 {
		t.Errorf("aliased = %v, want empty (no legacy alias named)", aliased)
	}
	got := toolNames(selected)
	want := []string{"fab-kit", "wt"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("selected = %v, want %v (roster order, not arg order)", got, want)
	}
}

func TestResolveTargets_ShllGatedByAllowShll(t *testing.T) {
	// allowShll=true: `shll` is accepted and sets selfSelected, returns no roster
	// Tools when it is the only arg.
	selected, self, _, err := resolveTargets([]string{shllTargetToken}, true)
	if err != nil {
		t.Fatalf("resolveTargets(allowShll=true) err = %v, want nil", err)
	}
	if !self {
		t.Error("selfSelected should be true when shll is named with allowShll=true")
	}
	if len(selected) != 0 {
		t.Errorf("selected = %v, want empty (shll is not a roster Tool)", toolNames(selected))
	}

	// allowShll=false: `shll` is an unknown target → error, and the error must NOT
	// advertise shll as valid.
	_, _, _, err = resolveTargets([]string{shllTargetToken}, false)
	if err == nil {
		t.Fatal("resolveTargets(allowShll=false) with `shll` should error")
	}
	if !strings.Contains(err.Error(), `"shll"`) {
		t.Errorf("err = %v, want to name `shll` as the unknown target", err)
	}
	if strings.Contains(err.Error(), "valid targets: shll") {
		t.Errorf("err = %v, install valid-target list must NOT include shll", err)
	}
}

func TestResolveTargets_MultipleUnknownAllReported(t *testing.T) {
	_, _, _, err := resolveTargets([]string{"foo", "wt", "bar"}, true)
	if err == nil {
		t.Fatal("resolveTargets with unknown args should error")
	}
	if !strings.Contains(err.Error(), `"foo"`) || !strings.Contains(err.Error(), `"bar"`) {
		t.Fatalf("err = %v, want to name BOTH unknown args foo and bar", err)
	}
	// The valid-target list is present (shll + roster, since allowShll=true).
	if !strings.Contains(err.Error(), "valid targets:") {
		t.Errorf("err = %v, want to list valid targets", err)
	}
}

func TestResolveTargets_EmptyArgs(t *testing.T) {
	selected, self, aliased, err := resolveTargets(nil, true)
	if err != nil {
		t.Fatalf("resolveTargets(nil) err = %v, want nil", err)
	}
	if self {
		t.Error("selfSelected should be false for empty args")
	}
	if len(selected) != 0 {
		t.Errorf("selected = %v, want empty for empty args", toolNames(selected))
	}
	if len(aliased) != 0 {
		t.Errorf("aliased = %v, want empty for empty args", aliased)
	}
}

// --- legacy alias rk → run-kit (change 9bak) ---

func TestResolveTargets_LegacyAliasResolvesToCanonical(t *testing.T) {
	// `rk` resolves to the canonical run-kit tool (for both update and install) and
	// is signalled via the aliased slice so the caller can print the rename notice.
	for _, allowShll := range []bool{true, false} {
		selected, _, aliased, err := resolveTargets([]string{"rk"}, allowShll)
		if err != nil {
			t.Fatalf("resolveTargets([rk], allowShll=%v) err = %v, want nil", allowShll, err)
		}
		got := toolNames(selected)
		if len(got) != 1 || got[0] != "run-kit" {
			t.Fatalf("resolveTargets([rk]) selected = %v, want [run-kit]", got)
		}
		if len(aliased) != 1 || aliased[0] != "rk" {
			t.Fatalf("resolveTargets([rk]) aliased = %v, want [rk]", aliased)
		}
	}
}

func TestResolveTargets_RepeatedAliasRecordedOnce(t *testing.T) {
	// Args form a SET: a repeated alias token (`rk rk`) resolves to a single
	// canonical selection AND is recorded once in aliased, so the caller prints one
	// rename notice (the once-per-run notice contract) — not one per occurrence.
	selected, _, aliased, err := resolveTargets([]string{"rk", "rk"}, true)
	if err != nil {
		t.Fatalf("resolveTargets([rk rk]) err = %v, want nil", err)
	}
	got := toolNames(selected)
	if len(got) != 1 || got[0] != "run-kit" {
		t.Fatalf("resolveTargets([rk rk]) selected = %v, want [run-kit]", got)
	}
	if len(aliased) != 1 || aliased[0] != "rk" {
		t.Fatalf("resolveTargets([rk rk]) aliased = %v, want [rk] (recorded once)", aliased)
	}
}

func TestResolveTargets_ValidTargetsListsCanonicalOnly(t *testing.T) {
	// The unknown-target diagnostic lists the canonical `run-kit`, never the legacy
	// alias `rk`.
	_, _, _, err := resolveTargets([]string{"nope"}, true)
	if err == nil {
		t.Fatal("resolveTargets with unknown arg should error")
	}
	if !strings.Contains(err.Error(), "run-kit") {
		t.Errorf("err = %v, want valid-targets to include canonical run-kit", err)
	}
	// The bare legacy token `rk` must NOT appear as a valid target (it appears only
	// inside `run-kit`). Guard against a bare `, rk,` / `: rk,` listing.
	if strings.Contains(err.Error(), " rk,") || strings.Contains(err.Error(), " rk)") {
		t.Errorf("err = %v, valid-targets must not advertise the legacy alias rk", err)
	}
}

func TestPrintAliasNotices(t *testing.T) {
	var buf bytes.Buffer
	printAliasNotices(&buf, []string{"rk"})
	if got, want := buf.String(), "note: rk is now run-kit\n"; got != want {
		t.Errorf("printAliasNotices = %q, want %q", got, want)
	}
	// Empty slice prints nothing.
	buf.Reset()
	printAliasNotices(&buf, nil)
	if buf.Len() != 0 {
		t.Errorf("printAliasNotices(nil) wrote %q, want empty", buf.String())
	}

	// Defensive: a repeated token is announced once, and a token absent from
	// legacyAliases is skipped (never a malformed `note: X is now ` line).
	buf.Reset()
	printAliasNotices(&buf, []string{"rk", "rk", "bogus"})
	if got, want := buf.String(), "note: rk is now run-kit\n"; got != want {
		t.Errorf("printAliasNotices([rk rk bogus]) = %q, want %q", got, want)
	}
}
