# Intake: Linux brew-trust sandbox workaround (HOMEBREW_NO_REQUIRE_TAP_TRUST)

**Change**: 260613-38a6-brew-trust-sandbox-workaround
**Created**: 2026-06-13
**Status**: Draft

## Origin

Originated from backlog item `[38a6]` (2026-06-13), itself a follow-up to a brew-upgrade failure diagnosed on 2026-06-12 while upgrading `idea` v0.0.9 → v0.0.10. One-shot intake from a fully-specified backlog entry — the root cause, the verified workaround, the preferred scoping, and the call-site inventory were all worked out during diagnosis and recorded in the backlog. The paired removal follow-up already exists as backlog item `[tkch]`.

> Bake `HOMEBREW_NO_REQUIRE_TAP_TRUST=1` into shll's brew install/upgrade invocations as a temporary workaround for a Homebrew 6.0 Linux-sandbox bug.
>
> ROOT CAUSE (diagnosed 2026-06-12; not a shll or tap bug): Homebrew 6.0's Linux bubblewrap sandbox runs the formula build/install in `build.rb` inside `bwrap`, where `deny_read_home` (`Library/Homebrew/sandbox.rb` ~line 191) masks almost all of `$HOME`. Its exception list covers `HOMEBREW_PREFIX`/`CACHE`/`LOGS`/`TEMP` but NOT `HOMEBREW_USER_CONFIG_HOME` (`~/.homebrew`), where `trust.json` lives. So when `HOMEBREW_REQUIRE_TAP_TRUST=1` is set, the sandboxed `build.rb` re-checks tap trust, cannot read `~/.homebrew/trust.json`, and raises `Homebrew::UntrustedTapError` at `build.rb:264` — three lines BEFORE the error pipe is connected (`build.rb:267`), so the message is swallowed and brew surfaces only an opaque `bwrap … exited with 1` with a giant arg dump. This breaks `brew install/upgrade sahil87/tap/<formula>` for EVERY roster tool on any Linux box where the user set `HOMEBREW_REQUIRE_TAP_TRUST=1` — which shll itself encourages via `shll shell-setup --trust-tap`. Verified workaround: prefix the brew call with `HOMEBREW_NO_REQUIRE_TAP_TRUST=1` (keeps the sandbox active, skips only the broken in-sandbox trust re-check); `HOMEBREW_NO_REQUIRE_TAP_TRUST=1 brew upgrade sahil87/tap/idea` completed cleanly.

## Why

**The pain point.** shll actively pushes users toward `HOMEBREW_REQUIRE_TAP_TRUST=1`: `shll shell-setup --trust-tap` runs `brew trust --tap sahil87/tap` and writes `export HOMEBREW_REQUIRE_TAP_TRUST=1` into the user's rc file (`ensureTapTrust` in `brew.go`). On any Linux box that has run that ceremony, every subsequent `shll install` and `shll update` that shells out to `brew install`/`brew upgrade sahil87/tap/<formula>` now fails — the sandboxed `build.rb` cannot read `~/.homebrew/trust.json` under bubblewrap's `deny_read_home`, raises `UntrustedTapError` before the error pipe is wired, and brew reports only an opaque `bwrap … exited with 1`. So shll's own recommended configuration breaks shll's two most important commands, with a failure mode that gives the user no actionable signal.

**The consequence if unfixed.** Every roster-tool install/upgrade is broken on trust-enabled Linux. The error is undiagnosable from the surface (`bwrap exited 1` + arg dump), so users cannot self-remediate. shll's value proposition — one entry point to keep the toolkit current — is dead on exactly the platform+config shll told the user to adopt.

**Why this approach over alternatives.** The verified, minimal fix is to set `HOMEBREW_NO_REQUIRE_TAP_TRUST=1` *only on shll's own brew subprocess invocations*. This is strictly narrower than disabling the sandbox (`HOMEBREW_NO_SANDBOX=1`) — it keeps bubblewrap active and skips only the broken in-sandbox trust *re-check*, not the security boundary. The trust was already verified out-of-sandbox before the build phase; the in-sandbox re-check is redundant and is exactly the buggy step. Alternatives considered and rejected:
- *Disable the sandbox entirely* — over-broad; removes a real security boundary to dodge a trust-check bug.
- *Stop recommending `HOMEBREW_REQUIRE_TAP_TRUST=1`* — abandons a deliberate pro-trust posture (Constitution-adjacent design intent in `shell_setup.go`/`brew.go`) over a transient upstream bug.
- *Set the override unconditionally on all platforms* — silently neuters trust enforcement during install/upgrade on macOS, where the bug does not exist. Rejected in favour of Linux-only gating (the bug is bubblewrap-specific; macOS sandboxing is unaffected).

This is explicitly a **temporary** workaround for an upstream Homebrew bug, paired with removal item `[tkch]`, not a permanent design choice.

## What Changes

Inject `HOMEBREW_NO_REQUIRE_TAP_TRUST=1` into the environment of shll's brew-spawning subprocesses **only**, gated to Linux, with a single source of truth and loud temporary-workaround documentation.

### 1. `proc.Request` gains an `Env` field

`internal/proc/proc.go` — `Request` currently has `Name`, `Args`, `Transport`, `Dir`. Add an `Env []string` field (extra `KEY=VALUE` entries to append to the child's environment, not replace it):

```go
type Request struct {
    Name      string
    Args      []string
    Transport Transport
    Dir       string
    Env       []string // extra KEY=VALUE entries appended to the parent env; nil = inherit only
}
```

`defaultRunner` sets `cmd.Env` only when `req.Env` is non-empty, by appending to the current process environment so the child still inherits everything else:

```go
func defaultRunner(ctx context.Context, req Request) Result {
    cmd := exec.CommandContext(ctx, req.Name, req.Args...)
    if req.Dir != "" {
        cmd.Dir = req.Dir
    }
    if len(req.Env) > 0 {
        cmd.Env = append(os.Environ(), req.Env...) // appended entries win on duplicate keys (Go uses last value)
    }
    // ... transport switch unchanged
}
```

Leaving `cmd.Env` nil when `req.Env` is empty preserves today's behavior (full parent-env inheritance) for every non-brew call. NOTE: Go's `exec` uses the **last** value for a duplicated key, so appending `HOMEBREW_NO_REQUIRE_TAP_TRUST=1` overrides any inherited value — correct.

### 2. Plumbing `Env` from the helper layer to `Request`

`proc.Run` and `proc.RunForeground` are variadic free functions (`Run(ctx, name, args...)`) that construct the `Request` internally — callers never build a `Request` directly. To carry `Env` from a brew call site to `defaultRunner`, add env-carrying variants rather than changing every existing call signature:

```go
// RunForegroundEnv is RunForeground with extra environment entries appended to the
// child's inherited environment. env is a slice of KEY=VALUE strings.
func RunForegroundEnv(ctx context.Context, env []string, name string, args ...string) (int, error) {
    res := Runner(ctx, Request{Name: name, Args: args, Transport: TransportForeground, Env: env})
    if res.Err != nil {
        return -1, res.Err
    }
    return res.ExitCode, nil
}
```

(A capture-mode `RunEnv` is only added if a brew *capture* call needs it — see Impact; the live brew failures are all foreground install/upgrade/update, so the foreground variant is the one required.) The existing `Run`/`RunForeground` keep their signatures and simply pass `Env: nil`.

### 3. `brewEnv()` — single source of truth in `brew.go`

Add a helper next to `brewBinary` so all brew invocations share one env source and the future removal (`[tkch]`) is a one-spot edit:

```go
// brewEnv returns the extra environment entries shll injects into its brew
// subprocesses. It is the SINGLE source of truth for the temporary Homebrew
// Linux-sandbox workaround.
//
// TEMPORARY WORKAROUND (backlog [38a6]; remove per backlog [tkch]):
// Homebrew 6.0's Linux bubblewrap sandbox masks ~/.homebrew (HOMEBREW_USER_CONFIG_HOME)
// via deny_read_home, so the sandboxed build.rb cannot read trust.json and wrongly
// raises UntrustedTapError when HOMEBREW_REQUIRE_TAP_TRUST=1 is set (the error is also
// swallowed — raised before build.rb connects its error pipe — surfacing as an opaque
// "bwrap exited with 1"). We set HOMEBREW_NO_REQUIRE_TAP_TRUST=1 to skip ONLY the broken
// in-sandbox trust re-check; the sandbox itself stays active and trust was already
// verified out-of-sandbox. Gated to Linux: the bug is bubblewrap-specific and macOS must
// keep enforcing trust. DELETE this (and its wiring/tests) once the upstream fix lands —
// see backlog [tkch] for the verification recipe.
func brewEnv() []string {
    if osGoos != "linux" {
        return nil
    }
    return []string{"HOMEBREW_NO_REQUIRE_TAP_TRUST=1"}
}
```

The Linux gate reuses the existing **`osGoos`** indirection (`var osGoos = runtime.GOOS` in `shell_setup.go:21`), which tests already override to exercise per-OS behavior — so the darwin/linux assertions need no build constraints. (If `osGoos`'s current home in `shell_setup.go` reads oddly for a brew helper, it MAY be hoisted to a more neutral file in the same package; that is a placement detail, not a behavior change.)

### 4. Apply `brewEnv()` at the live brew call sites

Switch the brew **install/upgrade/update** foreground calls from `proc.RunForeground` to `proc.RunForegroundEnv(ctx, brewEnv(), …)`:

| Site (current) | Call | Notes |
|---|---|---|
| `install.go:154` | `brew install <formula>` | live — per missing roster tool |
| `update.go:242` | `brew update --quiet` | live — once per `shll update` |
| `update.go:295` | `brew upgrade <shllFormula>` | live — shll self-upgrade |
| `update.go:404` (`upgradeTool` → `upgradeArgv`) | `brew upgrade <formula>` fallback | guarded — see below |

For the `upgradeTool`/`upgradeArgv` path: `upgradeArgv` builds the argv for BOTH the per-tool `<tool> update` delegation AND the `brew upgrade <formula>` fallback, and is shared with the dry-run preview. **The env must NOT be attached to per-tool `<tool> update` delegations** (only brew calls get it). So the env decision belongs in `upgradeTool` (the live runner), keyed on whether the argv is a brew call — e.g. inject `brewEnv()` only when `argv[0] == brewBinary`:

```go
func upgradeTool(ctx context.Context, t Tool, supportsSkipFlag bool) (int, error) {
    argv := upgradeArgv(t, supportsSkipFlag)
    var env []string
    if argv[0] == brewBinary { // brew upgrade fallback gets the workaround env; <tool> update does not
        env = brewEnv()
    }
    return proc.RunForegroundEnv(ctx, env, argv[0], argv[1:]...)
}
```

`upgradeArgv` and `argvString` (dry-run preview) are untouched — env is a runtime concern, not part of the displayed argv. NOTE: per `docs/memory/cli/update.md` / `context.md`, all six roster tools currently expose `update`, so the `brew upgrade <formula>` fallback is **presently unreachable for the roster**; it is still wired for correctness and future tools.

### 5. Out of scope — calls that do NOT get the env

- Per-tool `<tool> update [--skip-brew-update]` delegations (those tools run their own brew internally — not shll's process to configure).
- `brew` **capture/query** calls that are not install/upgrade and never trip the sandboxed build phase: `brew --version` (`hasBrew`), `brew list --formula --versions` (`isInstalled`), `brew info --json=v2` (version probes), `brew trust`/`brew trust --help` (the trust ceremony itself — `brewTrustTap`/`brewTrustAvailable`). These are read-only or trust-establishing and must not carry the override.
- Any non-brew subprocess (shell-init composition, `<tool> --version`).

### 6. Tests

Extend `proc` and/or `brew`/`update`/`install` tests (the `fakeRunner` already records full `proc.Request` values via `calls`, including the new `Env` field):
- Assert each live brew install/upgrade/update `Request` carries `Env` containing `HOMEBREW_NO_REQUIRE_TAP_TRUST=1` when `osGoos == "linux"`.
- Assert the same `Request`s carry **no** such env when `osGoos == "darwin"` (override `osGoos` in the test, mirroring existing `shell_setup_test.go` patterns).
- Assert per-tool `<tool> update` delegation `Request`s never carry the env (on any OS).
- A `defaultRunner`-level (or `proc_test.go`) assertion that a non-empty `Env` results in `cmd.Env` = parent env + appended entries, and an empty/nil `Env` leaves inheritance untouched.

## Affected Memory

- `internal/proc`: (modify) document the new `Env []string` field on `Request` and the env-carrying helper variant (`RunForegroundEnv`); note that empty `Env` preserves full parent-env inheritance.
- `cli/install`: (modify) note that `brew install` carries the Linux-only `HOMEBREW_NO_REQUIRE_TAP_TRUST=1` workaround env, cross-referencing backlog `[tkch]` for removal.
- `cli/update`: (modify) note the same workaround on `brew update`, `brew upgrade <shll-self>`, and the (currently unreachable) `brew upgrade <formula>` fallback; record that per-tool `<tool> update` delegations are deliberately excluded.

## Impact

- **Code**: `internal/proc/proc.go` (add `Env` field + `RunForegroundEnv`, set `cmd.Env` in `defaultRunner`); `src/cmd/shll/brew.go` (add `brewEnv()` next to `brewBinary`); `src/cmd/shll/install.go` (one call site); `src/cmd/shll/update.go` (three call sites: `runUpdate` brew-update + shll-self + `upgradeTool`). Possibly hoist/reference `osGoos`.
- **Constitution**: Principle I (Security First) — preserved: still routed through `internal/proc`, no raw `os/exec`, no shell strings; the env is an explicit `[]string`. Principle VII (Minimal Surface Area) — no new subcommand. Principle II (No State) — unaffected; the env is derived per-invocation, not persisted.
- **Cross-platform** (constitution "Cross-Platform Behavior"): the new behavior is OS-conditional via `osGoos`; darwin/linux both build and are covered by tests.
- **No user-facing CLI change**: no new flags, no changed output. The only observable difference is that trust-enabled Linux installs/upgrades stop failing.
- **Dependencies**: none added.
- **Removal coupling**: paired with backlog `[tkch]`; the `brewEnv()` centralization is what makes removal a one-spot edit.

## Open Questions

- Helper shape for env plumbing: add a dedicated `RunForegroundEnv` (and only-if-needed `RunEnv`) variant vs. changing `Run`/`RunForeground` to variadic-options. The variant approach keeps every existing call site untouched and is the assumed default; confirm during apply if a broader options refactor is preferred.
- `osGoos` placement: currently lives in `shell_setup.go`. Reuse in place, or hoist to a neutral file (e.g. `brew.go` or a small `platform.go`) since it now gates brew behavior too? Behavior-neutral either way.
- Whether to also add a capture-mode `RunEnv` now (unused by live failures) or defer until a capture brew call needs it. Default: defer (YAGNI) — only `RunForegroundEnv` is required.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Inject `HOMEBREW_NO_REQUIRE_TAP_TRUST=1` (not `HOMEBREW_NO_SANDBOX`) as the override | Backlog `[38a6]` records this as the *verified* minimal fix; keeps sandbox active | S:98 R:80 A:90 D:95 |
| 2 | Certain | Gate the workaround to Linux via the existing `osGoos` indirection | Backlog states "PREFERRED scoping: gate to Linux only"; bug is bubblewrap-specific; `osGoos` seam already exists for testable OS branching | S:95 R:75 A:90 D:90 |
| 3 | Certain | Add `Env []string` to `proc.Request`; `defaultRunner` sets `cmd.Env = append(os.Environ(), req.Env...)` only when non-empty | Backlog prescribes this exact shape; appending preserves inheritance; nil-guard preserves current behavior | S:95 R:70 A:88 D:85 |
| 4 | Certain | Centralize the override in a `brewEnv()` helper in `brew.go` next to `brewBinary` | Backlog: "Centralize … so the follow-up removal is a one-spot edit" | S:96 R:78 A:90 D:92 |
| 5 | Certain | Apply env only to brew install/upgrade/update foreground calls; exclude per-tool `<tool> update` delegations | Backlog scoping is explicit; delegations run their own brew out of shll's process | S:95 R:70 A:88 D:90 |
| 6 | Certain | Document loudly in code + cross-reference removal backlog item `[tkch]` | Backlog: "Document the override loudly … link it to the removal backlog item"; `[tkch]` already exists | S:97 R:85 A:92 D:95 |
| 7 | Confident | Add a `RunForegroundEnv` variant rather than changing `Run`/`RunForeground` signatures | Helpers are variadic free funcs that build `Request` internally; a variant keeps all existing call sites untouched (smallest blast radius) — but the exact helper shape was not pre-specified | S:70 R:65 A:75 D:70 |
| 8 | Confident | Inject `brewEnv()` in `upgradeTool` keyed on `argv[0] == brewBinary`, leaving `upgradeArgv`/`argvString` untouched | `upgradeArgv` is shared with the dry-run preview and builds both brew and `<tool> update` argv; env is a runtime concern, must not leak into the displayed argv or onto per-tool delegations | S:72 R:68 A:80 D:72 |
| 9 | Confident | Exclude all brew *capture* calls (`--version`, `list`, `info`, `trust`) — only the build phase trips the bug | The sandboxed `build.rb` only runs during install/upgrade; read-only/trust calls don't enter that phase | S:75 R:70 A:82 D:78 |
| 10 | Confident | Assert presence on linux / absence on darwin by overriding `osGoos` in tests, mirroring `shell_setup_test.go` | `fakeRunner` records full `Request`s incl. `Env`; `osGoos` is already a documented test seam | S:78 R:75 A:85 D:80 |
| 11 | Certain | Defer a capture-mode `RunEnv` helper (add only if a capture brew call later needs it) | Clarified — user confirmed assumptions; YAGNI deferral accepted (no live capture-brew failure path) <!-- clarified: defer RunEnv until a capture brew call needs env — user confirmed --> | S:95 R:70 A:55 D:50 |
| 12 | Certain | Keep `osGoos` in `shell_setup.go` (reference it from `brew.go`) rather than hoisting to a neutral file | Clarified — user confirmed assumptions; reuse-in-place chosen (smaller diff, behavior-neutral) <!-- clarified: reuse osGoos in place; hoist deferred — user confirmed --> | S:95 R:72 A:55 D:48 |

12 assumptions (8 certain, 4 confident, 0 tentative, 0 unresolved).
