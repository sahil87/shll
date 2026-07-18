package main

import (
	"fmt"
	"io"
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
	// SkillHint is a short task-domain phrase ("git worktrees", "backlog ideas")
	// woven into the generated `shll agent-setup` skill description, so an agent
	// matching on task vocabulary — not just tool names — triggers the toolkit
	// skill. Kept on the roster (not hand-written in agent_setup.go) so the skill
	// description cannot drift from the managed set (Constitution III). Required
	// for every roster entry — enforced by TestRosterSkillHints.
	SkillHint string
	// Repo is the github.com/sahil87/<Repo> slug for the tool's source
	// repository. It defaults to Name for most tools. Historically it was NOT
	// always equal to Name (rk's repo was `run-kit`); after the rk→run-kit rename
	// Name and Repo match for run-kit, so today every roster entry's Repo equals
	// its Name. The field stays explicit so `shll list` never emits a dead link
	// if a future tool's binary name and repo slug diverge again.
	Repo string
	// LegacyName is the tool's PRIOR binary name, used ONLY as a display-surface
	// PATH-probe fallback: when `<Name> --version` returns proc.ErrNotFound,
	// probeToolVersion retries `<LegacyName> --version` so a pre-rename install
	// (whose binary is still the old name on PATH) is shown as installed by
	// list/version/doctor. Empty for every tool except run-kit ("rk"). This is a
	// transitional field for the rk→run-kit rename — once legacy `rk`-only installs
	// die out it becomes a no-op fallback and can be retired.
	LegacyName string
	// LegacyFormula is the tool's PRIOR fully-qualified brew formula, used ONLY by
	// the update/install migration guard: when the current Formula is not installed
	// but LegacyFormula's keg is present (classified by keg leaf name), the tool is
	// a pre-rename install to be migrated brew-direct. Empty for every tool except
	// run-kit ("sahil87/tap/rk"). Transitional — see the rk→run-kit migration guard
	// in update.go; retire once legacy kegs die out.
	LegacyFormula string
}

// githubOrgBase is the GitHub organization base URL for the sahil87 toolkit.
// A tool's source-repo URL is githubOrgBase + tool.Repo. Named constant per
// code-quality.md (no magic strings) so `shll list` never open-codes the URL.
const githubOrgBase = "https://github.com/sahil87/"

// shellPlaceholder is the literal substituted with the requested shell at
// composition time inside ShellInit argv. Named constant so callers do not
// open-code the string.
const shellPlaceholder = "<shell>"

// legacyAliases maps a retired tool token to its current canonical Roster name.
// It is consulted by resolveTargets (before rosterHas) so `shll update rk` /
// `shll install rk` keep working after the rk→run-kit rename — muscle memory and
// existing scripts resolve to the canonical tool, and the caller prints a one-line
// notice. Aliases NEVER appear in the valid-targets error diagnostic (that lists
// canonical names only). Transitional for the rk→run-kit rename; retire when the
// alias is no longer worth carrying.
var legacyAliases = map[string]string{"rk": "run-kit"}

// aliasNoticeFmt is the one-line notice printed (to stdout) when the user named a
// legacy alias token — e.g. `note: rk is now run-kit`. Takes (alias, canonical).
// Named per code-quality.md (no magic strings).
const aliasNoticeFmt = "note: %s is now %s"

// printAliasNotices writes one aliasNoticeFmt line per legacy alias the user
// passed (in the order resolveTargets recorded them). Shared by `shll update` and
// `shll install` so the notice wording is single-sourced. A nil/empty slice prints
// nothing. IO is the caller's concern (resolveTargets stays IO-free).
//
// resolveTargets already de-duplicates aliased at its source (set semantics) and
// only ever records genuine legacyAliases keys, but this printer is defensive on
// both counts so it stays correct for any caller: it skips a token absent from
// legacyAliases (which would otherwise print a malformed `note: X is now ` line
// with an empty canonical), and de-dupes so a repeated token is announced once.
func printAliasNotices(stdout io.Writer, aliased []string) {
	seen := make(map[string]bool, len(aliased))
	for _, a := range aliased {
		canonical, ok := legacyAliases[a]
		if !ok || seen[a] {
			continue
		}
		seen[a] = true
		fmt.Fprintf(stdout, aliasNoticeFmt+"\n", a, canonical)
	}
}

// Roster is the hardcoded sahil87 toolkit list. Order matters and is declared
// leaves-first: every tool appears after all of its dependencies, so dependents
// are processed only once their dependencies are done.
//
// The dependency edges driving this order are:
//   - fab-kit -> wt, fab-kit -> idea  (fab-kit's brew formula upgrades wt/idea)
//   - hop -> wt                       (hop's brew formula upgrades wt; hop also
//     invokes wt at runtime)
//   - run-kit -> wt                   (run-kit invokes wt at runtime)
//
// so the leaves wt, idea, tu (no outgoing edges) precede the dependents run-kit,
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
	{Name: "wt", Formula: formulaPrefix + "wt", ShellInit: []string{"wt", "shell-init", shellPlaceholder}, Update: []string{"wt", "update"}, Repo: "wt", Description: "Git worktree management — create, list, open, delete worktrees", SkillHint: "git worktrees"},
	{Name: "idea", Formula: formulaPrefix + "idea", Update: []string{"idea", "update"}, Repo: "idea", Description: "Backlog idea management from the terminal", SkillHint: "backlog ideas"},
	{Name: "tu", Formula: formulaPrefix + "tu", ShellInit: []string{"tu", "shell-init", shellPlaceholder}, Update: []string{"tu", "update"}, Repo: "tu", Description: "Token-usage tracker for AI coding tools (Claude Code, Codex, OpenCode)", SkillHint: "AI token-usage tracking"},
	{Name: "run-kit", Formula: formulaPrefix + "run-kit", Update: []string{"run-kit", "update"}, Repo: "run-kit", LegacyName: "rk", LegacyFormula: formulaPrefix + "rk", Description: "Run-kit — tmux session manager with a web UI (rk stays as an alias)", SkillHint: "tmux sessions"},
	{Name: "hop", Formula: formulaPrefix + "hop", ShellInit: []string{"hop", "shell-init", shellPlaceholder}, Update: []string{"hop", "update"}, Repo: "hop", Description: "Fast directory/project jumping across worktrees", SkillHint: "directory/project jumping"},
	{Name: "fab-kit", Formula: formulaPrefix + "fab-kit", Update: []string{"fab-kit", "update"}, Repo: "fab-kit", Description: "Spec-driven workspace & workflow toolkit (the `fab` CLI)", SkillHint: "spec-driven workflows"},
}

// shllTargetToken is the literal positional argument that selects shll itself as
// an upgrade target for `shll update <tool...>` (e.g. `shll update shll`). shll is
// intentionally NOT in Roster (Roster is the sub-tool list per Constitution III),
// so the self-target name is a named constant rather than a Tool.Name. It is a
// valid target for `update` only — never for `install` (you cannot brew-install
// the running orchestrator). Named per code-quality.md (no magic strings).
const shllTargetToken = "shll"

// shllSelfDescription is the manager-framing one-liner printed for the shll-self
// entry by every command that shows the toolkit. Named per code-quality.md (no
// magic strings).
const shllSelfDescription = "the manager for the shll toolkit"

// shllSelf is the single shared descriptor representing shll ITSELF as a
// displayable entry — the one source of truth reused by every command that shows
// the toolkit as a family (`list`, `doctor`, `install`; `version`/`update`
// already lead with shll via their own self-handling). Each such command PREPENDS
// this descriptor, rendering shll FIRST, then the leaves-first Roster
// (`shll, wt, idea, tu, rk, hop, fab-kit`).
//
// It reuses the Tool struct shape but is deliberately NOT a Roster entry: Roster
// is the *managed sub-tool* list (Constitution III — Tool Roster Source of Truth),
// and adding shll there would break the leaves-first invariant guarded by
// TestRosterLeavesBeforeDependents and make install/update/shell-init try to
// operate on the running orchestrator itself (e.g. brew-install the live binary).
// Only Name, Description, and Repo are populated: shll has no managed Formula, no
// own ShellInit to compose (shell-init is the documented exception — its stdout is
// eval'd, Constitution V), and is self-upgraded via update.go's own path rather
// than a Roster Update argv.
//
// shll's version is NOT read from this descriptor; it comes from shllSelfVersion()
// (the package-level version var), never a `shll --version` self-subprocess.
var shllSelf = Tool{
	Name:        shllTargetToken,
	Description: shllSelfDescription,
	Repo:        shllTargetToken,
}

// shllSelfVersion returns shll's own normalized version for the shll-self entry.
// It reads the package-level version var (set via -ldflags at build time) rather
// than spawning `shll --version` on the running binary: shll is always present (it
// is the running process), so a self-subprocess would be wasteful and circular.
// normalizeVersion gives the same display shape `shll version` uses for its first
// row, so the two surfaces agree.
func shllSelfVersion() string {
	return normalizeVersion(version)
}

// resolveTargets maps the positional tool-name args of `shll update`/`shll install`
// to the work set, single-sourced with Roster so the valid-name list cannot drift
// between the two commands. It performs NAME validation only — it does not consult
// brew (install-status is layered on by the caller after probing, where brew facts
// already exist), so it makes no subprocess calls and is trivially unit-testable.
//
// Valid targets are the Roster names, plus shllTargetToken when allowShll is true
// (`update` passes true; `install` passes false — shll is not installable), plus the
// legacyAliases keys (e.g. `rk` → `run-kit`), which resolve to their canonical
// Roster tool. The args form a SET, not a sequence: selected Tools are returned in
// Roster (leaves-first) order regardless of the order they were supplied, and
// selfSelected reports whether shll itself was named (the caller processes it first,
// before the roster loop). aliased lists the legacy alias tokens the caller passed
// (in the order encountered) so the caller can print a one-line "note: rk is now
// run-kit" notice — resolveTargets stays IO-free and does no printing itself.
//
// On ANY unknown arg, it returns a non-nil error naming ALL unknown args (a better
// one-shot fix than reporting only the first) and listing the valid targets; the
// caller writes it to stderr and exits non-zero with no side effects. The error's
// valid-targets list shows CANONICAL names only — legacy aliases are accepted but
// never advertised. A zero-length args slice yields an empty selection,
// selfSelected=false, and no aliases (the caller keeps its whole-roster path).
func resolveTargets(args []string, allowShll bool) (selected []Tool, selfSelected bool, aliased []string, err error) {
	// Validate every arg up front; collect unknowns so all are reported at once.
	var unknown []string
	wanted := make(map[string]bool, len(args))
	for _, a := range args {
		if allowShll && a == shllTargetToken {
			selfSelected = true
			continue
		}
		// Legacy alias: resolve to the canonical Roster name and record the alias
		// token so the caller can print the rename notice. Checked before rosterHas
		// (an alias is by definition not a current Roster name). Args form a SET
		// (see the doc comment), so a repeated alias token (e.g. `update rk rk`) is
		// recorded ONCE — `wanted[canonical]` is idempotent, and appending to aliased
		// is gated on the same first-seen check so the caller prints one notice per
		// distinct alias, matching the once-per-run notice contract in update.go.
		if canonical, ok := legacyAliases[a]; ok && rosterHas(canonical) {
			if !wanted[canonical] {
				aliased = append(aliased, a)
			}
			wanted[canonical] = true
			continue
		}
		if rosterHas(a) {
			wanted[a] = true
			continue
		}
		unknown = append(unknown, a)
	}
	if len(unknown) > 0 {
		return nil, false, nil, fmt.Errorf("unknown target%s %s (valid targets: %s)",
			plural(len(unknown)), quoteJoin(unknown), validTargets(allowShll))
	}

	// Return the selected Tools in Roster order (not arg order) so the subset is
	// always processed leaves-first, matching the whole-roster contract.
	for _, t := range Roster {
		if wanted[t.Name] {
			selected = append(selected, t)
		}
	}
	return selected, selfSelected, aliased, nil
}

// rosterHas reports whether name is a Roster tool name. Source of truth is the live
// Roster, so the valid-name list never drifts from the roster itself.
func rosterHas(name string) bool {
	_, ok := rosterTool(name)
	return ok
}

// rosterTool returns the Roster Tool with the given name and true, or the zero Tool
// and false when name is not a roster tool. Source of truth is the live Roster
// (Constitution III), so callers never hardcode a second Tool descriptor — e.g. the
// `shll install` nudge resolves the run-kit Tool this way for its post-run install
// probe, rather than open-coding a formula/name pair.
func rosterTool(name string) (Tool, bool) {
	for _, t := range Roster {
		if t.Name == name {
			return t, true
		}
	}
	return Tool{}, false
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
