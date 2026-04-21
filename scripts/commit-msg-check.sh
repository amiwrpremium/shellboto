#!/usr/bin/env bash
# Validate a commit message against the Conventional Commits 1.0 spec.
# Wired as a lefthook `commit-msg` hook; also re-used by CI to check
# every commit on a PR.
#
# Usage:
#   scripts/commit-msg-check.sh <path-to-commit-msg-file>
#
# Exit 0 on valid message, 1 on rejected.

set -euo pipefail

if [[ $# -lt 1 ]]; then
    echo "usage: $0 <commit-msg-file>" >&2
    exit 2
fi

COMMIT_FILE="$1"
COMMIT_MSG=$(head -n 1 "$COMMIT_FILE")

# Allow mechanical commits through without checking.
case "$COMMIT_MSG" in
    Merge\ *|Revert\ *|fixup!\ *|squash!\ *|amend!\ *)
        exit 0
        ;;
esac

# Conventional Commits 1.0:
#   <type>(<scope>)!: <description>
# <type>:  one of feat|fix|docs|style|refactor|perf|test|build|ci|chore|revert
# <scope>: optional, lowercase identifier + hyphen. Multiple parenthesised
#          scope groups allowed so dependabot's `chore(ci)(deps): …` form
#          (where the repo-set prefix and dependabot's own `(deps)` stack)
#          passes without rewriting every existing PR.
# !:       optional, marks a breaking change
# <description>: 1-72 chars after ": "
TYPE_RE='(feat|fix|docs|style|refactor|perf|test|build|ci|chore|revert)'
SCOPE_RE='(\([a-z0-9-]+\))*'
BREAK_RE='!?'
REGEX="^${TYPE_RE}${SCOPE_RE}${BREAK_RE}: .{1,72}$"

if [[ "$COMMIT_MSG" =~ $REGEX ]]; then
    exit 0
fi

cat >&2 <<EOF
✗ commit message does not follow Conventional Commits

  got:      ${COMMIT_MSG}

  expected: <type>(<scope>)!: <description>
            type  ∈ feat|fix|docs|style|refactor|perf|test|build|ci|chore|revert
            scope ∈ lowercase identifier (optional)
            !     marks a breaking change (optional)
            description: 1-72 chars

  examples:
    feat: add audit replay subcommand
    feat(shell): PROMPT_COMMAND-based boundary signalling
    fix(audit): trim trailing newlines from command output
    fix(audit)!: drop legacy sentinel column

  spec: https://www.conventionalcommits.org/en/v1.0.0/

EOF
exit 1
