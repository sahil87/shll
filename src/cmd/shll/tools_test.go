package main

import (
	"strings"
	"testing"
)

// rosterEdge is one dependency edge in the sahil87 toolkit, oriented
// dependent -> dep. The leaves-first Roster invariant is that every dependent
// appears AFTER all of its deps (a strictly greater index).
type rosterEdge struct {
	dependent string
	dep       string
}

// TestRosterLeavesBeforeDependents guards the toolkit's FULL ordering contract:
// every dependent in Roster appears after all of its dependencies. The encoded
// graph below is a SUPERSET of what output-coherence strictly needs — coherence
// only depends on the brew-upgrade edges (a dependent's internal `brew upgrade`
// re-touching a leaf already reported done). The runtime-invocation edges are
// encoded too so the contract documents how the tools actually relate.
//
// IMPORTANT for the reader: a runtime-invocation edge (e.g. rk -> wt, because
// `rk riff` shells out to `wt create`) does NOT mean `rk update` touches `wt`
// during `shll update` — each `<tool> update` is self-update-only. Runtime edges
// matter for install/runtime ordering, not for any update-time upgrade cascade.
func TestRosterLeavesBeforeDependents(t *testing.T) {
	edges := []rosterEdge{
		{dependent: "fab-kit", dep: "wt"},   // brew-upgrade dep
		{dependent: "fab-kit", dep: "idea"}, // brew-upgrade dep
		// hop -> wt is BOTH a brew-upgrade dep AND a runtime-invocation dep
		// (`hop open` delegates to wt's menu; `hop ls --trees` fans out
		// `wt list --json`).
		{dependent: "hop", dep: "wt"},
		// rk -> wt is a runtime-invocation dep (`rk riff` shells out to
		// `wt create`) — NOT an `rk update`-time upgrade of wt.
		{dependent: "rk", dep: "wt"},
	}

	// Build name -> roster index from the live Roster (no re-listing of tool
	// names — the test derives its map from the source of truth).
	indexByName := make(map[string]int, len(Roster))
	for i, tool := range Roster {
		indexByName[tool.Name] = i
	}

	for _, e := range edges {
		depIdx, ok := indexByName[e.dep]
		if !ok {
			t.Fatalf("edge %s -> %s: dep %q not found in Roster", e.dependent, e.dep, e.dep)
		}
		dependentIdx, ok := indexByName[e.dependent]
		if !ok {
			t.Fatalf("edge %s -> %s: dependent %q not found in Roster", e.dependent, e.dep, e.dependent)
		}
		if dependentIdx <= depIdx {
			t.Errorf("%s (index %d) must come after %s (index %d)", e.dependent, dependentIdx, e.dep, depIdx)
		}
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
	// Args in reverse roster order must still resolve to roster (leaves-first)
	// order: fab-kit, wt → wt, fab-kit.
	selected, self, err := resolveTargets([]string{"fab-kit", "wt"}, true)
	if err != nil {
		t.Fatalf("resolveTargets err = %v, want nil", err)
	}
	if self {
		t.Error("selfSelected should be false when shll is not named")
	}
	got := toolNames(selected)
	want := []string{"wt", "fab-kit"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("selected = %v, want %v (roster order, not arg order)", got, want)
	}
}

func TestResolveTargets_ShllGatedByAllowShll(t *testing.T) {
	// allowShll=true: `shll` is accepted and sets selfSelected, returns no roster
	// Tools when it is the only arg.
	selected, self, err := resolveTargets([]string{shllTargetToken}, true)
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
	_, _, err = resolveTargets([]string{shllTargetToken}, false)
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
	_, _, err := resolveTargets([]string{"foo", "wt", "bar"}, true)
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
	selected, self, err := resolveTargets(nil, true)
	if err != nil {
		t.Fatalf("resolveTargets(nil) err = %v, want nil", err)
	}
	if self {
		t.Error("selfSelected should be false for empty args")
	}
	if len(selected) != 0 {
		t.Errorf("selected = %v, want empty for empty args", toolNames(selected))
	}
}
