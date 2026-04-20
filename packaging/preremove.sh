#!/bin/sh
# shellboto pre-remove: stop + disable the service cleanly before the
# package manager rips the binary out. Preserves /etc/shellboto/env
# and /var/lib/shellboto/state.db — the package manager only owns the
# binary + unit + *.example files.

set -e

if command -v systemctl >/dev/null 2>&1; then
    if systemctl is-active --quiet shellboto; then
        systemctl stop shellboto || true
    fi
    if systemctl is-enabled --quiet shellboto 2>/dev/null; then
        systemctl disable shellboto || true
    fi
fi

exit 0
