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

// Tool describes one entry in the hardcoded shll toolkit roster. The list is
// the source of truth for `shll update`, `shll shell-init`, and `shll version`
// (Constitution III — Tool Roster Source of Truth). Adding a new tool requires
// a shll release; no runtime discovery.
type Tool struct {
	// Name is the binary name (also used as the brew formula leaf and as the
	// label printed by `shll version`).
	Name string
	// Formula is the fully-qualified Homebrew formula name passed to brew.
	// EMPTY for a delegated (non-brew) tool — see the Install/Probe fields;
	// every brew-centric helper MUST branch on brewManaged() before using it.
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
	// For a delegated (non-brew) tool this IS the update path (e.g. rk-desktop's
	// `{"rk", "desktop", "update"}`) — there is no brew fallback. An empty slice
	// on a BREW-MANAGED tool means the tool exposes no `update` subcommand —
	// `shll update` falls back to `brew upgrade <formula>` for it. Every current
	// roster tool ships an `update`, so all entries populate this field.
	Update []string
	// Skill is the argv of the tool's skill-bundle invocation (e.g. {"fab",
	// "skill"} for fab-kit, whose `skill` subcommand lives on the `fab` router
	// binary rather than the `fab-kit` binary). An empty slice means the
	// default `{Name, "skill"}` — only tools whose skill surface diverges from
	// their roster Name populate this field.
	Skill []string
	// Description is a one-line, human-readable summary of what the tool does,
	// printed by `shll list`. Single-sourced here so the roster cannot drift
	// from the managed set (Constitution III — Tool Roster Source of Truth).
	Description string
	// SkillHint is a short task-domain phrase ("git worktrees", "backlog ideas")
	// woven into the generated `shll setup agent` skill description, so an agent
	// matching on task vocabulary — not just tool names — triggers the toolkit
	// skill. Kept on the roster (not hand-written in agent_setup.go) so the skill
	// description cannot drift from the managed set (Constitution III). Required
	// for every roster entry — enforced by TestRosterSkillHints.
	SkillHint string
	// ProactiveHint is one or more complete sentences (prose, appended verbatim)
	// describing a capability the AGENT should reach for UNPROMPTED (without the
	// user naming a tool) — the agent-proactive trigger vocabulary appended to
	// the generated `shll setup agent` skill description. Empty for every tool
	// except run-kit (the sprawl guard: only agent-proactive capabilities earn
	// description space; reactive tools stay behind the two-step router because
	// the user's words name them). Kept on the Roster (Constitution III) so the
	// description cannot drift from the managed set. Optional-by-design — unlike
	// SkillHint it is NOT required for every entry.
	ProactiveHint string
	// Repo is the github.com/sahil87/<Repo> slug for the tool's source
	// repository. It defaults to Name for most tools. Historically it was NOT
	// always equal to Name (rk's repo was `run-kit`); after the rk→run-kit rename
	// Name and Repo match for run-kit, so today every roster entry's Repo equals
	// its Name. The field stays explicit so `shll list` never emits a dead link
	// if a future tool's binary name and repo slug diverge again.
	Repo string
	// LegacyName is the tool's PRIOR binary name, retained as a binary-alias/
	// display surface: the run-kit formula still installs `rk` as an
	// interchangeable command alias, and when `<Name> --version` returns
	// proc.ErrNotFound, probeToolVersion retries `<LegacyName> --version` so an
	// install whose binary is on PATH under the old name is shown as installed by
	// list/version/doctor. Empty for every tool except run-kit ("rk").
	LegacyName string
	// Install is the argv of the tool's delegated install invocation (e.g.
	// {"rk", "desktop", "install"}). Empty for brew-managed tools, whose install
	// path is `brew install <Formula>`. A non-empty Install marks the NON-BREW
	// (delegated) seam: such a tool carries NO Formula, is skipped by every
	// brew-centric helper (trust, `brew list`, `brew upgrade`, `brew uninstall`),
	// and MUST carry a Probe. Populated today only by rk-desktop; the seam is
	// field-driven so a future non-brew tool needs only a roster entry.
	Install []string
	// Probe describes how to detect a delegated (non-brew) tool's installed
	// state and version — the non-brew counterpart of the brew
	// `brew list --versions` probe and the `<tool> --version` PATH probe. Nil for
	// brew-managed tools. See ToolProbe.
	Probe *ToolProbe
}

// ToolProbe is the installed-state probe spec for a delegated (non-brew) roster
// tool. The probe runs Argv (via proc.RunCaptured — both streams captured so a
// platform-refusal message on either stream is detectable), then scans the
// output for a line starting with LinePrefix:
//
//   - the line's value equals AbsentValue → NOT installed;
//   - any other value → installed, and the value IS the installed version
//     (e.g. `Installed: v1.2.3` → `v1.2.3`);
//   - no matching line, a transport error, or a non-zero exit → not installed
//     (callers distinguish the failure detail only when they need it).
//
// rk-desktop's probe is {"rk", "desktop", "status"} with LinePrefix
// "Installed:" — run-kit's status output already distinguishes
// `Installed: not installed` from `Installed: v<X>`.
type ToolProbe struct {
	// Argv is the probe invocation (binary + args).
	Argv []string
	// LinePrefix selects the status line to parse (e.g. "Installed:").
	LinePrefix string
	// AbsentValue is the line value meaning "not installed" (e.g.
	// "not installed").
	AbsentValue string
}

// brewManaged reports whether the tool is installed/upgraded/removed through
// Homebrew (Formula non-empty). The delegated (non-brew) seam is the inverse:
// brewManaged()==false means install/update delegate to Install/Update argvs
// and detection goes through Probe, with no formula for any brew helper.
func (t Tool) brewManaged() bool { return t.Formula != "" }

// rkBinary is the run-kit CLI binary name — the argv[0] every rk-desktop
// delegation (`rk desktop install/update/status`) runs through. Named per
// code-quality.md (no magic strings); distinct from runKitToolName
// (agent_setup.go), which is the ROSTER display name used for name matching.
const rkBinary = "rk"

// rkDesktopRefusalToken is the stable substring of run-kit's unsupported-platform
// refusal (errDesktopMacOnly in run-kit's cmd/rk/desktop.go: "rk desktop is
// macOS-only (the shell is packaged as a macOS .app)"). shll matches this token
// to distinguish a PLATFORM REFUSAL (skip-with-note in whole-roster runs, an
// explicit message on targeted runs) from a real failure — NEVER a hardcoded
// darwin check, so when run-kit grows Linux support shll needs zero changes.
// Named per code-quality.md (no magic strings).
const rkDesktopRefusalToken = "rk desktop is macOS-only"

// isRkDesktopRefusal reports whether captured `rk desktop …` output (either
// stream) carries run-kit's unsupported-platform refusal. See
// rkDesktopRefusalToken.
func isRkDesktopRefusal(out []byte) bool {
	return strings.Contains(string(out), rkDesktopRefusalToken)
}

// githubOrgBase is the GitHub organization base URL for the shll toolkit.
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

// Roster is the hardcoded shll toolkit list. Order matters: it is declared
// IMPORTANCE-DESCENDING with dependency adjacency — the tools a user reaches for
// first lead the list, and a tool sits immediately after the runtime it depends
// on (rk-desktop directly after run-kit, whose `rk desktop …` subcommands it
// delegates to).
//
// Every roster-driven surface inherits this order from the single slice:
// install/update walk order, uninstall's reverse walk, list/doctor/version row
// order, and shell-init composition order — no surface hardcodes a second order.
// (This replaces the earlier leaves-first dependency-edge ordering: brew
// resolves formula dependencies correctly and idempotently regardless of walk
// order, so the order is output coherence and meaning, not correctness.)
//
// rk-desktop is the roster's first DELEGATED (non-brew) entry: it carries no
// Formula — install/update delegate to `rk desktop install`/`rk desktop update`
// (Constitution III/IV — wrap, don't reinvent), and its installed-probe parses
// the `Installed:` line of `rk desktop status`. Its dependency on run-kit is
// expressed as a runtime probe + this roster adjacency only (install-composition
// standard, Policy A — no formula edge). The exact order is enforced by
// TestRosterOrder — a comment cannot fail CI, so the test guards against an
// accidental reorder.
var Roster = []Tool{
	{Name: "run-kit", Formula: formulaPrefix + "run-kit", Update: []string{"run-kit", "update"}, Repo: "run-kit", LegacyName: "rk", Description: "Run-kit — tmux session manager with a web UI; can display web pages/HTML to the user, push notifications, and run VS Code palette commands in its code editor via `rk code exec` (rk stays as an alias)", SkillHint: "tmux sessions", ProactiveHint: "Also use proactively — without the user naming a tool — to show the user visual content (HTML, diagrams, reports, a local dev server) in a browser window, to proxy a local http port to the user's browser, to push a notification to their devices, or to act inside the user's code editor — run any VS Code palette command (refresh a PR list, open a diff, focus a view) from the shell with `rk code exec` (run-kit). The user may be viewing this session remotely through run-kit's web dashboard, where `open`/`xdg-open` and localhost URLs never reach them and publishing to a hosted artifact page (e.g. claude.ai) forces them off the dashboard — before opening any file or local port in a browser, read `shll skill run-kit` for the proxied-iframe recipe; the same applies before publishing an artifact or hosted page to show the user something."},
	{Name: "rk-desktop", Update: []string{rkBinary, "desktop", "update"}, Install: []string{rkBinary, "desktop", "install"}, Probe: &ToolProbe{Argv: []string{rkBinary, "desktop", "status"}, LinePrefix: "Installed:", AbsentValue: "not installed"}, Repo: "run-kit", Description: "Run-kit desktop viewer shell — the macOS companion app, managed via `rk desktop install`/`rk desktop update`", SkillHint: "desktop viewer shell"},
	{Name: "fab-kit", Formula: formulaPrefix + "fab-kit", Update: []string{"fab-kit", "update"}, Skill: []string{"fab", "skill"}, Repo: "fab-kit", Description: "Spec-driven workspace & workflow toolkit (the `fab` CLI)", SkillHint: "spec-driven workflows"},
	{Name: "wt", Formula: formulaPrefix + "wt", ShellInit: []string{"wt", "shell-init", shellPlaceholder}, Update: []string{"wt", "update"}, Repo: "wt", Description: "Git worktree management — create, list, open, delete worktrees", SkillHint: "git worktrees"},
	{Name: "idea", Formula: formulaPrefix + "idea", Update: []string{"idea", "update"}, Repo: "idea", Description: "Backlog idea management from the terminal", SkillHint: "backlog ideas"},
	{Name: "tu", Formula: formulaPrefix + "tu", ShellInit: []string{"tu", "shell-init", shellPlaceholder}, Update: []string{"tu", "update"}, Repo: "tu", Description: "Token-usage tracker for AI coding tools (Claude Code, Codex, OpenCode)", SkillHint: "AI token-usage tracking"},
	{Name: "hop", Formula: formulaPrefix + "hop", ShellInit: []string{"hop", "shell-init", shellPlaceholder}, Update: []string{"hop", "update"}, Repo: "hop", Description: "Fast directory/project jumping across worktrees", SkillHint: "directory/project jumping"},
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
// this descriptor, rendering shll FIRST, then the Roster
// (`shll, run-kit, rk-desktop, fab-kit, wt, idea, tu, hop`).
//
// It reuses the Tool struct shape but is deliberately NOT a Roster entry: Roster
// is the *managed sub-tool* list (Constitution III — Tool Roster Source of Truth),
// and adding shll there would break the roster invariants guarded by
// TestRosterOrder and make install/update/shell-init try to
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
// Roster order regardless of the order they were supplied, and
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
	// always processed in roster order, matching the whole-roster contract.
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
