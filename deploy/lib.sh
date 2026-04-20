# shellcheck shell=bash
# shellboto installer — shared helpers, sourced by install.sh + uninstall.sh.
# Not executable on its own. Bash 4+ (associative arrays, [[ ]], read -rs).

# ---------------------------------------------------------------------------
# Color output
# ---------------------------------------------------------------------------

# Auto-enable color when stdout is a TTY AND NO_COLOR is unset AND TERM isn't
# "dumb". Respect the environment — piping output to a log file strips codes.
color_init() {
    if [[ -t 1 ]] && [[ -z "${NO_COLOR:-}" ]] && [[ "${TERM:-}" != "dumb" ]]; then
        C_RESET=$'\033[0m'
        C_BOLD=$'\033[1m'
        C_DIM=$'\033[2m'
        C_RED=$'\033[31m'
        C_GREEN=$'\033[32m'
        C_YELLOW=$'\033[33m'
        C_BLUE=$'\033[34m'
        C_CYAN=$'\033[36m'
    else
        C_RESET=''; C_BOLD=''; C_DIM=''; C_RED=''
        C_GREEN=''; C_YELLOW=''; C_BLUE=''; C_CYAN=''
    fi
}

# Render a one-line heading in a rounded box. Total visible width =
# `width` chars (default 58): 1 side bar + (width-2) content + 1 side bar.
title() {
    local text="$1"
    local width=${2:-58}
    local inner
    inner=$(printf "%-*s" "$((width - 2))" " $text")
    printf "%s╭%s╮%s\n" "$C_BOLD$C_CYAN" "$(_dash "$((width - 2))")" "$C_RESET"
    printf "%s│%s%s%s│%s\n" "$C_BOLD$C_CYAN" "$C_RESET" "$inner" "$C_BOLD$C_CYAN" "$C_RESET"
    printf "%s╰%s╯%s\n" "$C_BOLD$C_CYAN" "$(_dash "$((width - 2))")" "$C_RESET"
}

_dash() {
    local n=$1
    local s=""
    local i
    for (( i = 0; i < n; i++ )); do s="$s─"; done
    printf "%s" "$s"
}

section()   { printf "\n%s→ %s%s\n"   "$C_BOLD$C_BLUE" "$1" "$C_RESET"; }
step()      { printf "\n%s[%d/%d] %s%s\n" "$C_BOLD$C_CYAN" "$1" "$2" "$3" "$C_RESET"; }
ok()        { printf "  %s✓%s %s\n"   "$C_GREEN" "$C_RESET" "$*"; }
warn()      { printf "  %s⚠%s %s\n"   "$C_YELLOW" "$C_RESET" "$*"; }
fail()      { printf "  %s✗%s %s\n"   "$C_RED"   "$C_RESET" "$*" >&2; }
info()      { printf "  %s%s%s\n"      "$C_DIM"   "$*" "$C_RESET"; }
hint()      { printf "  %s• %s%s\n"    "$C_DIM"   "$*" "$C_RESET"; }
die()       { fail "$*"; trap - ERR; exit 1; }

# bail exits non-zero AFTER you've already printed your own context
# (fail + hints). Without this, `exit 1` would fire the ERR trap's
# generic "error on line X" message — noise on top of an intentional
# failure. Accepts an optional exit code; defaults to 1.
# shellcheck disable=SC2120  # optional arg; callers usually pass none
bail()      { trap - ERR; exit "${1:-1}"; }

# ---------------------------------------------------------------------------
# Input
# ---------------------------------------------------------------------------

# prompt VAR "label" ["default"]  — plain visible read with optional default.
prompt() {
    local var="$1" label="$2" default="${3:-}" value
    local suffix=""
    if [[ -n "$default" ]]; then suffix=" [$default]"; fi
    while true; do
        printf "  %s?%s %s%s: " "$C_BLUE" "$C_RESET" "$label" "$suffix"
        IFS= read -r value || die "stdin closed"
        if [[ -z "$value" && -n "$default" ]]; then
            value="$default"
        fi
        if [[ -n "$value" ]]; then
            printf -v "$var" '%s' "$value"
            return 0
        fi
        warn "empty input, try again"
    done
}

# prompt_secret VAR "label" — input hidden, not echoed, not in bash history.
# Reads with `read -rs` so the token never lands in `/proc/<pid>/cmdline`,
# `ps`, or the shell history file.
prompt_secret() {
    local var="$1" label="$2" value
    while true; do
        printf "  %s?%s %s (input hidden): " "$C_BLUE" "$C_RESET" "$label"
        IFS= read -rs value || die "stdin closed"
        printf "\n"
        if [[ -n "$value" ]]; then
            printf -v "$var" '%s' "$value"
            # Wipe the local var; the caller's variable still holds it.
            value=""
            return 0
        fi
        warn "empty input, try again"
    done
}

# prompt_yn VAR "question" ["Y"|"N"]
# Stores 1 for yes, 0 for no.
prompt_yn() {
    local var="$1" question="$2" default="${3:-Y}" ans
    local suffix
    if [[ "$default" == "Y" ]]; then suffix="[Y/n]"; else suffix="[y/N]"; fi
    while true; do
        printf "  %s?%s %s %s: " "$C_BLUE" "$C_RESET" "$question" "$suffix"
        IFS= read -r ans || die "stdin closed"
        if [[ -z "$ans" ]]; then ans="$default"; fi
        case "$ans" in
            [Yy]|[Yy][Ee][Ss]) printf -v "$var" '%s' "1"; return 0 ;;
            [Nn]|[Nn][Oo])     printf -v "$var" '%s' "0"; return 0 ;;
            *) warn "enter y or n" ;;
        esac
    done
}

# prompt_choice VAR "question" "opt1" "opt2" …
# Numbered menu; default is always option 1. Blank Enter picks the default.
prompt_choice() {
    local var="$1" question="$2"
    shift 2
    local options=("$@")
    local i label=""
    for (( i = 0; i < ${#options[@]}; i++ )); do
        if (( i == 0 )); then
            label+="[$((i + 1))] ${options[i]} (default)  "
        else
            label+="[$((i + 1))] ${options[i]}  "
        fi
    done
    printf "  %s%s%s\n" "$C_DIM" "$label" "$C_RESET"
    local ans
    while true; do
        printf "  %s?%s %s [1]: " "$C_BLUE" "$C_RESET" "$question"
        IFS= read -r ans || die "stdin closed"
        if [[ -z "$ans" ]]; then ans="1"; fi
        if [[ "$ans" =~ ^[0-9]+$ ]] && (( ans >= 1 && ans <= ${#options[@]} )); then
            printf -v "$var" '%s' "${options[$((ans - 1))]}"
            return 0
        fi
        warn "enter a number between 1 and ${#options[@]}"
    done
}

# prompt_typed "phrase" "reason"
# Requires the user to type `phrase` verbatim. Used for irreversible ops
# (deleting the audit DB) — prevents yes-by-reflex. Returns 0 on match.
prompt_typed() {
    local phrase="$1" reason="$2" ans
    printf "  %s⚠%s %s\n" "$C_YELLOW" "$C_RESET" "$reason"
    printf "  %s?%s Type %s'%s'%s to confirm: " \
        "$C_BLUE" "$C_RESET" "$C_BOLD" "$phrase" "$C_RESET"
    IFS= read -r ans || die "stdin closed"
    [[ "$ans" == "$phrase" ]]
}

# ---------------------------------------------------------------------------
# Validation
# ---------------------------------------------------------------------------

# Telegram bot tokens look like "123456789:AAbbCC…" — numeric ID, colon,
# 30+ URL-safe chars. Loose enough that @BotFather token-format tweaks
# don't break us; tight enough that a pasted URL or obviously-wrong
# string gets caught.
validate_token() {
    [[ "$1" =~ ^[0-9]+:[A-Za-z0-9_-]{30,}$ ]]
}

validate_int_positive() {
    [[ "$1" =~ ^[1-9][0-9]*$ ]]
}

validate_hex32() {
    [[ "$1" =~ ^[0-9a-fA-F]{64}$ ]]
}

# ---------------------------------------------------------------------------
# Environment / prerequisites
# ---------------------------------------------------------------------------

need_root() {
    if (( EUID != 0 )); then
        die "this script must run as root — try 'sudo $0'"
    fi
}

# need_cmd cmd1 cmd2 … — each must be in PATH or we die.
need_cmd() {
    local cmd missing=()
    for cmd in "$@"; do
        command -v "$cmd" >/dev/null 2>&1 || missing+=("$cmd")
    done
    if (( ${#missing[@]} > 0 )); then
        die "missing required commands: ${missing[*]}"
    fi
}

# detect_systemd — die with hints if systemctl isn't available.
detect_systemd() {
    if ! command -v systemctl >/dev/null 2>&1; then
        fail "systemctl not found — shellboto ships a systemd unit"
        hint "if you use OpenRC / runit / s6, re-run with --skip-systemd"
        hint "and write your own service file for /usr/local/bin/shellboto"
        exit 1
    fi
}

# ---------------------------------------------------------------------------
# File operations
# ---------------------------------------------------------------------------

# backup_file PATH — if PATH exists, copy to PATH.bak.YYYYMMDD-HHMMSS.
# Echoes the backup path on stdout when a backup was made; nothing
# otherwise. Callers can capture: local bk; bk=$(backup_file "$p")
backup_file() {
    local path="$1"
    [[ -e "$path" ]] || return 0
    local ts backup
    ts=$(date -u +%Y%m%d-%H%M%S)
    backup="${path}.bak.${ts}"
    cp -a "$path" "$backup"
    printf "%s" "$backup"
}

# install_file SRC DST MODE [OWNER]
# Atomic install: write to DST.tmp, chmod, chown if given, rename.
install_file() {
    local src="$1" dst="$2" mode="$3" owner="${4:-}"
    local tmp="${dst}.tmp.$$"
    if $DRY_RUN; then
        info "(dry-run) install $src → $dst (mode $mode${owner:+, owner $owner})"
        return 0
    fi
    install -m "$mode" "$src" "$tmp"
    if [[ -n "$owner" ]]; then
        chown "$owner" "$tmp"
    fi
    mv "$tmp" "$dst"
}

# install_dir PATH MODE [OWNER]
install_dir() {
    local path="$1" mode="$2" owner="${3:-}"
    if $DRY_RUN; then
        info "(dry-run) mkdir -p $path (mode $mode${owner:+, owner $owner})"
        return 0
    fi
    install -d -m "$mode" "$path"
    if [[ -n "$owner" ]]; then
        chown "$owner" "$path"
    fi
}

# read_env_value FILE KEY — echoes the current value for KEY= in an env
# file, or nothing if the key is absent. Handles leading whitespace and
# quoted values. Does NOT evaluate shell syntax — pure text scrape.
read_env_value() {
    local file="$1" key="$2"
    [[ -f "$file" ]] || return 0
    # Last occurrence wins (matches `source`-style behavior).
    local line
    line=$(grep -E "^[[:space:]]*${key}=" "$file" | tail -n 1 || true)
    if [[ -z "$line" ]]; then return 0; fi
    # Strip `KEY=` prefix and any surrounding quotes.
    local val="${line#*=}"
    val="${val#\"}"; val="${val%\"}"
    val="${val#\'}"; val="${val%\'}"
    printf "%s" "$val"
}

# write_env_value FILE KEY VALUE
# Sets KEY=VALUE in the env file. If KEY already exists, its line is
# replaced in place. If not, KEY=VALUE is appended. Atomic via temp +
# rename. Perms preserved.
write_env_value() {
    local file="$1" key="$2" value="$3"
    if $DRY_RUN; then
        info "(dry-run) set $key in $file"
        return 0
    fi
    local tmp
    tmp=$(mktemp "${file}.XXXXXX")
    chmod 0600 "$tmp"
    local have_key=0
    if [[ -f "$file" ]]; then
        while IFS= read -r line; do
            if [[ "$line" =~ ^[[:space:]]*${key}= ]]; then
                printf '%s=%s\n' "$key" "$value" >> "$tmp"
                have_key=1
            else
                printf '%s\n' "$line" >> "$tmp"
            fi
        done < "$file"
    fi
    if (( have_key == 0 )); then
        printf '%s=%s\n' "$key" "$value" >> "$tmp"
    fi
    mv "$tmp" "$file"
    chmod 0600 "$file"
}

# ---------------------------------------------------------------------------
# systemctl helpers
# ---------------------------------------------------------------------------

sctl() {
    if $DRY_RUN; then
        info "(dry-run) systemctl $*"
        return 0
    fi
    systemctl "$@"
}

service_is_active() {
    systemctl is-active --quiet shellboto 2>/dev/null
}

service_exists() {
    [[ -e /etc/systemd/system/shellboto.service ]] \
        || systemctl cat shellboto >/dev/null 2>&1
}

# ---------------------------------------------------------------------------
# Rollback stack
# ---------------------------------------------------------------------------
# Each register_rollback adds an "undo" command (as a string eval'd in the
# failure handler). The stack unwinds LIFO. Commands should be idempotent
# (safe to no-op if the original action hadn't happened yet).

ROLLBACK_STACK=()

register_rollback() {
    ROLLBACK_STACK+=("$1")
}

run_rollback() {
    local i
    if (( ${#ROLLBACK_STACK[@]} == 0 )); then return; fi
    fail "running rollback (${#ROLLBACK_STACK[@]} action(s))"
    for (( i = ${#ROLLBACK_STACK[@]} - 1; i >= 0; i-- )); do
        info "  rollback: ${ROLLBACK_STACK[i]}"
        eval "${ROLLBACK_STACK[i]}" || true
    done
    ROLLBACK_STACK=()
}

clear_rollback() {
    ROLLBACK_STACK=()
}

# install_setup_traps — call once from each script. Assumes DRY_RUN is
# set to true/false by the caller.
install_setup_traps() {
    trap '_on_err $LINENO $?' ERR
    trap '_on_exit $?' EXIT
}

_on_err() {
    local line=$1 code=$2
    fail "error on line $line (exit $code)"
    run_rollback
    exit "$code"
}

_on_exit() {
    local code=$1
    # TMPDIR_ROOT is optional — scripts that create their own temp dir set it.
    if [[ -n "${TMPDIR_ROOT:-}" && -d "$TMPDIR_ROOT" ]]; then
        rm -rf "$TMPDIR_ROOT"
    fi
    return "$code"
}

# ---------------------------------------------------------------------------
# Misc
# ---------------------------------------------------------------------------

# human_bytes N — formats a byte count like "34.1 MiB".
human_bytes() {
    local n=$1
    if (( n < 1024 )); then
        printf "%d B" "$n"
        return
    fi
    awk -v n="$n" 'BEGIN {
        units[1] = "KiB"; units[2] = "MiB"; units[3] = "GiB"
        units[4] = "TiB"; units[5] = "PiB"
        u = 1; v = n / 1024
        while (v >= 1024 && u < 5) { v = v / 1024; u = u + 1 }
        printf "%.1f %s", v, units[u]
    }'
}

# Refuse direct execution. When sourced, ${BASH_SOURCE[0]} is this file
# but $0 is the caller script; when executed directly, they're equal.
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    echo "deploy/lib.sh is a library, not a script — source it from install.sh or uninstall.sh" >&2
    exit 1
fi
