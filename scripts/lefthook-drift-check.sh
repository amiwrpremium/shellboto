#!/usr/bin/env bash
# Warn if .lefthook.yml changed in the merged/checked-out range.
# Lefthook reloads its config on every invocation, but a newly added
# hook type (e.g., pre-rebase) still needs `lefthook install` to wire
# into .git/hooks. Catches the "I pulled and forgot to re-install hooks"
# case.
#
# Never blocks the merge/checkout; prints a yellow line on stderr and
# returns 0. Safe to call — missing reflog entries (fresh clone) are
# swallowed.
set -eu

# post-checkout: git passes PREV_HEAD NEW_HEAD BRANCH_FLAG. Only fire
# on branch checkouts (branch-flag=1); file checkouts get noisy.
# post-merge passes just SQUASH_FLAG, which we don't care about — for
# post-merge the HEAD@{1} diff below covers it.
if [ "${1:-}" = "0" ]; then
    exit 0
fi

# 'HEAD@{1}' is git reflog syntax, not shell brace expansion — quote it
# so shellcheck doesn't flag SC1083 and so the shell doesn't try to
# treat it as a glob.
if git diff --name-only 'HEAD@{1}' HEAD 2>/dev/null \
    | grep -qx '\.lefthook\.yml'; then
    printf '\033[33m⚠  .lefthook.yml changed — run "make hooks-install" to pick up any new hook types.\033[0m\n' >&2
fi
