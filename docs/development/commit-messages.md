# Commit messages

shellboto uses [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/).
The `commit-msg` hook enforces it.

## Format

```
<type>(<scope>)!: <description>

[optional body]

[optional footer(s)]
```

- **type** — one of: `feat`, `fix`, `docs`, `style`, `refactor`,
  `perf`, `test`, `build`, `ci`, `chore`, `revert`.
- **scope** — optional; lowercase identifier. Common scopes in
  this repo: `shell`, `audit`, `db`, `config`, `danger`,
  `redact`, `telegram`, `deploy`, `lint`, `make`, `deps`,
  `hooks`, `runit`, `openrc`, `s6`, `cli`, `release`,
  `supernotify`, `middleware`, `ratelimit`, `rbac`, `files`,
  `stream`.
- **!** — optional; marks a breaking change.
- **description** — 1–72 chars, imperative mood ("add", not
  "added").

## Examples

Good:

```
feat(shell): add PROMPT_COMMAND-based boundary signalling
fix(audit): trim trailing newlines from captured output
fix(audit)!: drop legacy sentinel column from audit_events
docs: clarify rollback behaviour in deploy/README.md
chore(deps): bump gorm.io/gorm to v1.32.2
ci: adopt release-please for CHANGELOG automation
refactor(stream): extract pickBreak into its own helper
perf(db): add index on audit_events.ts
test(danger): cover the chattr +i / -i regex with more inputs
build(make): exclude .git from tarball target
revert: "feat(stream): add colour support" — broke HTML escape
```

Bad (will be rejected):

```
WIP
added stuff
Update README.md
feat Added danger matcher
fix bug
```

## What each type is for

- **feat** — new user-visible capability (command, flag, config
  key).
- **fix** — bug fix.
- **docs** — documentation only (this directory).
- **style** — formatting, whitespace, no-op refactors.
- **refactor** — code restructure with no behavioural change.
- **perf** — performance improvement.
- **test** — test addition / change.
- **build** — build system / dependencies (`go.mod`, Makefile).
- **ci** — CI config (GitHub Actions, lefthook, linter config).
- **chore** — anything else. Most commonly `chore(deps): ...`.
- **revert** — reverts a previous commit. Include the original
  subject line.

## The `!` marker

`feat!:` or `feat(scope)!:` indicates a breaking change. Bumps
the version differently under release-please's semver rules:

- Pre-1.0 (we're here): `feat!` bumps minor; `feat` bumps patch.
- Post-1.0: `feat!` bumps major; `feat` bumps minor.

Also prefer a `BREAKING CHANGE:` footer if the ! alone isn't
sufficient:

```
feat(db)!: rename users.enabled to users.disabled_at

BREAKING CHANGE: operator-written queries against users.enabled
need to be rewritten to `users.disabled_at IS NULL`.
```

## What release-please does with these

- `feat`, `fix`, `perf`, `refactor`, `docs`, `test`, `build`,
  `ci` are rendered in `CHANGELOG.md` under matching section
  headers.
- `chore`, `style` are hidden from the CHANGELOG.
- Breaking markers (`!` / `BREAKING CHANGE:`) bubble up to
  "⚠ BREAKING CHANGES" section.

## The hook

`scripts/commit-msg-check.sh` runs:

```bash
grep -E '^(feat|fix|docs|style|refactor|perf|test|build|ci|chore|revert)(\([a-z0-9-]+\))?!?: .{1,72}$' "$1"
```

Plus merge-commit / revert-commit exemptions for the messages
git generates automatically.

## Writing a long commit message

First line is the subject. Then blank line. Then body
(paragraphs wrap at ~72 chars). Then blank line. Then footers
(one per line, `Key: value` shape).

```
feat(shell): pty-backed bash subprocess management

Each authorized user gets a dedicated, persistent bash in a pty.
State (cwd, env, aliases, job control) survives between messages.

- Pty + bash with a dedicated control pipe (fd 3) for
  command-boundary signalling
- PROMPT_COMMAND dispatcher emits done:<N> on completion
- Per-command output cap with automatic SIGKILL on overflow

Closes #42
```

The body is optional. Small mechanical changes don't need one.
Anything larger than a one-liner benefits from a paragraph of
why.

## Don't

- **Reference internal tool names** no one outside will
  understand ("make `ci-lint-action-v9` work").
- **Write the subject in English past-tense** ("fixed the bug";
  the rest of the world writes "fix the bug").
- **Put the ticket number at the top** — put it in the footer as
  `Refs: #N`.

## Read next

- [releasing.md](releasing.md) — how these messages become
  releases.
- [ci.md](ci.md) — the `commit-lint` job that runs on every PR.
