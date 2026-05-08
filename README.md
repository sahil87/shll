# shll

> Part of [@sahil87's open source toolkit](https://ai.shll.in) — see all projects there.

`shll` is a meta-CLI for the sahil87 toolkit. It composes operations that span every per-tool CLI (`hop`, `wt`, `fab-kit`, `rk`, `tu`, `idea`) so you have one entry point for cross-toolkit concerns.

Three subcommands today:

| Command | Purpose |
|---------|---------|
| `shll update` | `brew update` then `brew upgrade` for every installed sahil87 tool |
| `shll shell-init <shell>` | Single eval-safe shell-init blob composed from all installed sahil87 tools that expose one |
| `shll version` | Versions of `shll` itself plus every installed sahil87 tool, plain text, paste-friendly for bug reports |

Per-tool CLIs continue to work standalone — `shll` wraps them, it does not replace them.

## Install

### Homebrew (macOS and Linux)

```sh
brew install sahil87/tap/shll
```

`shll` is also installed transitively via the `all` meta-formula:

```sh
brew install sahil87/tap/all   # installs every sahil87 tool, including shll
```

### From source

```sh
git clone https://github.com/sahil87/shll.git
cd shll
just local-install
```

Builds the binary and copies it to `~/.local/bin/shll`. Make sure that directory is on your `$PATH`.

## Usage

### Update every installed sahil87 tool

```sh
shll update
```

Runs `brew update --quiet` once, then `brew upgrade sahil87/tap/<formula>` for every roster tool that's currently installed. Uninstalled tools are skipped silently. Brew's progress streams directly to your terminal.

### Single-line shell integration

Replace per-tool eval lines with one composed line in your rc file:

```sh
eval "$(shll shell-init zsh)"   # in ~/.zshrc
eval "$(shll shell-init bash)"  # in ~/.bashrc
```

The output is the concatenation (in roster order) of every installed sahil87 tool's own shell-init. Today, `hop` and `wt` are the only roster tools with shell integration. Per-tool `hop shell-init` and `wt shell-setup` continue to work standalone if you'd rather wire them up individually.

### Print versions

```sh
shll version
```

Plain-text table — one row per tool, easy to paste into a bug report:

```
shll      v0.1.0
fab-kit   v0.4.2
rk        v0.7.1
tu        v0.2.0
hop       v0.0.3
wt        v0.1.5
idea      not installed
```

## Reference

- `shll --help` — full subcommand listing
- [`fab/project/constitution.md`](fab/project/constitution.md) — non-negotiable design constraints (security, no state, composition over absorption, tool roster as source of truth)
- [`fab/project/context.md`](fab/project/context.md) — repo shape, tech stack, tool roster
