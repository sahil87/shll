#!/bin/sh
# shll toolkit bootstrap — served at https://shll.ai/install
#
#   curl -fsSL https://shll.ai/install | sh                # converge everything
#   curl -fsSL https://shll.ai/install | sh -s -- hop wt   # converge a subset
#
# This script owns the whole pre-brew phase: it preflights the tools the
# install needs (git/CLT, curl, tmux), bootstraps Homebrew headlessly when
# it's absent, and then solves the circularity that shll cannot trust/install
# its own Homebrew formula before it exists. Everything else — the tool
# roster, subset filtering, per-formula trust for the other tools, graceful
# skips — lives in `shll install` / `shll update`, which this script hands
# off to.
#
# The hand-off converges the machine to *complete and current*: `shll
# install "$@"` fills the gaps (all args verbatim — tool names and flags),
# then `exec shll update <tools>` upgrades the already-installed tools.
# Only tool names ride the update pass — install-only flags like
# --no-agent-setup are filtered out (generic `-*` match). Updating runs
# each tool's own update contract — side effects included, e.g. run-kit
# restarting its daemon. Freshly installed tools are cheap no-op updates.
# A failed install stops the bootstrap (`set -e`), so the update pass
# never runs over a broken install.
#
# The whole body is wrapped in functions and only run via `main "$@"` on the
# last line, so a truncated download (a partial pipe) defines nothing
# runnable.
set -eu

# Phase-line helpers: one colored `→`/`✓` line as each of the script's three
# phases (preflight → brew bootstrap → shll handoff) starts/finishes. Color
# only on a stdout tty with NO_COLOR unset; piped output gets the plain
# glyph lines with zero escape sequences. Kept dumb by design — no scroll
# regions, no percentages.
color_on() {
    [ -t 1 ] && [ -z "${NO_COLOR:-}" ]
}

phase_start() {
    if color_on; then
        printf '\033[36m→ %s\033[0m\n' "$1"
    else
        printf '→ %s\n' "$1"
    fi
}

phase_done() {
    if color_on; then
        printf '\033[32m✓ %s\033[0m\n' "$1"
    else
        printf '✓ %s\n' "$1"
    fi
}

# OSC 9;4 progress hint (state 3 = indeterminate, 0 = clear) — a single
# printf, tty-gated, harmless on non-supporting terminals.
osc_progress() {
    if [ -t 1 ]; then
        printf '\033]9;4;%s;0\033\\' "$1"
    fi
}

# Probe git, curl, and tmux BEFORE the Homebrew step and report every miss at
# once, each with a per-platform fix command — never fail on the first missing
# dep. On macOS the git probe is `xcode-select -p`, never `command -v git`:
# the CLT shim at /usr/bin/git false-positives when the Command Line Tools
# are not installed.
#
# Fatality matrix:
#   curl missing                     -> fatal (brew.sh and brew both need it)
#   git  missing, Linux, no brew     -> fatal (brew.sh's Linux prerequisite)
#   git  missing, macOS, brew present-> fatal (brew's git/tap operations need
#                                       the real CLT)
#   git  missing, macOS, no brew     -> informational only (the NONINTERACTIVE
#                                       brew.sh bootstrap below installs the
#                                       CLT itself via softwareupdate)
#   tmux missing                     -> warn-only, never fatal (run-kit runtime
#                                       dep, not an install prerequisite)
preflight() {
    os=$(uname -s)

    brew_present=0
    if command -v brew >/dev/null 2>&1; then
        brew_present=1
    fi

    git_ok=0
    curl_ok=0
    tmux_ok=0
    case "$os" in
        Darwin)
            if xcode-select -p >/dev/null 2>&1; then
                git_ok=1
            fi
            ;;
        *)
            if command -v git >/dev/null 2>&1; then
                git_ok=1
            fi
            ;;
    esac
    if command -v curl >/dev/null 2>&1; then
        curl_ok=1
    fi
    if command -v tmux >/dev/null 2>&1; then
        tmux_ok=1
    fi

    if [ "$git_ok" -eq 1 ] && [ "$curl_ok" -eq 1 ] && [ "$tmux_ok" -eq 1 ]; then
        return 0
    fi

    echo "Preflight found missing dependencies:" >&2
    if [ "$git_ok" -eq 0 ]; then
        case "$os" in
            Darwin)
                echo "  - git (Xcode Command Line Tools) — fix: xcode-select --install" >&2
                ;;
            *)
                echo "  - git — fix: sudo apt-get install -y git   (Debian/Ubuntu; use your package manager otherwise)" >&2
                ;;
        esac
    fi
    if [ "$curl_ok" -eq 0 ]; then
        case "$os" in
            Darwin)
                echo "  - curl — fix: install curl (it ships with macOS; a missing curl means a broken PATH)" >&2
                ;;
            *)
                echo "  - curl — fix: sudo apt-get install -y curl   (Debian/Ubuntu; use your package manager otherwise)" >&2
                ;;
        esac
    fi
    if [ "$tmux_ok" -eq 0 ]; then
        case "$os" in
            Darwin)
                echo "  - tmux — fix: brew install tmux   (once this run has put Homebrew in place)" >&2
                ;;
            *)
                echo "  - tmux — fix: sudo apt-get install -y tmux   (Debian/Ubuntu; or brew install tmux once Homebrew is in place)" >&2
                ;;
        esac
    fi

    fatal=0
    if [ "$curl_ok" -eq 0 ]; then
        fatal=1
    fi
    if [ "$git_ok" -eq 0 ]; then
        case "$os" in
            Darwin)
                if [ "$brew_present" -eq 1 ]; then
                    fatal=1
                else
                    echo "Note: the Homebrew bootstrap below installs the Command Line Tools (git) itself." >&2
                fi
                ;;
            *)
                if [ "$brew_present" -eq 0 ]; then
                    fatal=1
                fi
                ;;
        esac
    fi
    if [ "$fatal" -eq 1 ]; then
        echo "Fix the missing dependencies above and re-run this script." >&2
        exit 1
    fi
    # tmux is run-kit's runtime dependency, not an install prerequisite — the
    # warning above (with its fix command) is the whole treatment.
}

main() {
    phase_start "preflight"
    preflight
    phase_done "preflight"

    phase_start "brew bootstrap"
    # Bootstrap Homebrew headlessly when absent. brew.sh is bash-only, so the
    # (POSIX sh) bootstrap invokes it via /bin/bash — present on fresh macOS
    # and Ubuntu images. Proven headless on macOS (including the CLT install
    # via softwareupdate) and on Linux (the preflight above has already
    # guaranteed git, brew.sh's Linux prerequisite). Under `set -eu` a failing
    # installer aborts the script with the installer's own error output.
    bootstrapped=0
    if ! command -v brew >/dev/null 2>&1; then
        echo "Homebrew not found — installing it with the official installer (NONINTERACTIVE=1)..."
        osc_progress 3
        NONINTERACTIVE=1 /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
        osc_progress 0
        bootstrapped=1
    fi

    # Resolve the absolute brew path once and use it for every brew call: a
    # fresh brew install is not on PATH in this process.
    if [ "$bootstrapped" -eq 0 ]; then
        BREW=$(command -v brew)
    else
        BREW=""
        for candidate in \
            /opt/homebrew/bin/brew \
            /usr/local/bin/brew \
            /home/linuxbrew/.linuxbrew/bin/brew
        do
            if [ -x "$candidate" ]; then
                BREW=$candidate
                break
            fi
        done
        if [ -z "$BREW" ]; then
            echo "Homebrew bootstrap finished, but no brew executable was found at the known install prefixes" >&2
            echo "(/opt/homebrew/bin/brew, /usr/local/bin/brew, /home/linuxbrew/.linuxbrew/bin/brew)." >&2
            exit 1
        fi

        # The freshly installed shll will live in the brew prefix, so bring
        # brew onto this process's PATH before the shll hand-off — otherwise
        # `shll install` fails with command-not-found on every bootstrap
        # run.
        eval "$("$BREW" shellenv)"

        # Kill the brew-not-on-PATH trap for the user's *next* shell too:
        # brew's shellenv line is the user's to keep (shll shell-setup wires
        # shll's own init, not brew's).
        echo "Homebrew installed. To make brew (and the installed tools) resolvable in future shells,"
        echo "add this line to your shell rc file:"
        echo "  eval \"\$($BREW shellenv)\""
    fi
    phase_done "brew bootstrap"

    phase_start "shll handoff"
    if ! command -v shll >/dev/null 2>&1; then
        echo "shll not found — installing sahil87/tap/shll via Homebrew..."
        # Homebrew 6.0+ requires a persisted trust record before a tap formula's
        # sandboxed install may run. Detect the trust subcommand by capability
        # probe (never a version-floor check) — pre-6.0 brews have no `brew
        # trust` and no trust requirement, so skip the step silently there.
        if "$BREW" trust --help >/dev/null 2>&1; then
            "$BREW" trust --formula sahil87/tap/shll
        fi
        "$BREW" install sahil87/tap/shll
    fi

    # Hand off to shll for the rest of the roster — converge to complete and
    # current: install the missing tools, then upgrade the installed ones
    # (running each tool's own update contract, e.g. run-kit's daemon
    # restart; freshly installed tools are cheap no-op updates). ALL args
    # pass verbatim to `shll install` (tool names and flags — its
    # `--no-*` flag surface is public bootstrap surface); under `set -e` a
    # failed install stops the bootstrap and skips the update pass.
    shll install "$@"

    # Only tool names ride the update pass: install-only flags (e.g.
    # --no-agent-setup) are not `shll update` flags, so forward the
    # positional tool subset and drop every dash-prefixed arg. The filter
    # is a generic `-*` match — no flag-name knowledge enters the script.
    n=$#
    while [ "$n" -gt 0 ]; do
        arg=$1
        shift
        case "$arg" in
            -*) ;;
            *) set -- "$@" "$arg" ;;
        esac
        n=$((n - 1))
    done
    exec shll update "$@"
}

main "$@"
