# Intake: Rename shell-setup command (shell-install becomes back-compat alias)

**Change**: 260602-ri3h-shell-setup-rename
**Created**: 2026-06-02
**Status**: Draft

## Origin

Synthesized from a prior conversation. The user wants `shell-setup` to be the
primary, more-intuitive verb for the command that wires the cross-tool eval line
into a shell rc file, while keeping the existing `shell-install` name working
forever via a cobra alias (zero breakage for existing rc files, scripts, docs,
and muscle memory).

The user made two explicit design choices during discussion:

1. **`shell-setup` is the new canonical name**; `shell-install` is the alias —
   NOT the other way around (a one-way alias with `shell-install` staying
   canonical was explicitly rejected).
2. **Full identifier rename** for maximal internal consistency — Go identifiers
   (file, factory, run helpers, test file/helpers) all move from the
   `ShellInstall` stem to `ShellSetup`. A command-string-only rename that left
   the Go identifiers as `ShellInstall` was explicitly rejected.

> "Rename `shll shell-install` so `shell-setup` becomes the new canonical name,
> with `shell-install` retained as a backward-compatibility cobra alias. Do the
> full rename including Go identifiers."

Mode: one-shot, fully-specified (not conversational). The branch is already
rebased onto origin/main (latest includes commit e15b382 "docs: recommend
--trust-tap in Quick start").

## Why

1. **Problem**: `shell-install` reads as "install software", but the command
   does not install anything — it appends a sentinel-wrapped `eval` block to a
   shell rc file. The user finds `shell-setup` clearer and easier to remember as
   the primary verb. (Sibling tools in the roster expose `shell-init`; "setup"
   is the natural umbrella verb for "wire my shell".)
2. **Consequence of doing nothing**: the less-intuitive name persists; "do
   nothing" was explicitly rejected.
3. **Why this approach**: cobra natively supports command `Aliases`, so
   back-compat is a single-field addition (`Aliases: []string{"shell-install"}`)
   with zero runtime cost — existing invocations dispatch to the same command.
   The full identifier rename (over a minimal command-string-only diff) buys
   internal consistency: a future reader greps `ShellSetup` and finds the file,
   factory, run helpers, and tests all aligned with the user-facing name.

## What Changes

`feat` — adds a user-facing affordance (a new canonical command name) while the
existing invocation keeps working via an alias. Surface area count is unchanged
(see Constitution VII note below).

### 1. Command factory (`src/cmd/shll/shell_install.go` → `shell_setup.go`)

- Rename the source file `src/cmd/shll/shell_install.go` →
  `src/cmd/shll/shell_setup.go`.
- In the factory: `Use: "shell-setup [shell]"`, add
  `Aliases: []string{"shell-install"}`.
- Rename the factory `newShellInstallCmd` → `newShellSetupCmd`.
- Rename the top-level run helper `runShellInstall` → `runShellSetup`.
- Rename the internal run-mode helpers off the `ShellInstall` stem for
  consistency: `runShellInstallDefault` → `runShellSetupDefault`,
  `runShellInstallPrint` → `runShellSetupPrint`,
  `runShellInstallUninstall` → `runShellSetupUninstall`.
- Update the `Short`/`Long` help text to read `shll shell-setup ...` in the
  usage examples (the canonical name in help output).

### 2. Root registration (`src/cmd/shll/root.go`)

- Update the `AddCommand(...)` call: `newShellInstallCmd()` → `newShellSetupCmd()`.
- Update the `rootLong` usage line. Currently:
  `shll shell-install [shell]  append the shell-init eval line to your rc file (idempotent)`.
  Flip the canonical reference to `shll shell-setup [shell]` (optionally note the
  alias). Keep the line aligned with the other subcommand description lines.

### 3. Test file (`shell_install_test.go` → `shell_setup_test.go`)

- Rename `src/cmd/shll/shell_install_test.go` → `src/cmd/shll/shell_setup_test.go`.
- Update test-internal identifiers/helpers that carry the stem — e.g. the helper
  `runShellInstallCmd(t, argv)` → `runShellSetupCmd(t, argv)`, and any call sites
  of `newShellInstallCmd()` → `newShellSetupCmd()`.
- All existing test cases keep passing against the renamed factory/run helpers.

### 4. NEW test — alias back-compat coverage

- Add a test asserting that invoking the command via the `shell-install` alias
  resolves/dispatches to the same cobra command as `shell-setup`. Concretely:
  build the root command and assert `rootCmd.Find([]string{"shell-install"})`
  resolves to the same `*cobra.Command` as `Find([]string{"shell-setup"})`
  (cobra's `Find` resolves aliases). <!-- assumed: assert via root.Find on both names returning the same *cobra.Command; an alternative is an end-to-end SetArgs([]string{"shell-install","--print"}) execution check — either satisfies the "alias dispatches to same command" requirement -->

### 5. Docs — `README.md`

- Flip canonical references from `shll shell-install` to `shll shell-setup`:
  the feature bullet (line ~10), the Quick-start example (line ~23), the
  section header `### shll shell-install — wire the rc file (recommended)`
  (line ~67) and its example block (lines ~70-74), and the troubleshooting
  references (lines ~101, ~107, ~174, ~177) — including the in-page anchor link
  to that section header (the header text and `#...` fragment must stay in sync).
- Note the `shell-install` alias for backward compatibility in an appropriate
  place (e.g. one line under the section header).

### 6. Docs — `docs/memory/cli/shell-install.md`

- Reflect the rename: canonical command name `shll shell-setup`, the
  `shell-install` alias, the `runShellSetup` seam name, and the renamed source
  file path `src/cmd/shll/shell_setup.go`.
- The `TestNoProcImports` source-path line reference must point at the renamed
  files. NOTE: the description cites `shell_install.go:923`; the actual citation
  in this memory file is `shell_install_test.go:923` (the line reference is to
  the *test* file). Both the source path (`shell_install.go` →
  `shell_setup.go`) AND the test-file path (`shell_install_test.go` →
  `shell_setup_test.go`) references need updating throughout this file (multiple
  occurrences: lines ~5, ~152, ~154, ~156, ~243-261). Treat exact line numbers
  (`:923`, `:28`) as best-effort — they will shift and may be dropped/updated
  during hydrate; the file/identifier names are the load-bearing part.
  <!-- assumed: this memory file is renamed (shell-install.md → shell-setup.md) and the memory index updated during hydrate, not during apply — intake only captures that its content references need updating -->

### Explicitly OUT OF SCOPE

- **Sentinel block** `# >>> shll >>>` / `# <<< shll <<<` — command-name-agnostic;
  do not touch.
- **Legacy sentinel handling** `# >>> shll shell-init >>>` — unrelated migration
  machinery; stays as-is.
- **Eval line** `eval "$(shll shell-init <shell>)"` — references the SEPARATE
  `shell-init` command, not this one. Do not change.
- No migration machinery changes, no rc-file behavior changes.

## Affected Memory

- `cli/shell-install`: (modify) Update canonical command name to `shll shell-setup`,
  document the `shell-install` alias, update the `runShellSetup` seam name and the
  renamed source/test file paths (`shell_setup.go` / `shell_setup_test.go`). The
  file itself is expected to be renamed to `cli/shell-setup` during hydrate, with
  `docs/memory/index.md` updated to match.
- `cli/commands`: (modify) The command roster / surface listing references
  `shell-install` as the canonical name — flip to `shell-setup` (alias noted).
  <!-- assumed: cli/commands.md enumerates the subcommand surface and will need the canonical name flipped; confirm at hydrate -->

## Impact

- **Code**: `src/cmd/shll/shell_install.go` (renamed → `shell_setup.go`, factory +
  run-helper identifiers), `src/cmd/shll/root.go` (registration + `rootLong`),
  `src/cmd/shll/shell_install_test.go` (renamed → `shell_setup_test.go`, helper
  identifiers + new alias test).
- **Implementation comment**: `src/cmd/shll/brew.go:92` references
  `shell_install.go` in a `TestNoProcImports` explanatory comment. Update it to
  `shell_setup.go` for consistency (low-risk, comment-only; aligns with the
  "internal helper names at implementer's discretion" intent).
- **No public API/behavior change**: the binary's command surface is identical
  except the canonical name displayed in help; the old name still works.
- **No dependency changes**: cobra `Aliases` is built in.
- **Build/test**: `just build` and the `cmd/shll` Go test package must pass.
- **Sibling-tool independence**: this is shll-only. No per-tool CLI changes
  (Constitution IV unaffected).

## Constitution Notes

- **VII — Minimal Surface Area**: This change does NOT add a new top-level
  subcommand. It renames an existing subcommand and aliases the old name. The
  command count is unchanged — still five top-level commands (`install`,
  `update`, `shell-init`, `shell-setup` (was `shell-install`), `version`). No new
  surface-area justification is required; the bar Constitution VII raises for
  *additions* does not apply to a rename-plus-alias.
- **I — Security First / TestNoProcImports invariant**: the file stays file-I/O
  only; the `TestNoProcImports` guard moves with the renamed source/test files.
  No subprocess seam changes.

## Open Questions

(none — the change is fully specified.)

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | `shell-setup` is the canonical `Use:` name; `shell-install` is a cobra `Aliases` entry | Explicitly chosen by user; rejected the inverse | S:98 R:75 A:95 D:95 |
| 2 | Certain | Full identifier rename (file, factory `newShellSetupCmd`, run helpers, test file/helpers) off the `ShellInstall` stem | User explicitly chose full internal-consistency rename over command-string-only | S:95 R:70 A:92 D:92 |
| 3 | Certain | Add `Aliases: []string{"shell-install"}` for back-compat | Stated requirement; cobra native one-field addition | S:98 R:80 A:98 D:98 |
| 4 | Certain | Change type is `feat` (new canonical affordance; old name keeps working) | Description states feat; keyword "rename" also present but the user-facing-affordance framing dominates and the description pins `feat` | S:90 R:85 A:85 D:80 |
| 5 | Certain | Out-of-scope items (`# >>> shll >>>` sentinel, legacy `shell-init` sentinel, the `shell-init` eval line, migration machinery) are untouched | Explicitly enumerated as out of scope | S:98 R:80 A:95 D:95 |
| 6 | Certain | Constitution VII surface-area count unchanged (still five commands) — note included, no new justification needed | Rename + alias is not an addition; description directs including the VII note | S:95 R:90 A:95 D:90 |
| 7 | Confident | Rename internal run-mode helpers too (`runShellInstallDefault/Print/Uninstall` → `runShellSetup*`) | Description leaves this "at implementer's discretion for consistency"; full-rename intent makes renaming the obvious default | S:80 R:80 A:80 D:75 |
| 8 | Confident | New alias test asserts both names resolve to the same `*cobra.Command` via `root.Find` | Standard cobra alias-coverage idiom; requirement is "alias dispatches to same command" | S:75 R:80 A:78 D:70 |
| 9 | Certain | README canonical refs flipped including the section-header anchor and its in-page link kept in sync | Description explicitly directs updating header + quick-start + troubleshooting links; keeping the `#...` anchor in sync with the renamed header is an entailed mechanical consequence, not a discretionary choice | S:90 R:85 A:88 D:85 |
| 10 | Confident | Update `brew.go:92` comment reference `shell_install.go` → `shell_setup.go` | Not explicitly listed but a dangling stale source-path reference; consistent with full-rename intent; comment-only, low risk | S:70 R:88 A:80 D:78 |
| 11 | Certain | Memory file `cli/shell-install.md` is renamed to `cli/shell-setup.md` (+ index update) during hydrate, not apply | Description explicitly states it ("the memory file itself may be renamed during hydrate; for intake purposes just capture that its content references need updating") — explicit direction plus hydrate's documented ownership of memory-file lifecycle | S:90 R:78 A:88 D:85 |
| 12 | Tentative | `cli/commands.md` also needs the canonical name flipped | Inferred from the memory index listing `shell-install` in the cli domain surface; genuine alternative is that commands.md describes the surface generically without naming the verb, needing no change | S:55 R:65 A:60 D:55 |

12 assumptions (8 certain, 3 confident, 1 tentative, 0 unresolved).
