#!/usr/bin/env bash
# shellboto uninstaller — safe by default.
#
# Removes the binary and systemd unit. Does NOT touch the state DB or
# config files unless explicitly asked — the audit chain is irreplaceable.
# Deleting the state DB requires a typed confirmation phrase that names
# the DB size, so you cannot yes-by-reflex your way into losing audit history.
#
# Usage:
#   sudo ./deploy/uninstall.sh                   # interactive
#   sudo ./deploy/uninstall.sh -y                # non-interactive (keeps data)
#   sudo ./deploy/uninstall.sh --remove-config --remove-state \
#        --i-understand-this-deletes-audit-log -y    # full wipe (CI)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# shellcheck source=deploy/lib.sh
source "$SCRIPT_DIR/lib.sh"

# ---------------------------------------------------------------------------
# Flags
# ---------------------------------------------------------------------------

YES=false
DRY_RUN=false
REMOVE_CONFIG=false
REMOVE_STATE=false
I_UNDERSTAND=false
PREFIX=""

usage() {
    cat <<EOF
shellboto uninstaller

Usage: sudo $0 [flags]

By default, removes the binary and systemd unit only. Config and state
(audit DB) are preserved; pass flags below to remove them.

Flags:
  -y, --yes                                non-interactive
      --remove-config                      also remove /etc/shellboto/
      --remove-state                       also remove /var/lib/shellboto/ (audit DB + all history)
      --i-understand-this-deletes-audit-log
                                           required alongside --remove-state in -y mode
      --prefix DIR                         operate on a prefix instead of /
      --dry-run                            print every action; don't modify the system
  -h, --help                               show this help
EOF
}

while (( $# > 0 )); do
    case "$1" in
        -y|--yes)                              YES=true; shift ;;
        --remove-config)                       REMOVE_CONFIG=true; shift ;;
        --remove-state)                        REMOVE_STATE=true; shift ;;
        --i-understand-this-deletes-audit-log) I_UNDERSTAND=true; shift ;;
        --prefix)                              PREFIX="$2"; shift 2 ;;
        --dry-run)                             DRY_RUN=true; shift ;;
        -h|--help)                             usage; exit 0 ;;
        *)                                     die "unknown flag: $1  (try --help)" ;;
    esac
done

color_init
install_setup_traps

BIN_PATH="${PREFIX}/usr/local/bin/shellboto"
ETC_DIR="${PREFIX}/etc/shellboto"
STATE_DIR="${PREFIX}/var/lib/shellboto"
UNIT_PATH="${PREFIX}/etc/systemd/system/shellboto.service"

# ---------------------------------------------------------------------------
# Header
# ---------------------------------------------------------------------------

title "shellboto uninstaller"
if $DRY_RUN;    then warn "dry-run mode — no changes will be made"; fi
if [[ -n "$PREFIX" ]]; then info "operating on prefix: $PREFIX"; fi

need_root

# ---------------------------------------------------------------------------
# Inspect current state
# ---------------------------------------------------------------------------

section "Current state"

has_service=false
has_binary=false
has_config=false
has_state=false
state_size=0
state_rows_users=""
state_rows_audit=""

if service_exists; then
    has_service=true
    if service_is_active; then
        pid=$(systemctl show -p MainPID --value shellboto 2>/dev/null || echo "?")
        ok "service running  (pid $pid)"
    else
        ok "service installed  (inactive)"
    fi
else
    info "service not installed"
fi

if [[ -x "$BIN_PATH" ]]; then
    has_binary=true
    ver=$("$BIN_PATH" -version 2>/dev/null | awk 'NR==1 {print $2}' || true)
    ok "binary            $BIN_PATH  ${ver:+v$ver}"
else
    info "binary not installed at $BIN_PATH"
fi

if [[ -d "$ETC_DIR" ]]; then
    has_config=true
    cfg_count=$(find "$ETC_DIR" -maxdepth 1 -type f 2>/dev/null | wc -l || echo 0)
    ok "config            $ETC_DIR/  ($cfg_count file(s))"
else
    info "no config dir at $ETC_DIR"
fi

if [[ -f "$STATE_DIR/state.db" ]]; then
    has_state=true
    state_size=$(stat -c %s "$STATE_DIR/state.db" 2>/dev/null || echo 0)
    human=$(human_bytes "$state_size")
    # Try to get row counts via shellboto db info if binary is still present.
    if $has_binary && [[ -f "$ETC_DIR/config.toml" || -f "$ETC_DIR/config.yaml" || -f "$ETC_DIR/config.json" ]]; then
        cfg_file=""
        for ext in toml yaml yml json; do
            if [[ -f "${ETC_DIR}/config.${ext}" ]]; then
                cfg_file="${ETC_DIR}/config.${ext}"; break
            fi
        done
        if [[ -n "$cfg_file" && -f "$ETC_DIR/env" ]]; then
            # Source env for SHELLBOTO_TOKEN — shellboto requires it to
            # open the DB even for read-only ops.
            set -a
            # shellcheck disable=SC1090,SC1091
            source "$ETC_DIR/env" 2>/dev/null || true
            set +a
            info_out=$("$BIN_PATH" db info -config "$cfg_file" 2>/dev/null || true)
            state_rows_users=$(echo "$info_out"  | awk '/^rows\(users\)/        {print $2}')
            state_rows_audit=$(echo "$info_out"  | awk '/^rows\(audit_events\)/ {print $2}')
        fi
    fi
    ok "state DB          $STATE_DIR/state.db  ($human)"
    if [[ -n "$state_rows_users" || -n "$state_rows_audit" ]]; then
        info "                  ${state_rows_users:-?} users, ${state_rows_audit:-?} audit rows"
    fi
else
    info "no state DB at $STATE_DIR/state.db"
fi

if ! $has_service && ! $has_binary && ! $has_config && ! $has_state; then
    die "nothing to remove — shellboto is not installed"
fi

# ---------------------------------------------------------------------------
# Decide what to remove
# ---------------------------------------------------------------------------

section "What to remove"

do_bin=1
if $has_service || $has_binary; then
    if ! $YES; then
        prompt_yn do_bin "Remove systemd unit and binary?" "Y"
    fi
fi

do_config=0
if $has_config; then
    if $REMOVE_CONFIG; then
        do_config=1
    elif ! $YES; then
        prompt_yn do_config "Remove config files under $ETC_DIR/?" "N"
    fi
fi

do_state=0
if $has_state; then
    if $REMOVE_STATE; then
        # In -y mode, --remove-state alone is insufficient — also need
        # --i-understand-this-deletes-audit-log so a careless shell
        # one-liner can't erase the audit log.
        if $YES && ! $I_UNDERSTAND; then
            die "--remove-state in -y mode also requires --i-understand-this-deletes-audit-log"
        fi
        do_state=1
    elif ! $YES; then
        prompt_yn do_state "Remove state DB and audit log under $STATE_DIR/?" "N"
        if (( do_state == 1 )); then
            human=$(human_bytes "$state_size")
            phrase="DELETE-SHELLBOTO-${human// /}"   # e.g. DELETE-SHELLBOTO-34.1MiB
            if ! prompt_typed "$phrase" "This removes the audit chain. IRREVERSIBLE."; then
                warn "phrase did not match — keeping state DB"
                do_state=0
            fi
        fi
    fi
fi

if (( do_bin == 0 && do_config == 0 && do_state == 0 )); then
    echo
    info "nothing selected for removal — exiting"
    exit 0
fi

# ---------------------------------------------------------------------------
# Remove
# ---------------------------------------------------------------------------

section "Removing"

if (( do_bin == 1 )); then
    if $has_service; then
        if service_is_active; then sctl stop shellboto; ok "systemctl stop shellboto"; fi
        if systemctl is-enabled --quiet shellboto 2>/dev/null; then
            sctl disable shellboto
            ok "systemctl disable shellboto"
        fi
        if [[ -e "$UNIT_PATH" ]]; then
            if $DRY_RUN; then
                info "(dry-run) rm $UNIT_PATH"
            else
                rm -f "$UNIT_PATH"
            fi
            ok "removed $UNIT_PATH"
            sctl daemon-reload
            ok "systemctl daemon-reload"
        fi
    fi
    if $has_binary; then
        if $DRY_RUN; then
            info "(dry-run) rm $BIN_PATH"
        else
            rm -f "$BIN_PATH"
        fi
        ok "removed $BIN_PATH"
    fi
fi

if (( do_config == 1 )); then
    if $DRY_RUN; then
        info "(dry-run) rm -rf $ETC_DIR"
    else
        rm -rf "$ETC_DIR"
    fi
    ok "removed $ETC_DIR"
elif $has_config; then
    info "$ETC_DIR/ kept  (pass --remove-config to delete next time)"
fi

if (( do_state == 1 )); then
    human=$(human_bytes "$state_size")
    if $DRY_RUN; then
        info "(dry-run) rm -rf $STATE_DIR"
    else
        rm -rf "$STATE_DIR"
    fi
    ok "removed $STATE_DIR  ($human freed)"
elif $has_state; then
    info "$STATE_DIR/ kept  (audit chain preserved)"
fi

clear_rollback

# ---------------------------------------------------------------------------
# Done
# ---------------------------------------------------------------------------

echo
title "done"
info ""
info "  shellboto removed."
if (( do_config == 0 )) && $has_config; then
    info "  Config kept at $ETC_DIR/ for easy reinstall."
fi
if (( do_state == 0 )) && $has_state; then
    info "  Audit DB kept at $STATE_DIR/state.db."
fi
echo
