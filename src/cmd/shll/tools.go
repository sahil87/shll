package main

import (
	"fmt"
	"strings"
)

// formulaPrefix is the brew tap qualifier used for every roster formula. Named
// constant per code-quality.md (no magic strings).
const formulaPrefix = "sahil87/tap/"

// tapName is the Homebrew tap itself — the argument to `brew trust --tap`.
// Distinct from formulaPrefix (`sahil87/tap/`, with the trailing slash used to
// build *formula* references like `sahil87/tap/shll`): the trust ceremony acts
// on the tap, not a formula, so it must NOT carry the trailing slash. Named
// constant per code-quality.md (no magic strings).
const tapName = "sahil87/tap"

// Tool describes one entry in the hardcoded sahil87 toolkit roster. The list is
// the source of truth for `shll update`, `shll shell-init`, and `shll version`
// (Constitution III — Tool Roster Source of Truth). Adding a new tool requires
// a shll release; no runtime discovery.
type Tool struct {
	// Name is the binary name (also used as the brew formula leaf and as the
	// label printed by `shll version`).
	Name string
	// Formula is the fully-qualified Homebrew formula name passed to brew.
	Formula string
	// ShellInit is the argv of the tool's shell-init invocation, with `<shell>`
	// substituted at composition time. An empty slice means the tool has no
	// shell integration — it is skipped during `shll shell-init`.
	//
	// Use the literal token `<shell>` to indicate where the user-supplied shell
	// name (zsh, bash) should be substituted at composition time. Every current
	// integrator (`tu`, `hop`, `wt`) takes a shell argument; if a future tool
	// shipped a no-arg shell-init, its argv would simply omit the placeholder.
	ShellInit []string
	// Update is the argv of the tool's own update invocation (e.g. `{"rk",
	// "update"}`). `shll update` delegates to this rather than calling `brew
	// upgrade <formula>` directly, so each tool's post-upgrade side effects
	// (e.g. rk's daemon restart) are preserved (Constitution IV — Composition).
	// An empty slice means the tool exposes no `update` subcommand — `shll
	// update` falls back to `brew upgrade <formula>` for it. Every current
	// roster tool ships an `update`, so all entries populate this field.
	Update []string
	// Description is a one-line, human-readable summary of what the tool does,
	// printed by `shll list`. Single-sourced here so the roster cannot drift
	// from the managed set (Constitution III — Tool Roster Source of Truth).
	Description string
	// Repo is the github.com/sahil87/<Repo> slug for the tool's source
	// repository. It defaults to Name for most tools, but is NOT always equal
	// to Name: rk's repository is `run-kit` (github.com/sahil87/rk is a 404).
	// Stored explicitly so `shll list` never emits a dead link.
	Repo string
}

// githubOrgBase is the GitHub organization base URL for the sahil87 toolkit.
// A tool's source-repo URL is githubOrgBase + tool.Repo. Named constant per
// code-quality.md (no magic strings) so `shll list` never open-codes the URL.
const githubOrgBase = "https://github.com/sahil87/"

// shellPlaceholder is the literal substituted with the requested shell at
// composition time inside ShellInit argv. Named constant so callers do not
// open-code the string.
const shellPlaceholder = "<shell>"

// Roster is the hardcoded sahil87 toolkit list. Order matters and is declared
// leaves-first: every tool appears after all of its dependencies, so dependents
// are processed only once their dependencies are done.
//
// The dependency edges driving this order are:
//   - fab-kit -> wt, fab-kit -> idea  (fab-kit's brew formula upgrades wt/idea)
//   - hop -> wt                       (hop's brew formula upgrades wt; hop also
//     invokes wt at runtime)
//   - rk -> wt                        (rk invokes wt at runtime)
//
// so the leaves wt, idea, tu (no outgoing edges) precede the dependents rk,
// hop, fab-kit. This is OUTPUT COHERENCE, not a correctness fix: brew already
// resolves formula dependencies correctly and idempotently, and each
// `<tool> update` is self-update-only, so the order can neither break nor
// improve upgrade correctness. What it buys is that each tool's per-tool output
// section in `shll update` / `shll install` completes (and is counted) before a
// dependent's internal `brew upgrade` can re-touch a leaf already reported done
// under its own header. `shll shell-init` likewise concatenates output in this
// order, so users can reason about init sequencing.
//
// The full ordering contract (brew-upgrade AND runtime edges) is enforced by
// TestRosterLeavesBeforeDependents — a comment cannot fail CI, so the test
// guards against an accidental reorder.
var Roster = []Tool{
	{Name: "wt", Formula: formulaPrefix + "wt", ShellInit: []string{"wt", "shell-init", shellPlaceholder}, Update: []string{"wt", "update"}, Repo: "wt", Description: "Git worktree management — create, list, open, delete worktrees"},
	{Name: "idea", Formula: formulaPrefix + "idea", Update: []string{"idea", "update"}, Repo: "idea", Description: "Backlog idea management from the terminal"},
	{Name: "tu", Formula: formulaPrefix + "tu", ShellInit: []string{"tu", "shell-init", shellPlaceholder}, Update: []string{"tu", "update"}, Repo: "tu", Description: "Token-usage tracker for AI coding tools (Claude Code, Codex, OpenCode)"},
	{Name: "rk", Formula: formulaPrefix + "rk", Update: []string{"rk", "update"}, Repo: "run-kit", Description: "Run-kit — tmux session manager with a web UI"},
	{Name: "hop", Formula: formulaPrefix + "hop", ShellInit: []string{"hop", "shell-init", shellPlaceholder}, Update: []string{"hop", "update"}, Repo: "hop", Description: "Fast directory/project jumping across worktrees"},
	{Name: "fab-kit", Formula: formulaPrefix + "fab-kit", Update: []string{"fab-kit", "update"}, Repo: "fab-kit", Description: "Spec-driven workspace & workflow toolkit (the `fab` CLI)"},
}

// shllTargetToken is the literal positional argument that selects shll itself as
// an upgrade target for `shll update <tool...>` (e.g. `shll update shll`). shll is
// intentionally NOT in Roster (Roster is the sub-tool list per Constitution III),
// so the self-target name is a named constant rather than a Tool.Name. It is a
// valid target for `update` only — never for `install` (you cannot brew-install
// the running orchestrator). Named per code-quality.md (no magic strings).
const shllTargetToken = "shll"

// resolveTargets maps the positional tool-name args of `shll update`/`shll install`
// to the work set, single-sourced with Roster so the valid-name list cannot drift
// between the two commands. It performs NAME validation only — it does not consult
// brew (install-status is layered on by the caller after probing, where brew facts
// already exist), so it makes no subprocess calls and is trivially unit-testable.
//
// Valid targets are the Roster names, plus shllTargetToken when allowShll is true
// (`update` passes true; `install` passes false — shll is not installable). The args
// form a SET, not a sequence: selected Tools are returned in Roster (leaves-first)
// order regardless of the order they were supplied, and selfSelected reports whether
// shll itself was named (the caller processes it first, before the roster loop).
//
// On ANY unknown arg, it returns a non-nil error naming ALL unknown args (a better
// one-shot fix than reporting only the first) and listing the valid targets; the
// caller writes it to stderr and exits non-zero with no side effects. A zero-length
// args slice yields an empty selection and selfSelected=false (the caller keeps its
// whole-roster path for that case).
func resolveTargets(args []string, allowShll bool) (selected []Tool, selfSelected bool, err error) {
	// Validate every arg up front; collect unknowns so all are reported at once.
	var unknown []string
	wanted := make(map[string]bool, len(args))
	for _, a := range args {
		if allowShll && a == shllTargetToken {
			selfSelected = true
			continue
		}
		if rosterHas(a) {
			wanted[a] = true
			continue
		}
		unknown = append(unknown, a)
	}
	if len(unknown) > 0 {
		return nil, false, fmt.Errorf("unknown target%s %s (valid targets: %s)",
			plural(len(unknown)), quoteJoin(unknown), validTargets(allowShll))
	}

	// Return the selected Tools in Roster order (not arg order) so the subset is
	// always processed leaves-first, matching the whole-roster contract.
	for _, t := range Roster {
		if wanted[t.Name] {
			selected = append(selected, t)
		}
	}
	return selected, selfSelected, nil
}

// rosterHas reports whether name is a Roster tool name. Source of truth is the live
// Roster, so the valid-name list never drifts from the roster itself.
func rosterHas(name string) bool {
	for _, t := range Roster {
		if t.Name == name {
			return true
		}
	}
	return false
}

// validTargets returns the comma-separated list of valid target names for an error
// message — the Roster names, prefixed with shll when allowShll. Derived from the
// live Roster so it stays in sync.
func validTargets(allowShll bool) string {
	names := make([]string, 0, len(Roster)+1)
	if allowShll {
		names = append(names, shllTargetToken)
	}
	for _, t := range Roster {
		names = append(names, t.Name)
	}
	return strings.Join(names, ", ")
}

// quoteJoin renders a list of names as quoted, comma-separated tokens (e.g.
// `"foo", "bar"`) for the unknown-target diagnostic.
func quoteJoin(names []string) string {
	quoted := make([]string, len(names))
	for i, n := range names {
		quoted[i] = fmt.Sprintf("%q", n)
	}
	return strings.Join(quoted, ", ")
}

// plural returns "s" when n != 1, for grammatical agreement in diagnostics.
func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
