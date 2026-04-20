# CI — GitHub Actions

Five workflows under `.github/workflows/`.

| Workflow | Fires on | Runs |
|----------|----------|------|
| `ci.yml` | push to `master`, every PR | lint, test, vet, vuln, shellcheck, gitleaks, goreleaser-check, commit-lint (PR only) |
| `release.yml` | push of a `v*` tag | goreleaser release --clean (publishes artifacts, optional Homebrew tap push) |
| `codeql.yml` | push/PR on master, weekly cron | CodeQL Go analysis with `security-and-quality` query set |
| `release-please.yml` | push to master | Opens / updates a release PR bumping the next version + CHANGELOG |
| `dependabot-auto-merge.yml` | PR from `dependabot[bot]` | Auto-queues squash-merge for semver-patch + non-security PRs |

## `ci.yml`

### Jobs

| Job | Run |
|-----|-----|
| `lint` | gofmt diff, goimports diff, golangci-lint v2.11.4 (goinstall) |
| `test (go 1.26)` | `go test ./...` with Go 1.26 |
| `test (go stable)` | `go test ./...` with latest stable |
| `vet` | `go vet ./...` |
| `vuln` | `govulncheck ./...` |
| `shellcheck` | `ludeeus/action-shellcheck@master` — scans `deploy/`, `packaging/`, `scripts/`, init scripts |
| `gitleaks` | full-repo scan with our `.gitleaks.toml` |
| `commit-lint` (PR only) | Validates every PR commit via `scripts/commit-msg-check.sh` |
| `goreleaser-check` | `goreleaser check` on `.goreleaser.yaml` |

### Status check names

When setting up branch protection (required checks), these are
the GitHub-side names:

- `ci/lint`
- `ci/test (go 1.26)`
- `ci/test (go stable)`
- `ci/vet`
- `ci/vuln`
- `ci/shellcheck`
- `ci/gitleaks`
- `ci/commit-lint`
- `ci/goreleaser-check`
- `codeql/analyze (go)`

## `release.yml`

Fires on `push` of any tag matching `v*`.

Single job: `goreleaser`:

```yaml
- uses: actions/checkout@v4
  with: { fetch-depth: 0 }
- uses: actions/setup-go@v5
  with: { go-version-file: go.mod, cache: true }
- uses: goreleaser/goreleaser-action@v7
  with: { version: latest, args: "release --clean" }
  env:
    GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
    HOMEBREW_TAP_GITHUB_TOKEN: ${{ secrets.HOMEBREW_TAP_GITHUB_TOKEN }}
```

goreleaser does the rest — see
[../packaging/goreleaser.md](../packaging/goreleaser.md).

If `HOMEBREW_TAP_GITHUB_TOKEN` is unset, goreleaser logs a warning
and skips the brew push. Rest of the release still completes.

## `codeql.yml`

Triggers:

- push to `master`
- PR to `master`
- cron `0 5 * * 1` (Monday 05:00 UTC weekly)

Uses the standard `github/codeql-action` setup. `queries:
security-and-quality` — superset of `security-extended`, includes
some stylistic rules. Pre-existing findings on first scan are
expected; triage under Security → Code scanning.

`go-version-file: go.mod` so CodeQL tracks the toolchain bump
automatically.

## `release-please.yml`

Fires on every push to master. Runs
`googleapis/release-please-action@v4`:

```yaml
- uses: googleapis/release-please-action@v4
  with:
    config-file: release-please-config.json
    manifest-file: .release-please-manifest.json
```

Effect:

- Scans recent commits since the last release.
- If any trigger a version bump (`feat`, `fix`, etc.), opens or
  updates a "release PR" titled `chore(master): release X.Y.Z`.
- The PR bumps the version in `.release-please-manifest.json` and
  rewrites `CHANGELOG.md`.
- Merging the PR auto-tags `vX.Y.Z` → `release.yml` fires.

See [releasing.md](releasing.md).

## `dependabot-auto-merge.yml`

Fires on any PR from `dependabot[bot]`.

Uses `dependabot/fetch-metadata@v2` to classify the update; then:

```yaml
if: |
  steps.meta.outputs.update-type == 'version-update:semver-patch' &&
  !contains(github.event.pull_request.labels.*.name, 'security')
run: gh pr merge --auto --squash "$PR"
```

Rules:

- Only `semver-patch` level bumps auto-queue.
- Never auto-merge if the PR is labelled `security` — those stay
  open for human review.
- `--auto` flag means GitHub waits for required status checks
  before actually merging.

Minor + major bumps stay open for human review regardless.

## `.github/dependabot.yml`

Config for Dependabot itself:

```yaml
version: 2
updates:
  - package-ecosystem: gomod
    directory: /
    schedule: {interval: weekly, day: monday, time: "05:00", timezone: UTC}
    commit-message: {prefix: "chore(deps)", include: scope}
    groups:
      go-minor-patch: {update-types: [minor, patch]}
    labels: [dependencies, go]

  - package-ecosystem: github-actions
    directory: /
    schedule: {interval: weekly}
    commit-message: {prefix: "chore(ci)", include: scope}
    groups:
      actions-minor-patch: {update-types: [minor, patch]}
    labels: [dependencies, ci]
```

Weekly scans, minor+patch grouped into single PRs per ecosystem,
major bumps open individually.

## Secrets in use

- `GITHUB_TOKEN` — provided automatically.
- `HOMEBREW_TAP_GITHUB_TOKEN` — optional; fine-grained PAT
  scoped to `amiwrpremium/homebrew-shellboto` for the Homebrew
  tap push.

No other secrets. No CI providers other than GitHub Actions.

## What CI doesn't do

- **Deploy anywhere.** Deployment is an operator step after
  release artifacts are published.
- **Run the shellboto service.** CI is build + test + lint.
  Acceptance testing against a live bot is out of scope.
- **Nightly runs.** CodeQL's weekly cron is the only scheduled
  run.

## Read next

- [releasing.md](releasing.md) — how releases flow.
- [github-settings.md](github-settings.md) — branch protection +
  required checks.
