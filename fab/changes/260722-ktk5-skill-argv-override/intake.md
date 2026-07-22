# Intake: Per-Tool Skill Argv Override

**Change**: 260722-ktk5-skill-argv-override
**Created**: 2026-07-22

## Origin

Backlog item `[ktk5]` (2026-07-22), invoked one-shot via `/fab-new ktk5` — no prior conversation context.

> shll skill fab-kit probes 'fab-kit skill' but the bundle lives on the 'fab' router binary — add a per-tool skill argv override on the roster Tool struct (like Update/ShellInit), e.g. {fab, skill} for fab-kit, and cover the topic passthrough too. Also soften skillUnsupportedFmt: 'run shll update' is misleading when the tool is already current (fab skill works on fab-kit 2.16.8; the skill standard's own example is 'fab skill dispatch').

## Why

1. **The pain point**: `shll skill fab-kit` (and `shll skill fab-kit <topic>`) can never succeed. The composer invokes `<tool.Name> skill` — literally `fab-kit skill` — but fab-kit serves its skill bundle on its `fab` router binary, not the `fab-kit` binary. The toolkit's own `skill` standard confirms this: its topic-page example is `fab skill dispatch` (`docs/site/standards/skill.md`). So a fully current fab-kit (2.16.8, where `fab skill` works) still hits the composer's `code != 0` unsupported path.

2. **The consequence**: fab-kit's bundle is unreachable through the composer — the exact tool (a "large-scope" tool per the standard) whose depth the topic-page mechanism was built for. Worse, the failure notice (`skillUnsupportedFmt`: "does not support 'skill' yet — run 'shll update'") sends the user on a futile `shll update` loop: the tool is already current, updating changes nothing, and the notice never gets less wrong.

3. **Why this approach**: the roster `Tool` struct already solves "this tool's invocation differs from `{Name, subcommand}`" twice — `ShellInit` and `Update` are per-tool argv slices with `<shell>` substitution / brew fallback respectively. A `Skill []string` argv override is the same proven pattern, keeps the roster the single source of truth (Constitution III), and avoids any special-casing inside `skill.go`'s dispatch logic. Alternatives rejected:
   - **Hardcoding `fab` in skill.go** (e.g. a `name == "fab-kit"` branch) — bespoke per-tool logic outside the roster violates the roster-as-source-of-truth principle and would drift.
   - **Renaming the roster entry to `fab`** — `fab-kit` is the formula leaf, the binary probed by `toolInstalled`, and the user-facing tool name across `list`/`version`/`update`; renaming would ripple through every command for a skill-only concern.

## What Changes

### 1. `Tool.Skill` argv override field (`src/cmd/shll/tools.go`)

Add a `Skill []string` field to the `Tool` struct, documented in the same style as `Update`/`ShellInit`:

```go
// Skill is the argv of the tool's skill-bundle invocation (e.g. {"fab",
// "skill"} for fab-kit, whose `skill` subcommand lives on the `fab` router
// binary rather than the `fab-kit` binary). An empty slice means the
// default `{Name, "skill"}` — only tools whose skill surface diverges from
// their roster Name populate this field.
Skill []string
```

Populate it on exactly one roster entry — fab-kit:

```go
{Name: "fab-kit", Formula: formulaPrefix + "fab-kit", Update: []string{"fab-kit", "update"}, Skill: []string{"fab", "skill"}, Repo: "fab-kit", Description: "Spec-driven workspace & workflow toolkit (the `fab` CLI)", SkillHint: "spec-driven workflows"},
```

All other tools keep an empty `Skill` and get the default. (This mirrors `ShellInit`'s "empty means no divergence" semantics rather than `Update`'s "all entries populate" convention — the default is derivable, so only the exception is stated.)

### 2. Resolve the argv in both passthroughs (`src/cmd/shll/skill.go`)

Add a small resolver (named per code-quality.md, no open-coded argv composition):

```go
// skillArgv returns the argv of the tool's skill-bundle invocation: the
// roster Skill override when set (fab-kit → {"fab", "skill"}), else the
// default {Name, skillSubcommand}.
func skillArgv(tool Tool) []string {
	if len(tool.Skill) > 0 {
		return tool.Skill
	}
	return []string{tool.Name, skillSubcommand}
}
```

Both subprocess sites switch from the hardcoded `tool.Name, skillSubcommand` to the resolved argv:

- **`writeSkillBundle`**: `argv := skillArgv(tool)` → `proc.RunCaptured(subCtx, argv[0], argv[1:]...)`
- **`writeSkillTopic`** (the backlog's "cover the topic passthrough too"): same resolution with the topic appended — `proc.RunCaptured(subCtx, argv[0], append(argv[1:], topic)...)`. Build the final args without mutating the roster slice (no `append` aliasing into `Tool.Skill`'s backing array).

Everything else in both functions is untouched: error notices keep printing `tool.Name` (the user-facing name stays `fab-kit`), the failure classification (suppress-and-rewrap for one-arg, verbatim propagation for two-arg, the `code < 0` deadline guard) is unchanged, and the bare glossary keeps the PATH-only `toolInstalled` probe on `tool.Name` (installedness ≠ skill argv; the fab-kit formula ships both binaries, so probing `fab-kit --version` remains correct).

### 3. Soften `skillUnsupportedFmt` (`src/cmd/shll/skill.go`)

Current (misleading — asserts the fix is updating, even when the tool is current):

```go
const skillUnsupportedFmt = "shll skill: %s does not support 'skill' yet — run 'shll update'"
```

New (hedged: names the likely cause without promising the remedy, and targets the update at the one tool):

```go
const skillUnsupportedFmt = "shll skill: %s does not support 'skill' — its installed version may predate it (try 'shll update %s')"
```

The format now takes the tool name twice; both call sites (one in `writeSkillBundle`; the constant is not used by the topic path, which propagates verbatim) pass `tool.Name, tool.Name`.
<!-- assumed: exact softened wording — backlog fixes the direction ("soften", drop the misleading imperative) but not the phrasing; this keeps one line, names the probable cause, and scopes the suggestion to the tool -->

### 4. Tests (`src/cmd/shll/skill_test.go`)

The existing fake-runner seam (`runSkill` + fake `proc.Runner` asserting exact argv via `TransportCaptureAll`) covers the new behavior directly:

- `shll skill fab-kit` → fake asserts argv is exactly `fab skill` (not `fab-kit skill`), stdout byte-identical on success.
- `shll skill fab-kit <topic>` → fake asserts argv is exactly `fab skill <topic>`.
- A default-argv regression case: a non-override tool (e.g. `wt`) still invokes `wt skill` — pins the `skillArgv` default branch.
- The updated `skillUnsupportedFmt` assertion in the existing unsupported-path test (wording + the doubled tool-name arg).

## Affected Memory

- `cli/skill`: (modify) the passthrough sections gain the roster `Skill` argv resolution (`skillArgv`, fab-kit → `fab skill`) and the softened `skillUnsupportedFmt` wording in the named-constants and classification sections.
- `cli/commands`: (modify) the `Roster` / `Tool` struct description gains the `Skill` argv-override field alongside `ShellInit`/`Update`.

## Impact

- **Files**: `src/cmd/shll/tools.go` (struct field + fab-kit roster entry), `src/cmd/shll/skill.go` (resolver, two call sites, one constant), `src/cmd/shll/skill_test.go` (new argv assertions, updated wording assertion). Possibly `src/cmd/shll/tools_test.go` if a roster-shape test is warranted (not required — behavior tests pin the contract).
- **No CLI surface change**: no new subcommand (Constitution VII not triggered), no flag changes, `shll skill fab-kit` remains the user-facing invocation. Help text and `docs/site/skill.md` (shll's own bundle) need no edits.
- **Standards**: checked against `docs/site/standards/skill.md` — the change *implements* the standard's reality (its own example routes fab-kit's skill through `fab`); no standard text changes.
- **Blast radius**: `skill.go` only — `update`/`install`/`version`/`list`/`doctor` all key off `Name`/`Formula`/`Update` and are untouched by the new field.

## Open Questions

None — the backlog entry specifies the mechanism (roster argv override like Update/ShellInit), the value (`{fab, skill}`), the topic-passthrough coverage, and the notice-softening direction.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | `Skill []string` argv override on the `Tool` struct, resolved via a `skillArgv` helper defaulting to `{Name, "skill"}` | Backlog names the mechanism explicitly ("like Update/ShellInit, e.g. {fab, skill}"); the struct has two precedents for per-tool argv fields | S:90 R:85 A:90 D:85 |
| 2 | Certain | Only fab-kit populates the override; other tools derive the default (ShellInit-style "state only the exception") | The default is derivable from `Name` + `skillSubcommand`; populating all entries (Update-style) adds six redundant slices with no information | S:75 R:90 A:85 D:70 |
| 3 | Confident | New `skillUnsupportedFmt` wording: `"shll skill: %s does not support 'skill' — its installed version may predate it (try 'shll update %s')"` | Backlog fixes direction (soften, stop asserting update as the fix) but not phrasing; wording is trivially reversible. Marked `<!-- assumed -->` at the site | S:55 R:95 A:70 D:50 |
| 4 | Certain | Bare-glossary `toolInstalled` probe stays on `tool.Name` (`fab-kit --version`) | Installedness and skill argv are different questions; the fab-kit formula ships both binaries, so the probe is already correct | S:60 R:90 A:90 D:80 |
| 5 | Certain | Failure classification in both passthroughs unchanged (suppress-and-rewrap one-arg, verbatim propagation two-arg, `code < 0` guard) | Existing deliberate design (backlog `[tp2s]`, memory cli/skill); this change only swaps the argv fed to `proc.RunCaptured` | S:80 R:85 A:95 D:90 |

5 assumptions (4 certain, 1 confident, 0 tentative, 0 unresolved).
