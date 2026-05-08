# cli — Memory Index

Top-level command surface of the `shll` binary: the cobra root, the three v0.1.0 subcommands (`update`, `shell-init`, `version`), and the hardcoded tool roster they share.

| Memory File | Description |
|-------------|-------------|
| [commands](commands.md) | Root command, subcommand wiring, exit-code sentinels (`errSilent`, `errExitCode`), version ldflags injection, and the hardcoded `Roster` slice. |
| [update](update.md) | `shll update` — brew detection, installed-tool filtering, sequential `brew upgrade`, exit-code aggregation. |
| [shell-init](shell-init.md) | `shll shell-init <shell>` — composition rules across roster tools, eval-safety invariants, deterministic ordering. |
| [version](version.md) | `shll version` — column-aligned plain-text table, per-tool 2s timeout, ldflags-injected `shll` version. |
