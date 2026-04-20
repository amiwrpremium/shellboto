# Contributing to shellboto

Thanks for poking around. shellboto is a small project; the workflow
below keeps it that way.

## Prerequisites

- Go 1.25+
- `make`, `bash`, `git`
- A Unix-like environment (the bot itself is Linux-only; development
  happens fine on macOS/Linux)

## First-time setup

```bash
git clone https://github.com/amiwrpremium/shellboto
cd shellboto

# Installs lefthook, golangci-lint, goreleaser, git-chglog, gitleaks,
# govulncheck, goimports. Idempotent — safe to re-run.
./scripts/install-dev-tools.sh

# Wire lefthook into .git/hooks so pre-commit/commit-msg/pre-push fire.
make hooks-install
```

## Commit message format

shellboto uses **[Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/)**.
Every commit message must match:

```
<type>(<scope>)!: <description>
```

- **type** — one of `feat`, `fix`, `docs`, `style`, `refactor`, `perf`,
  `test`, `build`, `ci`, `chore`, `revert`.
- **scope** — optional; lowercase identifier (e.g. `shell`, `audit`,
  `deploy`).
- **!** — optional; marks a breaking change.
- **description** — 1–72 chars, imperative mood ("add", not "added").

Good:

```
feat(shell): add PROMPT_COMMAND-based boundary signalling
fix(audit): trim trailing newlines from captured output
fix(audit)!: drop legacy sentinel column from audit_events
docs: clarify rollback behaviour in deploy/README.md
chore: bump golangci-lint to v2.1
```

Bad (will be rejected by the commit-msg hook):

```
WIP
added stuff
Update README.md
```

Merge, revert, fixup, and squash commits are exempt — they pass through
unchanged.

## Hooks

lefthook runs three stages:

| Stage | When | What |
|---|---|---|
| pre-commit | `git commit` | gofmt, goimports, golangci-lint (incremental), gitleaks, `go mod tidy` diff, shellcheck, yamllint |
| commit-msg | after you write the message | Conventional Commits regex |
| pre-push | `git push` | `go test`, `go vet`, `govulncheck`, `goreleaser check` |

To run a stage on demand:

```bash
lefthook run pre-commit --all-files
lefthook run pre-push
```

To skip in an emergency: `git commit --no-verify` or `git push --no-verify`.
**CI mirrors every hook**, so skipped hooks get caught at PR time.

## Running checks manually

```bash
make fmt            # gofmt + goimports across the tree
make lint           # golangci-lint run
make test           # go test ./...
make vet            # go vet ./...
make vuln           # govulncheck ./...
make test-deploy    # shell-script unit tests (deploy/lib.sh)
```

## Release process

Releases are tag-driven. To cut one:

```bash
# 1. Make sure main is green.
make release-check      # lint + test + vet + vuln + goreleaser check

# 2. Tag with a semver prefix.
git tag -a v0.1.0 -m "v0.1.0"

# 3. Push the tag; CI does the rest.
git push origin v0.1.0
```

`.github/workflows/release.yml` runs goreleaser, which produces:

- `linux/amd64` + `linux/arm64` + `darwin/*` binaries and tarballs
- `.deb` and `.rpm` packages for Linux
- Homebrew formula pushed to `amiwrpremium/homebrew-shellboto` (if the
  `HOMEBREW_TAP_GITHUB_TOKEN` repo secret is set)
- `checksums.txt`
- CycloneDX SBOMs per archive
- GitHub release with Conventional-Commit-grouped notes

After the release, a second job regenerates `CHANGELOG.md` and commits
it back to `main`.

### Dry-run locally

```bash
make release-snapshot   # builds everything into dist/ without publishing
```

## Changelog

`CHANGELOG.md` is **generated**, not hand-edited. `make changelog`
runs `git-chglog` which reads tags + commit messages and writes the
file. Same output both locally and in the release workflow.

## Code style

- `gofmt` + `goimports -local github.com/amiwrpremium/shellboto` — enforced.
- Comments explain **why** a piece of code exists, not what it does.
  Identifiers already say what.
- No gratuitous abstraction. Three similar lines are better than a
  premature helper.
- Don't mock the database in integration tests. Use a temp SQLite via
  the existing `newTestRepo(t)` helpers.

## Reporting security issues

Please **don't** open a public issue for security reports. Email
amiwrpremium@gmail.com with details.

## Where things live

```
cmd/shellboto/          main binary + CLI subcommands
internal/shell/         pty + bash subprocess management
internal/telegram/      bot + callbacks + middleware
internal/db/            SQLite + hash-chained audit log
internal/config/        multi-format config loader
internal/danger/        danger-pattern regex matcher
internal/redact/        secret scrubber for audit storage
deploy/                 install/uninstall/rollback scripts + unit files
packaging/              goreleaser-driven .deb/.rpm assets
scripts/                dev tooling (install, commit-msg check)
.github/workflows/      CI + release pipelines
```
