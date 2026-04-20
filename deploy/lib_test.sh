#!/usr/bin/env bash
# Unit tests for deploy/lib.sh — covers the pure validation helpers and
# env-file IO. Does not cover prompts (they need a TTY) or install_file
# (covered by an actual prefix install in CI).

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Required by install_file / install_dir in lib.sh. We don't actually
# call those in the unit tests, but lib.sh references $DRY_RUN.
DRY_RUN=false

# shellcheck source=deploy/lib.sh
source "$SCRIPT_DIR/lib.sh"

fails=0
assert_ok()   { if "$@"; then :; else echo "FAIL: assertion true failed:  $*" >&2; fails=$((fails + 1)); fi }
assert_fail() { if "$@"; then echo "FAIL: assertion false failed: $*" >&2; fails=$((fails + 1)); fi }
assert_eq() {
    if [[ "$1" != "$2" ]]; then
        echo "FAIL: expected '$2', got '$1'" >&2
        fails=$((fails + 1))
    fi
}

# ---- validate_token -------------------------------------------------------
assert_ok   validate_token "123456789:AAbbCCddEEffGGhhIIjjKKllMMnnOOpp-_"
assert_ok   validate_token "7:ABCDEFGHIJKLMNOPQRSTUVWXYZ_0123456789-abc"
assert_fail validate_token ""
assert_fail validate_token "no-colon-here-and-too-short"
assert_fail validate_token "123456789"                            # no colon
assert_fail validate_token "123:short"                            # token body too short
assert_fail validate_token "abc:AAbbCCddEEffGGhhIIjjKKllMMnnOOpp" # non-numeric ID
assert_fail validate_token "https://api.telegram.org/bot123:abc"  # URL

# ---- validate_int_positive ------------------------------------------------
assert_ok   validate_int_positive "1"
assert_ok   validate_int_positive "9876543210"
assert_fail validate_int_positive ""
assert_fail validate_int_positive "0"        # reject zero (Telegram IDs are > 0)
assert_fail validate_int_positive "-5"
assert_fail validate_int_positive "12.3"
assert_fail validate_int_positive "abc"
assert_fail validate_int_positive "01"       # leading zero rejected (unambiguous)

# ---- validate_hex32 -------------------------------------------------------
assert_ok   validate_hex32 "0000000000000000000000000000000000000000000000000000000000000000"
assert_ok   validate_hex32 "ABCDEFabcdef0123456789abcdef0123456789abcdef0123456789abcdef0123"
assert_fail validate_hex32 ""
assert_fail validate_hex32 "short"
assert_fail validate_hex32 "00000000000000000000000000000000000000000000000000000000000000000"  # 65 chars
assert_fail validate_hex32 "zz00000000000000000000000000000000000000000000000000000000000000"  # non-hex

# ---- read_env_value / write_env_value -------------------------------------
tmp=$(mktemp)
trap 'rm -f "$tmp"' EXIT

# Missing key → empty
assert_eq "$(read_env_value "$tmp" FOO)" ""

# Write and read back
write_env_value "$tmp" FOO "bar"
assert_eq "$(read_env_value "$tmp" FOO)" "bar"

# Overwrite (not append)
write_env_value "$tmp" FOO "baz"
count=$(grep -c '^FOO=' "$tmp")
assert_eq "$count" "1"
assert_eq "$(read_env_value "$tmp" FOO)" "baz"

# Multiple keys coexist
write_env_value "$tmp" BAR "qux"
assert_eq "$(read_env_value "$tmp" FOO)" "baz"
assert_eq "$(read_env_value "$tmp" BAR)" "qux"

# Values with `=` in them preserved
write_env_value "$tmp" URL "https://example.com/path?a=1&b=2"
assert_eq "$(read_env_value "$tmp" URL)" "https://example.com/path?a=1&b=2"

# ---- human_bytes ----------------------------------------------------------
assert_eq "$(human_bytes 512)"        "512 B"
assert_eq "$(human_bytes 1024)"       "1.0 KiB"
assert_eq "$(human_bytes 1048576)"    "1.0 MiB"
assert_eq "$(human_bytes 35651584)"   "34.0 MiB"    # 34 * 1024 * 1024
assert_eq "$(human_bytes 1073741824)" "1.0 GiB"

# ---- backup_file ----------------------------------------------------------
tmp2=$(mktemp)
trap 'rm -f "$tmp" "$tmp2" "$tmp2".bak.*' EXIT
echo "original" > "$tmp2"
bk=$(backup_file "$tmp2")
assert_ok test -f "$bk"
assert_eq "$(cat "$bk")" "original"

# ---- Summary --------------------------------------------------------------
if (( fails > 0 )); then
    echo "FAIL  $fails assertion(s) failed"
    exit 1
fi
echo "OK  all assertions passed"
