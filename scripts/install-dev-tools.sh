#!/usr/bin/env bash
# One-shot install for shellboto's dev toolchain.
# Re-running is safe — each tool is installed only if missing.
#
# Usage:  ./scripts/install-dev-tools.sh
# After:  make hooks-install   (wires lefthook into .git/hooks)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091 source=../deploy/lib.sh
source "$SCRIPT_DIR/../deploy/lib.sh"

color_init

title "shellboto dev tooling"

need_cmd go git

GOBIN="$(go env GOPATH)/bin"
case ":$PATH:" in
    *":$GOBIN:"*) ;;
    *) warn "$GOBIN is not on your PATH — add it so the tools install below are usable." ;;
esac

# install_go_tool NAME IMPORT_PATH
#
# Runs `go install` only when the binary is missing, so re-running the
# script is cheap.
install_go_tool() {
    local name="$1" importpath="$2"
    if command -v "$name" >/dev/null 2>&1; then
        ok "$name already installed ($(command -v "$name"))"
        return
    fi
    info "installing $name …"
    go install "$importpath"
    ok "$name installed"
}

section "Go-based tools"

install_go_tool lefthook        github.com/evilmartians/lefthook@latest
install_go_tool golangci-lint   github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
install_go_tool goreleaser      github.com/goreleaser/goreleaser/v2@latest
install_go_tool git-chglog      github.com/git-chglog/git-chglog/cmd/git-chglog@latest
install_go_tool gitleaks        github.com/zricethezav/gitleaks/v8@latest
install_go_tool govulncheck     golang.org/x/vuln/cmd/govulncheck@latest
install_go_tool goimports       golang.org/x/tools/cmd/goimports@latest

section "System tools"

if command -v yamllint >/dev/null 2>&1; then
    ok "yamllint already installed"
elif command -v brew >/dev/null 2>&1; then
    info "installing yamllint via brew …"
    brew install yamllint
    ok "yamllint installed"
elif command -v apt-get >/dev/null 2>&1; then
    info "installing yamllint via apt …"
    sudo apt-get update -qq
    sudo apt-get install -y yamllint
    ok "yamllint installed"
elif command -v dnf >/dev/null 2>&1; then
    info "installing yamllint via dnf …"
    sudo dnf install -y yamllint
    ok "yamllint installed"
elif command -v pip3 >/dev/null 2>&1; then
    info "installing yamllint via pip3 (user) …"
    pip3 install --user yamllint
    ok "yamllint installed"
else
    warn "yamllint not installed automatically — install manually if you want the pre-commit yaml check"
fi

if command -v shellcheck >/dev/null 2>&1; then
    ok "shellcheck already installed"
else
    warn "shellcheck not installed — install via your package manager (apt/brew/dnf install shellcheck)"
fi

echo
title "done"
info ""
info "  Next: make hooks-install"
info "  See CONTRIBUTING.md for the full dev workflow."
echo
