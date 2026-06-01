# shll

> Part of [@sahil87's open source toolkit](https://shll.ai) — see all projects there.

One command to install, update, and shell-wire every tool in the [@sahil87 toolkit](https://shll.ai) (`fab-kit`, `rk`, `tu`, `hop`, `wt`, `idea`). `shll` doesn't replace the per-tool CLIs — it composes them.

## Why shll?

- **One-shot install** — `shll install` runs `brew install sahil87/tap/<formula>` for every roster tool you don't already have. Idempotent and safe to re-run.
- **One-line shell integration** — `shll shell-install` appends a single eval line to your rc file that wires up `hop`, `wt`, and any future toolkit shell-init in one block. No more managing four eval lines.
- **One update for everything** — `shll update` runs `brew update` once, then upgrades every installed roster tool in sequence. Skips ones you don't have. Skips itself if it wasn't installed via brew.
- **Paste-friendly version dump** — `shll version` prints one row per tool, ideal for bug reports.

Per-tool CLIs continue to work standalone — `shll` wraps them, it does not replace them.

## Quick start

From a clean machine to a fully wired toolkit:

```sh
brew install sahil87/tap/shll   # or: brew install sahil87/tap/all
shll install                    # brew-installs every roster tool you're missing
shll shell-install              # appends the composed eval block to your rc file
exec $SHELL                     # reload so the shell integration takes effect
```

That's it. `hop`, `wt`, and the other tools are now installed and their shell integration is live.

## Install

```sh
brew install sahil87/tap/shll
```

`shll` is also installed transitively via the `all` meta-formula (`brew install sahil87/tap/all`), which pulls in every roster tool at once.

### From source

```sh
git clone https://github.com/sahil87/shll.git
cd shll
just install
```

Builds the binary and copies it to `~/.local/bin/shll`. Make sure that directory is on your `$PATH`.

## Commands

### `shll install` — bootstrap missing tools

```sh
shll install
```

Iterates the roster (`fab-kit`, `rk`, `tu`, `hop`, `wt`, `idea`) and runs `brew install sahil87/tap/<formula>` for each one that's missing. Already-installed tools are skipped silently. Does NOT upgrade — use `shll update` for that.

### `shll update` — upgrade everything

```sh
shll update
```

Runs `brew update --quiet` once, then `brew upgrade sahil87/tap/shll` (when shll itself was installed via brew), then `brew upgrade sahil87/tap/<formula>` for every roster tool currently installed. Brew's progress streams directly to your terminal.

### `shll shell-install` — wire the rc file (recommended)

```sh
shll shell-install              # auto-detect shell, append eval block to your rc file
shll shell-install --print      # dry-run: print the block to stdout, modify nothing
shll shell-install --uninstall  # clean removal of the block
shll shell-install --rc-file ~/.zshrc.local   # override the target path
```

The appended block is sentinel-wrapped and idempotent — re-running is a no-op when the line is already present:

```sh
# >>> shll shell-init >>>
eval "$(shll shell-init zsh)"
# <<< shll shell-init <<<
```

The rc file is opened with plain `O_APPEND`, so dotfile-manager symlinks (chezmoi, dotbot, stow, yadm) are preserved. Default targets: `${ZDOTDIR:-$HOME}/.zshrc` for zsh, `$HOME/.bash_profile` (macOS) or `$HOME/.bashrc` (Linux) for bash.

### `shll shell-init <shell>` — composed shell-init

If you'd rather wire the eval line by hand, this is what `shell-install` writes to your rc file:

```sh
eval "$(shll shell-init zsh)"   # in ~/.zshrc
eval "$(shll shell-init bash)"  # in ~/.bashrc
```

The output is the concatenation (in roster order) of every installed sahil87 tool's own shell-init. What each roster tool contributes:

| Tool | What it adds to your shell |
|------|----------------------------|
| `hop` | `hop` shell function (bare-name `cd`, verb dispatch, tool-form), `h` / `hi` aliases, completion |
| `wt`  | `wt` shell function wrapper (so the "Open here" menu option can `cd` your shell), completion |
| `tu`  | completion |
| `idea` | completion |
| `rk`  | completion |
| `fab-kit` | completion |

`hop` and `wt` are the only tools that ship *shell functions* — those need eval-time installation because a function defined inside the binary can't escape into the parent shell. Everything else is completion, which the shell sources lazily on tab. Per-tool `<tool> shell-init <shell>` continues to work standalone if you'd rather wire them up individually.

### `shll version` — paste-friendly version dump

```sh
$ shll version
shll     v0.0.5
fab-kit  v1.9.4
rk       v1.5.3
tu       v0.4.13
hop      v0.1.5
wt       v0.0.5
idea     v0.0.2
```

One row per tool. Uninstalled tools render as `not installed`. Drop the whole block into a bug report.

## How composition works

shll has no state, no database, and no special knowledge of the tools it wraps. Every subcommand is a thin coordinator over the per-tool CLIs:

| `shll` command | What it actually runs |
|----------------|------------------------|
| `shll install` | `brew install sahil87/tap/<formula>` per missing tool |
| `shll update` | `brew update`, then `brew upgrade sahil87/tap/<formula>` per installed tool |
| `shll shell-init zsh` | concatenates the stdout of each installed tool's `<tool> shell-init zsh` |
| `shll version` | invokes `<tool> --version` per tool, formats as a table |

Per Constitution Principle IV (Composition, Not Replacement): `hop update`, `wt shell-init`, etc. continue to work standalone. shll's only job is to fan-out, collect output, and degrade gracefully when a tool is missing.

## Reference

- `shll --help` — full subcommand listing
- Per-tool repos for the wrapped CLIs:
  [fab-kit](https://github.com/sahil87/fab-kit) ·
  [run-kit](https://github.com/sahil87/run-kit) ·
  [tu](https://github.com/sahil87/tu) ·
  [hop](https://github.com/sahil87/hop) ·
  [wt](https://github.com/sahil87/wt) ·
  [idea](https://github.com/sahil87/idea)
