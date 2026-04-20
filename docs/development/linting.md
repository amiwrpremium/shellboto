# Linting

`golangci-lint` v2 config in `.golangci.yml`.

## Enabled linters

- **govet** — the standard Go vet checks (shadowing, struct
  tags, etc.). Fast, catches real bugs.
- **errcheck** — flags unchecked error returns.
- **staticcheck** — deep semantic analysis. Catches nil
  dereferences, redundant nil checks, deprecated API usage.
- **ineffassign** — flags variable writes that are never read.
- **unused** — flags unused code.
- **misspell** — flags English typos.

## Deliberately disabled

- **revive** — much overlap with staticcheck + more style
  warnings. Not worth the noise.
- **gocritic** — more style. Reasonable for some projects; not
  here.
- **gosec** — lots of G204 noise (`exec.Command` with variable
  input — literally the entire point of shellboto). False
  positives dominate.

## `errcheck` exclude-functions

From `.golangci.yml`:

```yaml
errcheck:
  check-type-assertions: false
  check-blank: false
  exclude-functions:
    - (*os.File).Close
    - (*database/sql.DB).Close
    - (io.Closer).Close
    - (*gorm.io/gorm.DB).Close
    - fmt.Fprintln
    - fmt.Fprintf
    - fmt.Fprint
    - os.Remove
    - os.RemoveAll
    - os.Chmod
    - os.MkdirAll
    - (*github.com/go-telegram/bot.Bot).SendMessage
```

Reason: deferred `Close()` on best-effort cleanup paths is a
widespread idiom in this codebase. Failing errcheck on every one
adds noise without catching bugs.

Similarly `fmt.Fprint*` — we do `fmt.Fprintf(os.Stderr, ...)` in
the CLI subcommands; the error path (stderr unwritable) is
handled by the process dying, not by error returns.

If you want to force an error-check on a call we've excluded,
write the explicit form:

```go
if err := io.Closer(something).Close(); err != nil {
    logger.Warn("close failed", zap.Error(err))
}
```

## `staticcheck` disables

```yaml
staticcheck:
  checks:
    - all
    - -QF1002      # "could use tagged switch" — stylistic
    - -QF1008      # "could remove embedded field" — stylistic
    - -ST1000      # "package should have package comment"
    - -ST1003      # "should not use ALL_CAPS" — breaks on some generated code
    - -ST1020      # "comment should start with identifier name"
```

Why: QF* (quickfix) are informational; ST1000/ST1003/ST1020 are
stylistic prescriptions; too much noise on the existing tree to
fix incrementally. Revisit in a dedicated style pass if desired.

## Test-file exclusions

```yaml
exclusions:
  rules:
    - path: _test\.go
      linters:
        - errcheck
```

Tests routinely touch internals + error-return combos where the
returned error is deliberately ignored. Silencing errcheck in
tests lets the mainline code stay strict.

## Running

Full:

```bash
make lint
```

Or directly:

```bash
golangci-lint run --timeout=3m
```

Pre-commit (incremental):

```bash
golangci-lint run --new-from-rev=HEAD --timeout=2m
```

## CI

GitHub Actions uses `golangci/golangci-lint-action@v9` with
`install-mode: goinstall`:

```yaml
- name: golangci-lint
  uses: golangci/golangci-lint-action@v9
  with:
    version: v2.11.4
    install-mode: goinstall
    args: --timeout=3m
```

`goinstall` mode compiles the binary under the runner's Go 1.26,
which is required because the prebuilt v2.x binaries on the
release page are Go-1.24-built and refuse to load configs that
target Go 1.26 or later.

## Upgrading linter versions

- Bump the `v2.x.y` pin in both `.github/workflows/ci.yml` and
  `scripts/install-dev-tools.sh`.
- Run `make lint` locally; accept any new / fixed issues.
- Commit under `chore(lint): bump golangci-lint to vX.Y.Z`.

## Suppressing individual issues

Inline comment:

```go
// nolint:errcheck // deliberate — best-effort cleanup, failure is acceptable
defer f.Close()
```

Use sparingly. Prefer fixing the issue; suppress only when the
lint rule's assumption doesn't apply (e.g. defer-cleanup path
where error handling is a distraction).

## Read next

- [testing.md](testing.md) — the other quality gate.
- [commit-messages.md](commit-messages.md) — how the lint bumps
  get announced.
