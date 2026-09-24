#!/bin/sh
# Tests scripts/install.sh's persistence of brew's shellenv line
# (persist_brew_shellenv) — the fix for "zsh: command not found: shll" in every
# new shell after a fresh-Homebrew bootstrap, placed so it never jumps ahead
# of run-kit's tmux guard shims ("rk doctor: [FAIL] tmux-guard shim").
#
# Portable POSIX sh, no GNU-only tools, so it runs unchanged on macOS (BSD
# userland, /bin/sh = bash 3.2) and Linux (dash). Needs a real brew somewhere
# (on PATH or at a standard prefix) for the "new shell" checks.
#
#   sh scripts/test-install-rc.sh              # functions under sh
#   TEST_SH=dash sh scripts/test-install-rc.sh # functions under another shell
set -eu

here=$(cd "$(dirname "$0")" && pwd)
test_sh=${TEST_SH:-sh}
work=$(mktemp -d)
trap 'chmod -R u+w "$work"; rm -rf "$work"' EXIT

# The functions without the trailing `main "$@"` — the same cut the
# hexokit.com deploy makes before appending its composition tail.
if [ "$(tail -n 1 "$here/install.sh")" != 'main "$@"' ]; then
    echo "FAIL: install.sh no longer ends in 'main \"\$@\"'; update this test's cut" >&2
    exit 1
fi
sed '$d' "$here/install.sh" >"$work/fns.sh"

brew_bin=$(command -v brew 2>/dev/null || true)
if [ -z "$brew_bin" ]; then
    for c in /opt/homebrew/bin/brew /usr/local/bin/brew /home/linuxbrew/.linuxbrew/bin/brew; do
        if [ -x "$c" ]; then brew_bin=$c; break; fi
    done
fi
if [ -z "$brew_bin" ]; then
    echo "FAIL: no brew found (needed for the new-shell checks)" >&2
    exit 1
fi

# Where the line is expected (brew_rc_file): zsh → .zshenv on Linux,
# .zprofile on macOS; bash → .bashrc on Linux, .bash_profile on macOS.
if [ "$(uname -s)" = Darwin ]; then
    zf=.zprofile brc=.bash_profile
else
    zf=.zshenv brc=.bashrc
fi

fails=0
pass() { echo "ok   - $1"; }
fail() { echo "FAIL - $1" >&2; fails=$((fails + 1)); }

# persist <home> <login-shell> [zdotdir] — run persist_brew_shellenv under
# $test_sh with set -eu (as the installer does); stdout+stderr → $work/out.
persist() {
    HOME=$1 SHELL=$2 ZDOTDIR=${3:-} "$test_sh" -c \
        'set -eu; [ -n "$ZDOTDIR" ] || unset ZDOTDIR; . "$0"; persist_brew_shellenv "$1"; echo __continued__' \
        "$work/fns.sh" "$brew_bin" >"$work/out" 2>&1
}
count() { grep -c "$1" "$2" 2>/dev/null || true; }
newhome() { rm -rf "${work:?}/$1"; mkdir -p "$work/$1"; printf '%s\n' "$work/$1"; }
line="eval \"\$($brew_bin shellenv)\""
shll_block='# >>> shll >>>
eval "$(shll shell-init zsh)"
# <<< shll <<<'
guard_block='# >>> rk tmux guard >>>
export PATH="$HOME/.local/share/rk/shims:$PATH"
# <<< rk tmux guard <<<'

# 1. Existing file without a trailing newline: appended on its own line.
h=$(newhome t1); printf 'export A=1' >"$h/$zf"
persist "$h" /bin/zsh
if [ "$(sed -n 1p "$h/$zf")" = 'export A=1' ] && [ "$(tail -n 1 "$h/$zf")" = "$line" ]; then
    pass "zsh: appends to ~/$zf (fixes a missing trailing newline)"
else fail "zsh: append to ~/$zf"; cat "$h/$zf" >&2; fi

# 2. rk tmux guard block present (rk agent setup ran before) → line lands
#    ABOVE it, file mode kept.
h=$(newhome t2); printf 'export A=1\n%s\n' "$guard_block" >"$h/$zf"; chmod 600 "$h/$zf"
persist "$h" /bin/zsh
brew_ln=$(grep -n 'brew shellenv' "$h/$zf" | cut -d: -f1)
guard_ln=$(grep -n '^# >>> rk tmux guard >>>' "$h/$zf" | cut -d: -f1)
mode=$(ls -l "$h/$zf" | cut -c1-10)
if [ -n "$brew_ln" ] && [ "$brew_ln" -lt "$guard_ln" ] && [ "$mode" = "-rw-------" ]; then
    pass "zsh: inserts above an rk tmux guard block, keeps file mode"
else fail "zsh: insert above rk guard (mode $mode)"; cat "$h/$zf" >&2; fi

# 3. Idempotent: a second run adds nothing.
persist "$h" /bin/zsh
if [ "$(count 'brew shellenv' "$h/$zf")" = 1 ]; then pass "idempotent re-run"
else fail "idempotent re-run"; fi

# 4. bash: shll block and rk guard in the same file → above the FIRST of them.
h=$(newhome t4); printf 'x\n%s\n%s\n' "$shll_block" "$guard_block" >"$h/$brc"
persist "$h" /bin/bash
if [ "$(sed -n 3p "$h/$brc")" = "$line" ] && [ "$(sed -n 5p "$h/$brc")" = '# >>> shll >>>' ]; then pass "bash: inserts above the first toolkit block in ~/$brc"
else fail "bash: first block in ~/$brc"; cat "$h/$brc" >&2; fi

# 5. Legacy `# >>> shll shell-init >>>` block is also recognized.
h=$(newhome t5); printf '# >>> shll shell-init >>>\neval "$(shll shell-init bash)"\n# <<< shll shell-init <<<\n' >"$h/$brc"
persist "$h" /bin/bash
if [ "$(sed -n 2p "$h/$brc")" = "$line" ]; then pass "bash: inserts above a legacy shll block"
else fail "bash: legacy block"; cat "$h/$brc" >&2; fi

# 6. Fresh zsh account: no startup files at all → the line's file is
#    created, and an empty .zshrc is created for `shll setup shell` (which
#    refuses to create one).
h=$(newhome t6)
persist "$h" /bin/zsh
if [ "$(tail -n 1 "$h/$zf" 2>/dev/null)" = "$line" ] && [ -f "$h/.zshrc" ] && [ ! -s "$h/.zshrc" ]; then
    pass "zsh: fresh account gets ~/$zf with the line and an empty ~/.zshrc"
else fail "zsh: fresh account"; ls -A "$h" >&2; fi

# 7. ZDOTDIR is honored, $HOME untouched.
h=$(newhome t7); mkdir -p "$h/z"
persist "$h" /bin/zsh "$h/z"
if grep -q 'brew shellenv' "$h/z/$zf" && [ ! -e "$h/$zf" ] && [ ! -e "$h/.zshrc" ]; then pass "zsh: honors ZDOTDIR"
else fail "zsh: ZDOTDIR"; fi

# 8. Symlinked file (dotfile manager): link survives, target is edited.
h=$(newhome t8); mkdir -p "$h/dot"; printf '%s\n' "$guard_block" >"$h/dot/f"; ln -s dot/f "$h/$zf"
persist "$h" /bin/zsh
if [ -L "$h/$zf" ] && grep -q 'brew shellenv' "$h/dot/f"; then pass "zsh: keeps a symlinked ~/$zf a symlink"
else fail "zsh: symlinked file"; fi

# 9. Unsupported shell: nothing written, the line is printed instead.
h=$(newhome t9)
persist "$h" /usr/bin/fish
if [ -z "$(ls -A "$h")" ] && grep -q 'add this line to your shell startup file' "$work/out"; then
    pass "other shell: prints the line, writes nothing"
else fail "other shell"; fi

# 10. Unwritable file: never fatal under set -eu, prints the line, no raw shell noise.
for variant in plain block; do
    h=$(newhome "t10$variant"); : >"$h/.zshrc"
    if [ "$variant" = block ]; then printf '%s\n' "$guard_block" >"$h/$zf"; else printf 'x\n' >"$h/$zf"; fi
    chmod 400 "$h/$zf"
    persist "$h" /bin/zsh
    if grep -q __continued__ "$work/out" && grep -q 'could not be updated' "$work/out" &&
        ! grep -qi 'permission denied' "$work/out"; then
        pass "unwritable file ($variant): falls back to printing, installer continues"
    else fail "unwritable file ($variant)"; cat "$work/out" >&2; fi
done

# 11. The real point, in a brand-new shell with a bare PATH (a fresh terminal):
#     brew resolves, sourcing the shll-style rc raises no "command not found",
#     and — on Linux, where the line shares .zshenv/.bashrc with rk's guard —
#     the rk shims dir stays AHEAD of brew's bin (the tmux-guard doctor check).
#     macOS terminals start login shells; Linux terminals start non-login ones.
bare=/usr/bin:/bin
brew_dir=$(dirname "$brew_bin")
if [ "$(uname -s)" = Darwin ]; then flags=-il; else flags=-i; fi
for sh_name in zsh bash; do
    sh_bin=$(command -v "$sh_name" 2>/dev/null || true)
    [ -n "$sh_bin" ] || { echo "skip - $sh_name not installed"; continue; }
    h=$(newhome "t11$sh_name")
    if [ "$sh_name" = zsh ]; then
        persist "$h" "$sh_bin"
        # What `rk agent setup` then `shll setup shell` append after it.
        printf '%s\n' "$guard_block" >>"$h/.zshenv"
        printf 'command -v brew >/dev/null || echo "command not found: brew"\n' >>"$h/.zshrc"
    else
        persist "$h" "$sh_bin"
        printf '%s\n' "$guard_block" >>"$h/$brc"
        printf 'command -v brew >/dev/null || echo "command not found: brew"\n' >>"$h/$brc"
    fi
    got=$(cd "$h" && env -i HOME="$h" TERM=dumb PATH="$bare" "$sh_bin" $flags \
        -c 'command -v brew; printf "PATH=%s\n" "$PATH"' 2>&1 </dev/null)
    if echo "$got" | grep -q 'command not found'; then
        fail "new $sh_name terminal: command not found"; echo "$got" >&2; continue
    fi
    if echo "$got" | grep -qx "$brew_bin"; then pass "new $sh_name terminal resolves brew"
    else fail "new $sh_name terminal resolves brew"; echo "$got" >&2; fi
    path_line=$(echo "$got" | grep '^PATH=' | tail -n 1)
    shims_pos=$(echo "$path_line" | tr ':=' '\n\n' | grep -n -x "$h/.local/share/rk/shims" | head -n 1 | cut -d: -f1)
    brew_pos=$(echo "$path_line" | tr ':=' '\n\n' | grep -n -x "$brew_dir" | head -n 1 | cut -d: -f1)
    if [ "$(uname -s)" = Darwin ] && [ "$sh_name" = zsh ]; then
        # .zprofile runs after rk's .zshenv guard — run-kit's documented
        # login-profile limitation, not something this line can order around.
        echo "skip - macOS zsh: rk guard (.zshenv) vs brew (.zprofile) order is run-kit's documented limitation"
    elif [ -n "$shims_pos" ] && [ -n "$brew_pos" ] && [ "$shims_pos" -lt "$brew_pos" ]; then
        pass "new $sh_name terminal keeps the rk shims ahead of brew"
    else fail "new $sh_name terminal keeps the rk shims ahead of brew (shims=$shims_pos brew=$brew_pos)"; echo "$path_line" >&2; fi
done

echo
if [ "$fails" -ne 0 ]; then
    echo "$fails failure(s)" >&2
    exit 1
fi
echo "all passed"
