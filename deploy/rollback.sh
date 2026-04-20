#!/usr/bin/env bash
# shellboto rollback — swap the installed binary with its .prev backup.
#
# install.sh saves the previous binary at /usr/local/bin/shellboto.prev
# every time it upgrades. This script swaps current ↔ prev: re-run it
# to flip back. Safe, reversible, idempotent-by-toggle.
#
# Usage:
#   sudo ./deploy/rollback.sh        # interactive
#   sudo ./deploy/rollback.sh -y     # non-interactive (CI / Ansible)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# shellcheck source=deploy/lib.sh
source "$SCRIPT_DIR/lib.sh"

# ---------------------------------------------------------------------------
# Flags
# ---------------------------------------------------------------------------

YES=false
DRY_RUN=false
PREFIX=""

usage() {
    cat <<EOF
shellboto rollback — swap the installed binary with its .prev backup.

Usage: sudo $0 [flags]

  -y, --yes         non-interactive (skip the confirm prompt)
      --dry-run     print every action without making changes
      --prefix DIR  operate on a prefix instead of /
  -h, --help        show this help

Rollback is reversible: the installer keeps your previous binary at
/usr/local/bin/shellboto.prev on every upgrade, and rollback swaps it
with the current one. Running rollback twice returns to the original.
EOF
}

while (( $# > 0 )); do
    case "$1" in
        -y|--yes)    YES=true; shift ;;
        --dry-run)   DRY_RUN=true; shift ;;
        --prefix)    PREFIX="$2"; shift 2 ;;
        -h|--help)   usage; exit 0 ;;
        *)           die "unknown flag: $1  (try --help)" ;;
    esac
done

color_init
install_setup_traps

BIN_PATH="${PREFIX}/usr/local/bin/shellboto"
BIN_PREV="${BIN_PATH}.prev"

# ---------------------------------------------------------------------------
# Header + prereqs
# ---------------------------------------------------------------------------

title "shellboto rollback"
if $DRY_RUN;            then warn "dry-run mode — no changes will be made"; fi
if [[ -n "$PREFIX" ]];  then info "operating on prefix: $PREFIX"; fi

section "Prerequisites"
need_root;        ok "root privileges"
detect_systemd
ok "systemd $(systemctl --version | awk 'NR==1 {print $2}')"

# ---------------------------------------------------------------------------
# Inspect current state
# ---------------------------------------------------------------------------

section "State"
if [[ ! -f "$BIN_PATH" ]]; then
    die "no current binary at $BIN_PATH — shellboto is not installed"
fi
if [[ ! -f "$BIN_PREV" ]]; then
    fail "no previous binary at $BIN_PREV"
    hint "install.sh saves the predecessor on upgrades, not on the first install."
    hint "nothing to roll back to until a second install creates one."
    bail
fi

# Best-effort version probes — binaries that can't -version degrade to "unknown".
curr_ver=$("$BIN_PATH" -version 2>/dev/null | awk 'NR==1 {print $2}' || true)
prev_ver=$("$BIN_PREV"  -version 2>/dev/null | awk 'NR==1 {print $2}' || true)
curr_ver=${curr_ver:-unknown}
prev_ver=${prev_ver:-unknown}

ok "current:  $BIN_PATH  v$curr_ver"
ok "previous: $BIN_PREV  v$prev_ver"

# ---------------------------------------------------------------------------
# Confirm
# ---------------------------------------------------------------------------

if ! $YES; then
    confirm=0
    prompt_yn confirm "Swap?  current ↔ previous" "Y"
    if (( confirm != 1 )); then
        info "cancelled"
        exit 0
    fi
fi

# ---------------------------------------------------------------------------
# Swap
# ---------------------------------------------------------------------------

section "Swap"

was_active=false
if service_is_active; then
    was_active=true
    sctl stop shellboto
    register_rollback "systemctl start shellboto || true"
    ok "systemctl stop shellboto"
fi

SWAP="${BIN_PATH}.swap.$$"
if $DRY_RUN; then
    info "(dry-run) mv $BIN_PATH $SWAP"
    info "(dry-run) mv $BIN_PREV $BIN_PATH"
    info "(dry-run) mv $SWAP $BIN_PREV"
else
    # Three atomic rename(2) calls. The service is already stopped above,
    # so the transient "$BIN_PATH doesn't exist" window between mv #1 and
    # mv #2 is invisible to systemd. register_rollback entries unwind any
    # partial state if SIGINT or an error fires mid-swap.
    mv "$BIN_PATH" "$SWAP"
    register_rollback "mv '$SWAP' '$BIN_PATH' 2>/dev/null || true"

    mv "$BIN_PREV" "$BIN_PATH"
    register_rollback "mv '$BIN_PATH' '$BIN_PREV' 2>/dev/null || true"

    mv "$SWAP" "$BIN_PREV"
    # Clear unwinders now that the swap succeeded.
    clear_rollback
fi
ok "binaries swapped  (now: $BIN_PATH is v${prev_ver}; $BIN_PREV is v${curr_ver})"

if $was_active; then
    sctl start shellboto
    if ! $DRY_RUN; then
        for _ in 1 2 3 4 5 6 7 8; do
            if service_is_active; then break; fi
            sleep 0.5
        done
        if service_is_active; then
            pid=$(systemctl show -p MainPID --value shellboto)
            ok "systemctl start shellboto  (active, pid $pid)"
        else
            fail "service did not become active after rollback"
            hint "the rolled-back binary (now at $BIN_PATH) may have its own runtime issue"
            hint "to revert the rollback: sudo $0"
            bail
        fi
    else
        ok "systemctl start shellboto (dry-run)"
    fi
fi

# ---------------------------------------------------------------------------
# Done
# ---------------------------------------------------------------------------

echo
title "done"
info ""
info "  Rolled back from v${curr_ver} → v${prev_ver}"
info "  Previous version kept at $BIN_PREV — run this script again to swap back."
echo
