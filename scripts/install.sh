#!/bin/sh
# sahil87 toolkit bootstrap — served at https://shll.ai/install
#
#   curl -fsSL https://shll.ai/install | sh                # install everything
#   curl -fsSL https://shll.ai/install | sh -s -- hop wt   # install a subset
#
# This is a *thin* bootstrap: it only solves the circularity that shll cannot
# trust/install its own Homebrew formula before it exists. Everything else —
# the tool roster, subset filtering, per-formula trust for the other tools,
# graceful skips — lives in `shll install`, which this script execs into.
#
# The whole body is wrapped in main() and only run via `main "$@"` on the last
# line, so a truncated download (a partial pipe) defines nothing runnable.
set -eu

main() {
    if ! command -v brew >/dev/null 2>&1; then
        echo "Homebrew is required but was not found." >&2
        echo "Install it from https://brew.sh, then re-run this script." >&2
        exit 1
    fi

    if ! command -v shll >/dev/null 2>&1; then
        echo "shll not found — installing sahil87/tap/shll via Homebrew..."
        # Homebrew 6.0+ requires a persisted trust record before a tap formula's
        # sandboxed install may run. Detect the trust subcommand by capability
        # probe (never a version-floor check) — pre-6.0 brews have no `brew
        # trust` and no trust requirement, so skip the step silently there.
        if brew trust --help >/dev/null 2>&1; then
            brew trust --formula sahil87/tap/shll
        fi
        brew install sahil87/tap/shll
    fi

    # Hand off to shll for the rest of the roster. All args pass through as the
    # install subset (e.g. `hop wt`); `shll install` validates the names itself.
    exec shll install "$@"
}

main "$@"
