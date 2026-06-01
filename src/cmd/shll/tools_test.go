package main

import "testing"

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
