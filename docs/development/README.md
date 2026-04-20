# Development

Hacking on shellboto — prerequisites, build, test, lint, CI,
release flow.

| File | What it covers |
|------|----------------|
| [prerequisites.md](prerequisites.md) | Go 1.26+, make, required tooling |
| [build-from-source.md](build-from-source.md) | `make build`, `make test`, all targets |
| [hooks.md](hooks.md) | Lefthook stages + local loop |
| [linting.md](linting.md) | golangci-lint v2 config explained |
| [commit-messages.md](commit-messages.md) | Conventional Commits rules + scope list |
| [testing.md](testing.md) | Real SQLite, real pty, no-mocks rule |
| [ci.md](ci.md) | GitHub Actions workflows walkthrough |
| [releasing.md](releasing.md) | release-please flow + emergency manual |
| [github-settings.md](github-settings.md) | Pointer to `.github/SETTINGS.md` |

## Quick start

```bash
git clone https://github.com/amiwrpremium/shellboto.git
cd shellboto
./scripts/install-dev-tools.sh      # lefthook, golangci-lint, etc.
make hooks-install                   # wire lefthook into .git/hooks
make test                            # go test ./...
make lint                            # golangci-lint
```

Commit messages follow
[Conventional Commits](https://www.conventionalcommits.org/) —
the `commit-msg` hook enforces the format.

## Contributing

See [CONTRIBUTING.md](../../CONTRIBUTING.md) in the repo root.
Short version: open an issue first for non-trivial changes, run
the hooks, send a PR.

## Read next

- [prerequisites.md](prerequisites.md) — what to install first.
- [build-from-source.md](build-from-source.md) — get the binary.
