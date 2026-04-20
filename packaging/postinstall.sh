#!/bin/sh
# shellboto post-install: register the unit + tell the operator what
# they still need to do. Does NOT auto-start — the env file has
# placeholders until the operator sets TOKEN/SUPERADMIN_ID.

set -e

# Make sure /etc/shellboto exists with the right mode. The example
# files were already placed there by the package payload; this just
# tightens the parent dir if it happened to be looser.
install -d -m 0700 /etc/shellboto

# Copy the examples into the active config / env file only if those
# don't already exist (upgrade-safe — preserves operator edits).
if [ ! -e /etc/shellboto/env ]; then
    cp -a /etc/shellboto/env.example /etc/shellboto/env
    chmod 0600 /etc/shellboto/env
fi
if [ ! -e /etc/shellboto/config.toml ]; then
    cp -a /etc/shellboto/config.toml.example /etc/shellboto/config.toml
    chmod 0600 /etc/shellboto/config.toml
fi

if command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload || true
fi

cat <<'EOF'

shellboto installed.

Next steps:
  1. Edit /etc/shellboto/env and set SHELLBOTO_TOKEN (from @BotFather)
     and SHELLBOTO_SUPERADMIN_ID (your Telegram user ID).
  2. Generate an audit seed:
       openssl rand -hex 32
     and paste it as the value of SHELLBOTO_AUDIT_SEED.
  3. Verify with: shellboto doctor
  4. Enable + start: systemctl enable --now shellboto
  5. Tail logs:      journalctl -u shellboto -f

EOF

exit 0
