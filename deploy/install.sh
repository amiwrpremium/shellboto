#!/usr/bin/env bash
# shellboto installer — interactive, idempotent, safe.
#
# Usage:
#   sudo ./deploy/install.sh                     # interactive
#   sudo ./deploy/install.sh -y \                # CI / Ansible mode
#        --superadmin-id 123456789
#   sudo ./deploy/install.sh --help              # all flags
#
# Reads SHELLBOTO_TOKEN / SHELLBOTO_SUPERADMIN_ID / SHELLBOTO_AUDIT_SEED
# from the environment in -y mode (alongside CLI flags). In interactive
# mode the token is read with `read -rs` (no echo, not in shell history).

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# shellcheck disable=SC1091 source=lib.sh
source "$SCRIPT_DIR/lib.sh"

# ---------------------------------------------------------------------------
# Flags
# ---------------------------------------------------------------------------

YES=false
DRY_RUN=false
SKIP_BUILD=false
SKIP_SYSTEMD=false
PREFIX=""
CONFIG_FORMAT=""        # toml|yaml|json — empty = prompt
SUPERADMIN_ID_ARG=""
AUDIT_SEED_ARG=""

usage() {
    cat <<EOF
shellboto installer

Usage: sudo $0 [flags]

Flags:
  -y, --yes                  non-interactive (requires env vars / flags below)
      --superadmin-id N      Telegram user ID of the superadmin
      --config-format FMT    toml | yaml | json  (default: toml)
      --audit-seed HEX       re-use a specific 32-byte hex seed (skip generation)
      --skip-build           use existing bin/shellboto (don't run 'make build')
      --skip-systemd         install files only; don't touch systemctl
      --prefix DIR           install to DIR instead of / (chroot / Docker builds)
      --dry-run              print every action; don't modify the system
  -h, --help                 show this help

In -y mode, SHELLBOTO_TOKEN must be passed via environment variable.
SHELLBOTO_AUDIT_SEED is auto-generated unless --audit-seed is given.
EOF
}

while (( $# > 0 )); do
    case "$1" in
        -y|--yes)                YES=true; shift ;;
        --superadmin-id)         SUPERADMIN_ID_ARG="$2"; shift 2 ;;
        --config-format)         CONFIG_FORMAT="$2"; shift 2 ;;
        --audit-seed)            AUDIT_SEED_ARG="$2"; shift 2 ;;
        --skip-build)            SKIP_BUILD=true; shift ;;
        --skip-systemd)          SKIP_SYSTEMD=true; shift ;;
        --prefix)                PREFIX="$2"; shift 2 ;;
        --dry-run)               DRY_RUN=true; shift ;;
        -h|--help)               usage; exit 0 ;;
        *)                       die "unknown flag: $1  (try --help)" ;;
    esac
done

color_init
install_setup_traps

# ---------------------------------------------------------------------------
# Paths (honor --prefix)
# ---------------------------------------------------------------------------

BIN_DIR="${PREFIX}/usr/local/bin"
BIN_PATH="${BIN_DIR}/shellboto"
ETC_DIR="${PREFIX}/etc/shellboto"
STATE_DIR="${PREFIX}/var/lib/shellboto"
SYSTEMD_DIR="${PREFIX}/etc/systemd/system"
ENV_PATH="${ETC_DIR}/env"
UNIT_PATH="${SYSTEMD_DIR}/shellboto.service"

# ---------------------------------------------------------------------------
# Header + prereqs
# ---------------------------------------------------------------------------

title "shellboto installer"
if $DRY_RUN; then warn "dry-run mode — no changes will be made"; fi
if [[ -n "$PREFIX" ]]; then info "installing into prefix: $PREFIX"; fi

section "Prerequisites"
need_root;        ok "root privileges"
need_cmd bash install mv cp awk grep sed openssl
ok "bash $(bash --version | awk 'NR==1 {print $4}')"
ok "openssl $(openssl version | awk '{print $2}')"

if ! $SKIP_SYSTEMD; then
    detect_systemd
    ok "systemd $(systemctl --version | awk 'NR==1 {print $2}')"
fi

if ! $SKIP_BUILD; then
    need_cmd go
    ok "go $(go version | awk '{print $3}' | sed 's/^go//')"
fi

# ---------------------------------------------------------------------------
# Step 1: build
# ---------------------------------------------------------------------------

step 1 7 "Build binary"
if $SKIP_BUILD; then
    [[ -x "$REPO_ROOT/bin/shellboto" ]] \
        || die "bin/shellboto missing — drop --skip-build or 'make build' first"
    ok "using existing $REPO_ROOT/bin/shellboto"
else
    if $DRY_RUN; then
        info "(dry-run) make -C $REPO_ROOT build"
    else
        make -C "$REPO_ROOT" build >/dev/null
    fi
    if [[ -x "$REPO_ROOT/bin/shellboto" ]]; then
        local_ver=$("$REPO_ROOT/bin/shellboto" -version 2>/dev/null | awk 'NR==1 {print $2}')
        ok "bin/shellboto built (version=${local_ver:-dev})"
    else
        $DRY_RUN || die "build failed — bin/shellboto not found after make"
        ok "bin/shellboto (dry-run)"
    fi
fi

# ---------------------------------------------------------------------------
# Step 2: install binary
# ---------------------------------------------------------------------------

step 2 7 "Install binary"
install_dir "$BIN_DIR" 0755
# Stop before replacing if running — avoids exec-text-busy on some kernels.
# On success, the systemd step below starts it again; if the script errors
# in between, the rollback trap restores the running state.
if ! $SKIP_SYSTEMD && service_is_active; then
    info "service active — stopping for binary replacement"
    sctl stop shellboto
    register_rollback "systemctl start shellboto || true"
fi
# INSTALL-1: before overwriting, copy the current binary to .prev so
# deploy/rollback.sh has something to swap back to if the new version
# turns out to be broken. cp -a preserves 0755 + root:root.
if [[ -f "$BIN_PATH" ]] && ! $DRY_RUN; then
    prev_ver=$("$BIN_PATH" -version 2>/dev/null | awk 'NR==1 {print $2}' || echo unknown)
    cp -a "$BIN_PATH" "${BIN_PATH}.prev"
    ok "saved current → ${BIN_PATH}.prev  (v${prev_ver})"
fi
install_file "$REPO_ROOT/bin/shellboto" "$BIN_PATH" 0755 "root:root"
ok "$BIN_PATH  (0755, root:root)"

# ---------------------------------------------------------------------------
# Step 3: directories
# ---------------------------------------------------------------------------

step 3 7 "Create directories"
install_dir "$ETC_DIR"   0700 "root:root"
ok "$ETC_DIR  (0700, root:root)"
install_dir "$STATE_DIR" 0700 "root:root"
ok "$STATE_DIR  (0700, root:root)"

# ---------------------------------------------------------------------------
# Step 4: configure environment (/etc/shellboto/env)
# ---------------------------------------------------------------------------

step 4 7 "Configure environment"

TOKEN=""
SUPERADMIN_ID=""
AUDIT_SEED=""
KEEP_EXISTING_ENV=0

if [[ -f "$ENV_PATH" ]]; then
    existing_token=$(read_env_value "$ENV_PATH" SHELLBOTO_TOKEN)
    existing_super=$(read_env_value "$ENV_PATH" SHELLBOTO_SUPERADMIN_ID)
    existing_seed=$(read_env_value "$ENV_PATH" SHELLBOTO_AUDIT_SEED)
    info "existing env file detected at $ENV_PATH"
    if [[ -n "$existing_token" && "$existing_token" != *REPLACE_ME* ]]; then
        hint "SHELLBOTO_TOKEN is set"
    else
        hint "SHELLBOTO_TOKEN is NOT set (or placeholder)"
    fi
    if [[ -n "$existing_super" ]]; then
        hint "SHELLBOTO_SUPERADMIN_ID = $existing_super"
    else
        hint "SHELLBOTO_SUPERADMIN_ID is NOT set"
    fi
    if [[ -n "$existing_seed" ]]; then
        hint "SHELLBOTO_AUDIT_SEED is set (${#existing_seed} chars)"
    else
        hint "SHELLBOTO_AUDIT_SEED is NOT set"
    fi

    if $YES; then
        KEEP_EXISTING_ENV=1
    else
        prompt_yn KEEP_EXISTING_ENV "Keep existing values (fill in missing ones only)?" "Y"
    fi

    if (( KEEP_EXISTING_ENV == 1 )); then
        TOKEN="$existing_token"
        SUPERADMIN_ID="$existing_super"
        AUDIT_SEED="$existing_seed"
    else
        bk=$(backup_file "$ENV_PATH")
        if [[ -n "$bk" ]]; then info "backed up existing env → $bk"; fi
    fi
fi

# Fill token.
if [[ -z "$TOKEN" || "$TOKEN" == *REPLACE_ME* ]]; then
    if $YES; then
        TOKEN="${SHELLBOTO_TOKEN:-}"
        [[ -n "$TOKEN" ]] || die "non-interactive mode: SHELLBOTO_TOKEN env var not set"
        validate_token "$TOKEN" || die "SHELLBOTO_TOKEN does not match Telegram bot format"
    else
        hint "find the bot token at @BotFather in Telegram (format: 123456789:ABC…)"
        attempts=0
        while true; do
            prompt_secret TOKEN "Bot token"
            if validate_token "$TOKEN"; then break; fi
            warn "that doesn't look like a Telegram bot token"
            attempts=$(( attempts + 1 ))
            (( attempts < 3 )) || die "token not provided after 3 attempts — edit $ENV_PATH and re-run"
        done
    fi
fi

# Fill superadmin ID.
if [[ -z "$SUPERADMIN_ID" ]]; then
    if $YES; then
        SUPERADMIN_ID="${SUPERADMIN_ID_ARG:-${SHELLBOTO_SUPERADMIN_ID:-}}"
        [[ -n "$SUPERADMIN_ID" ]] || die "non-interactive mode: pass --superadmin-id N or SHELLBOTO_SUPERADMIN_ID"
        validate_int_positive "$SUPERADMIN_ID" || die "--superadmin-id must be a positive integer"
    else
        hint "don't know your Telegram ID? message @userinfobot"
        attempts=0
        while true; do
            prompt SUPERADMIN_ID "Superadmin Telegram user ID"
            if validate_int_positive "$SUPERADMIN_ID"; then break; fi
            warn "must be a positive integer"
            attempts=$(( attempts + 1 ))
            (( attempts < 3 )) || die "superadmin ID not provided after 3 attempts"
        done
    fi
fi

# Fill audit seed — always auto-generate unless explicit override.
if [[ -z "$AUDIT_SEED" ]]; then
    if [[ -n "$AUDIT_SEED_ARG" ]]; then
        AUDIT_SEED="$AUDIT_SEED_ARG"
        validate_hex32 "$AUDIT_SEED" || die "--audit-seed must be 64 hex chars (32 bytes)"
        ok "using --audit-seed (${#AUDIT_SEED} hex chars)"
    elif $YES && [[ -n "${SHELLBOTO_AUDIT_SEED:-}" ]]; then
        AUDIT_SEED="$SHELLBOTO_AUDIT_SEED"
        validate_hex32 "$AUDIT_SEED" || die "SHELLBOTO_AUDIT_SEED env var must be 64 hex chars"
        ok "using SHELLBOTO_AUDIT_SEED from env"
    else
        AUDIT_SEED=$(openssl rand -hex 32)
        ok "SHELLBOTO_AUDIT_SEED generated (32 bytes)"
    fi
fi

# Seed a fresh env file from the example if missing.
if [[ ! -f "$ENV_PATH" ]]; then
    install_file "$REPO_ROOT/deploy/env.example" "$ENV_PATH" 0600 "root:root"
fi
write_env_value "$ENV_PATH" SHELLBOTO_TOKEN          "$TOKEN"
write_env_value "$ENV_PATH" SHELLBOTO_SUPERADMIN_ID  "$SUPERADMIN_ID"
write_env_value "$ENV_PATH" SHELLBOTO_AUDIT_SEED     "$AUDIT_SEED"
unset TOKEN AUDIT_SEED     # scrub the secrets from script scope
ok "$ENV_PATH  (0600, root:root)"

# ---------------------------------------------------------------------------
# Step 5: config file
# ---------------------------------------------------------------------------

step 5 7 "Install config"

existing_cfg=""
for ext in toml yaml yml json; do
    if [[ -f "${ETC_DIR}/config.${ext}" ]]; then
        existing_cfg="${ETC_DIR}/config.${ext}"
        break
    fi
done

keep_cfg=0
if [[ -n "$existing_cfg" ]]; then
    info "existing config at $existing_cfg"
    if $YES; then
        keep_cfg=1
    else
        prompt_yn keep_cfg "Keep existing config?" "Y"
    fi
fi

if (( keep_cfg == 1 )); then
    ok "$existing_cfg (kept)"
else
    fmt="$CONFIG_FORMAT"
    if [[ -z "$fmt" ]]; then
        if $YES; then
            fmt="toml"
        else
            prompt_choice fmt "Config format?" "toml" "yaml" "json"
        fi
    fi
    case "$fmt" in
        toml|yaml|json) ;;
        *) die "unsupported config format: $fmt (use toml|yaml|json)" ;;
    esac
    if [[ -n "$existing_cfg" ]]; then
        bk=$(backup_file "$existing_cfg")
        if [[ -n "$bk" ]]; then info "backed up $existing_cfg → $bk"; fi
    fi
    dst="${ETC_DIR}/config.${fmt}"
    install_file "$REPO_ROOT/deploy/config.example.${fmt}" "$dst" 0600 "root:root"
    ok "$dst  (0600, root:root)"
    # If the old config used a different extension, the unit file's
    # `-config` flag points at it. Make sure the unit is pointing at
    # the file we just installed.
    INSTALLED_CONFIG="$dst"
fi
INSTALLED_CONFIG="${INSTALLED_CONFIG:-$existing_cfg}"

# ---------------------------------------------------------------------------
# Step 6: systemd unit
# ---------------------------------------------------------------------------

if $SKIP_SYSTEMD; then
    step 6 7 "Systemd unit"
    info "skipped (--skip-systemd)"
else
    step 6 7 "Systemd unit"
    tmp_unit=$(mktemp)
    sed "s|/etc/shellboto/config.toml|${INSTALLED_CONFIG#"$PREFIX"}|g" \
        "$REPO_ROOT/deploy/shellboto.service" > "$tmp_unit"
    install_file "$tmp_unit" "$UNIT_PATH" 0644 "root:root"
    rm -f "$tmp_unit"
    ok "$UNIT_PATH"
    sctl daemon-reload;     ok "systemctl daemon-reload"
    sctl enable shellboto;  ok "systemctl enable shellboto"
    sctl start shellboto
    if $DRY_RUN; then
        ok "systemctl start shellboto (dry-run)"
    else
        # Give systemd a beat to activate.
        for _ in 1 2 3 4 5; do
            if service_is_active; then break; fi
            sleep 0.5
        done
        if service_is_active; then
            pid=$(systemctl show -p MainPID --value shellboto)
            ok "systemctl start shellboto  (active, pid $pid)"
        else
            fail "service did not become active"
            info "try: systemctl status shellboto  &&  journalctl -u shellboto -n 50"
            bail
        fi
    fi
    # We started cleanly; drop the "restart on rollback" entry.
    clear_rollback
fi

# ---------------------------------------------------------------------------
# Step 7: preflight
# ---------------------------------------------------------------------------

step 7 7 "Preflight"
if $DRY_RUN; then
    info "(dry-run) would run: shellboto doctor -config $INSTALLED_CONFIG"
elif [[ -n "$PREFIX" ]]; then
    info "skipped under --prefix (db_path would point at the real /var/lib/shellboto)"
else
    # Pull env so the doctor sees the same values the service does.
    set -a
    # shellcheck disable=SC1090
    source "$ENV_PATH"
    set +a
    if "$BIN_PATH" doctor -config "$INSTALLED_CONFIG"; then
        :
    else
        rc=$?
        warn "doctor reported check failures (exit $rc)"
        warn "the service may still be running — inspect and fix, then restart"
    fi
fi

# ---------------------------------------------------------------------------
# Done
# ---------------------------------------------------------------------------

echo
title "done"
info ""
if $SKIP_SYSTEMD; then
    info "  shellboto installed. Wire up your init system to run:"
    info "    $BIN_PATH -config $INSTALLED_CONFIG"
    info "  Example OpenRC / runit / s6 unit files at deploy/init/"
else
    info "  shellboto is running. Message the bot on Telegram to try it."
    info ""
    info "  Logs:    journalctl -u shellboto -f"
    info "  Status:  systemctl status shellboto"
fi
info "  Config:  $ETC_DIR/"
info "  State:   $STATE_DIR/state.db"
echo
