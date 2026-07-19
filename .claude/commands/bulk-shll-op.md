---
description: "Run one operation across all (or a subset of) shll toolkit repos — spawn a fresh-worktree agent per repo in a dedicated tmux session via fab agent, dispatching one slash command each. The generic spawn primitive; bulk-shll-fab-upgrade reuses its roster and conventions with a mechanical core (agents only for migrations), and bulk-shll-release is a direct-loop sibling that does NOT spawn at all."
---

# /bulk-shll-op

Run a single per-repo operation across the shll toolkit as a batch: for each target repo, create a fresh worktree, start an agent in it via `fab agent` (a binary call that resolves and launches that repo's own configured session command), and hand it **one** slash command. All spawned windows land in one dedicated tmux session so the fab-operator can see and group them via `fab pane map --all-sessions`.

This is the generic spawn primitive. `/bulk-shll-fab-upgrade` is a preset over its roster and conventions — but its core move is mechanical (`fab upgrade-repo`, a binary command), and it starts an agent only when migrations are required; see its own doc. `/bulk-shll-release` is a sibling preset over the same roster that does **not** use this spawn loop — no worktree, no spawned agent, no PR — see its own doc.

> **The operator keeps tracking — and keeps ticking.** Bulk skills are generally invoked from inside a fab operator session. The operator enrolls every spawned window in its monitored set (see § Operator tracking) and resumes its loop immediately after the spawn sweep — it MUST NOT pause to babysit the spawned agents; progress is observed on subsequent ticks.

## Inputs

1. **Target repos** — the repos to run the op in.
   - **Default**: the 7-repo shll toolkit roster — `fab-kit, hop, idea, run-kit, shll, tu, wt`.
   - **Subset**: an explicit list may be passed to run against fewer repos.
   - **Root resolution**: resolve each repo's main-worktree root with `hop <repo> where` (hop's selection-first grammar). If resolution fails (`hop` unavailable or the repo not in `hop.yaml`), ask the user for the location or skip that repo — never guess a path.

2. **Per-repo task** — the recipe to run in each repo's fresh worktree, expressed as the **single** prompt / slash command handed to the spawned agent. Examples:
   - `/fab-new <description>` for a described op the agent should intake-and-run itself, or
   - `/fab-fff <change>` to drive a pre-created change through the full pipeline.

3. **PR-skip rule** — the dispatched prompt MUST instruct each agent to skip `/git-pr` entirely when the recipe produced no diff. No empty PRs.

## Per-repo loop

Mirrors fab-operator §6's spawn sequence, **minus the operator's enrollment steps**. For each target repo:

1. **Resolve the main-worktree root** — `hop <repo> where`; on failure, ask the user for the location or skip the repo.
2. **Create a fresh worktree** — run `wt create --non-interactive` **with the target repo as the working directory** (so the worktree lands under `$(dirname <repo-root>)/<repo>.worktrees/`, not the invoker's repo). Capture the generated worktree name `<wt>` and its absolute path.
3. **Start the agent in the worktree** — open one window in the dedicated bulk session running `fab agent` (a binary call):

   ```sh
   tmux new-window -t <bulk-session> -n "<wt>-<repo>" -c <worktree-path> "fab agent"
   ```

   `fab agent` resolves and launches the repo's own configured session command from the worktree — the invoker never reads or composes another repo's session command.

4. **Dispatch the task** — once the session is up, send the single per-repo task into the window:

   ```sh
   tmux send-keys -t "<bulk-session>:<wt>-<repo>" "<task>" Enter
   ```

   **Exactly one slash command per agent** — no `&&`-joined strings. A spawned agent reads one leading `/command` per prompt (fab-operator §6).

5. **Enroll with the operator** — when running inside a fab operator session, enroll the window per fab-operator §6 enrollment: record pane/repo/session/branch in the operator state file, add the `branch_map` entry, and prefix the window name via `fab pane window-name ensure-prefix <pane> »`.

## Conventions

- **One tmux session per bulk task.** Create a dedicated session (default name `bulk-<task-slug>`, where `<task-slug>` is a short kebab-case name for the op) and open every per-repo window in it. Grouping the windows as a unit keeps them visible to the operator via `fab pane map --all-sessions`.
- **Repo-name window suffix.** Each window is named `<wt>-<repo>` (e.g. `swift-fox-hop`) so every window is self-identifying at a glance.

## Operator tracking

Bulk skills generally run inside a fab operator session, and the operator MUST continue tracking the spawned windows through its normal loop rather than pausing:

- **Enroll each spawned window** per fab-operator §6 enrollment (per-repo loop step 5): operator state-file entry, `branch_map` entry, `»` window prefix.
- **Resume ticking immediately** after the spawn sweep. Do NOT block waiting on the spawned agents — completions, prompts, and failures are picked up on later ticks like any other monitored agent.
- **Outside an operator session**, enrollment is skipped; the session-per-task and `<wt>-<repo>` conventions keep the windows discoverable via `fab pane map --all-sessions`.
