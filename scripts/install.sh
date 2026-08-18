#!/bin/sh
# shll toolkit bootstrap — served at https://shll.ai/install
#
#   curl -fsSL https://shll.ai/install | sh                # install everything
#   curl -fsSL https://shll.ai/install | sh -s -- hop wt   # install a subset
#
# This script owns the whole pre-brew phase: it preflights the tools the
# install needs (git/CLT, curl, tmux), bootstraps Homebrew headlessly when
# it's absent, and then solves the circularity that shll cannot trust/install
# its own Homebrew formula before it exists. Everything else — the tool
# roster, subset filtering, per-formula trust for the other tools, graceful
# skips — lives in `shll install`, which this script execs into.
#
# The whole body is wrapped in functions and only run via `main "$@"` on the
# last line, so a truncated download (a partial pipe) defines nothing
# runnable.
set -eu

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
    preflight

    # Bootstrap Homebrew headlessly when absent. brew.sh is bash-only, so the
    # (POSIX sh) bootstrap invokes it via /bin/bash — present on fresh macOS
    # and Ubuntu images. Proven headless on macOS (including the CLT install
    # via softwareupdate) and on Linux (the preflight above has already
    # guaranteed git, brew.sh's Linux prerequisite). Under `set -eu` a failing
    # installer aborts the script with the installer's own error output.
    bootstrapped=0
    if ! command -v brew >/dev/null 2>&1; then
        echo "Homebrew not found — installing it with the official installer (NONINTERACTIVE=1)..."
        NONINTERACTIVE=1 /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
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
        # brew onto this process's PATH before the exec hand-off — otherwise
        # `exec shll install` fails with command-not-found on every bootstrap
        # run.
        eval "$("$BREW" shellenv)"

        # Kill the brew-not-on-PATH trap for the user's *next* shell too:
        # brew's shellenv line is the user's to keep (shll shell-setup wires
        # shll's own init, not brew's).
        echo "Homebrew installed. To make brew (and the installed tools) resolvable in future shells,"
        echo "add this line to your shell rc file:"
        echo "  eval \"\$($BREW shellenv)\""
    fi

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

    # Hand off to shll for the rest of the roster. All args pass through as the
    # install subset (e.g. `hop wt`); `shll install` validates the names itself.
    exec shll install "$@"
}

main "$@"
