# cli — Memory Index

Top-level command surface of the `shll` binary: the cobra root, the six user-facing subcommands (`install`, `update`, `shell-init`, `shell-setup`, `version`, `list`) plus the hidden `help-dump`, and the hardcoded tool roster they share. (`shell-setup` is the canonical name for the rc-file installer; `shell-install` is retained as a back-compat alias.)

| Memory File | Description |
|-------------|-------------|
| [commands](commands.md) | Root command, subcommand wiring, exit-code sentinels (`errSilent`, `errExitCode`), version ldflags injection, and the hardcoded `Roster` slice. |
| [install](install.md) | `shll install` — brew detection, bootstrap of missing roster tools via `brew install`, idempotent re-run. |
| [update](update.md) | `shll update` — brew detection, installed-tool filtering, sequential `brew upgrade`, exit-code aggregation. |
| [shell-init](shell-init.md) | `shll shell-init <shell>` — composition rules across roster tools, eval-safety invariants, deterministic ordering. |
| [shell-setup](shell-setup.md) | `shll shell-setup [shell]` (alias `shell-install`) — sentinel-wrapped rc-file block, idempotent install/`--print`/`--uninstall`, `--trust-tap` ceremony. |
| [version](version.md) | `shll version` — column-aligned plain-text table, per-tool 2s timeout, ldflags-injected `shll` version; also hosts the shared `toolInstalled`/`probeToolVersion` install probe. |
| [list](list.md) | `shll list` — toolkit roster with install status, descriptions, and repo links; aligned table + `--json`; reuses the shared `toolInstalled` probe; the `rk`/`run-kit` repo-slug footgun. |
| [help-dump-contract](help-dump-contract.md) | Hidden `shll help-dump` subcommand — the frozen `help/<tool>.json` JSON contract (shared 7-tool, `wt.json` reference) and producer rules (programmatic cobra walk, filter completion/help/Hidden, prune-before-render). |
