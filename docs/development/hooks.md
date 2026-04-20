# Hooks

Lefthook runs a set of checks on each git operation. Defined in
`.lefthook.yml`.

## Stages

### `pre-commit` (parallel, staged-files only)

Fires on `git commit`. Fast checks on staged files.

| Command | Glob | Runs |
|---------|------|------|
| `gofmt` | `*.go` | `gofmt -l -w {staged_files}` + re-stage |
| `goimports` | `*.go` | `goimports -l -w -local github.com/amiwrpremium/shellboto {staged_files}` + re-stage |
| `golangci-lint` | `*.go` | `golangci-lint run --new-from-rev=HEAD --timeout=2m` |
| `gitleaks` | any | `gitleaks protect --staged --redact --verbose` |
| `go-mod-tidy` | `go.mod,go.sum` | `go mod tidy -diff` (fails if tidy would change something) |
| `shellcheck` | `*.sh` | `shellcheck {staged_files}` |
| `shellcheck-init-run` | `deploy/init/runit/**/run` | `shellcheck {staged_files}` |
| `shellcheck-init-openrc` | `deploy/init/openrc/shellboto` | `shellcheck {staged_files}` |
| `yamllint` | `*.{yml,yaml}` | `yamllint -s {staged_files}` |

### `commit-msg`

Fires after you've written the commit message. Validates
Conventional Commits.

```
bash scripts/commit-msg-check.sh {1}
```

Regex: `^(feat|fix|docs|style|refactor|perf|test|build|ci|chore|revert)(\([a-z0-9-]+\))?!?: .+$`

Merge commits / revert commits / fixup + squash commits are
exempt.

### `pre-push` (serial)

Fires on `git push`. Heavier checks.

| Command | Runs |
|---------|------|
| `test` | `go test -count=1 -timeout 60s ./...` |
| `vet` | `go vet ./...` |
| `vuln` | `govulncheck ./...` |
| `goreleaser-check` | `goreleaser check` |

If any fail, the push is rejected. CI will fail the same way if
you skip the hook — so it's just saving you the round-trip.

### `post-merge` / `post-checkout`

```
scripts/lefthook-drift-check.sh
```

Warns (doesn't block) if `.lefthook.yml` itself changed since last
invocation. Prompts you to re-run `make hooks-install` to pick up
any newly-declared hook types.

## Bypassing

Emergency bypass:

```bash
git commit --no-verify          # skip pre-commit + commit-msg
git push --no-verify            # skip pre-push
```

CI will re-run every check on the resulting PR, so bypassing
doesn't actually skip the wall — just defers it.

## Re-running manually

```bash
lefthook run pre-commit --all-files
lefthook run pre-push
lefthook run commit-msg .git/COMMIT_EDITMSG    # the current message
```

Useful when debugging a hook failure — isolate which command is
complaining.

## Wiring

`make hooks-install` runs `lefthook install`, which writes
scripts under `.git/hooks/`:

- `.git/hooks/pre-commit` → `lefthook run pre-commit "$@"`
- `.git/hooks/commit-msg` → `lefthook run commit-msg "$@"`
- `.git/hooks/pre-push` → `lefthook run pre-push "$@"`
- `.git/hooks/post-merge` → `lefthook run post-merge "$@"`
- `.git/hooks/post-checkout` → `lefthook run post-checkout "$@"`

Standard git-hook mechanism. Run `make hooks-uninstall` to
remove.

## Adding a new hook

1. Edit `.lefthook.yml`.
2. Run `make hooks-install` to pick up any new hook type.
3. Test: `lefthook run <stage> --all-files`.
4. Commit. The `post-merge` drift-check nudges other developers
   to run `make hooks-install` when they pull your change.

## What CI runs

Mirrors every hook stage. See [ci.md](ci.md).

## Why lefthook, not pre-commit / husky

- **Lefthook** — Go binary; one artifact; simple config. Matches
  shellboto's "one binary" aesthetic.
- **pre-commit** (Python) — requires Python. More features but
  more weight.
- **husky** (Node) — requires Node. Wrong community for a Go
  project.

## Reading the code

- `.lefthook.yml` — the config.
- `scripts/commit-msg-check.sh` — the regex.
- `scripts/lefthook-drift-check.sh` — the nudge.

## Read next

- [linting.md](linting.md) — the golangci-lint config.
- [commit-messages.md](commit-messages.md) — Conventional
  Commits rules.
