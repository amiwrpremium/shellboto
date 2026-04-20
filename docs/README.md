# shellboto documentation

Everything you need to run, operate, extend, or audit shellboto.
Every subdirectory has its own `README.md` that indexes the files
inside it, so browsing on GitHub is self-guiding.

> ⚠ **shellboto is a remote shell bot that, by default, runs as root.**
> The whitelist is the only thing between a compromised Telegram
> account and full control of your VPS. Read [security/](security/)
> *before* you whitelist anyone.

## Where to start

| If you want to… | Read this |
|---|---|
| Get a bot running in under 10 minutes | [getting-started/quickstart.md](getting-started/quickstart.md) |
| Understand how the pieces fit together | [architecture/overview.md](architecture/overview.md) |
| Know every config key and env var | [configuration/](configuration/) |
| Understand the audit chain | [security/audit-chain.md](security/audit-chain.md) |
| See every regex the danger matcher ships with | [security/danger-matcher.md](security/danger-matcher.md) |
| Fix a broken prod after a bad release | [runbooks/](runbooks/) |
| Hack on the code or add a command | [development/](development/) |

## Documentation map

### Onboarding
- **[getting-started/](getting-started/)** — Create the Telegram bot
  at @BotFather, find your user ID, run the installer, send your
  first command.

### What it is + how it's built
- **[architecture/](architecture/)** — Overview, tech stack, project
  layout, package-import graph, runtime model, data flow end-to-end,
  concurrency model, design-decision log.

### Running it
- **[configuration/](configuration/)** — TOML/YAML/JSON config, env
  vars, role table, non-root user-shells, timeouts and idle reaping,
  audit output modes.
- **[deployment/](deployment/)** — Interactive installer, systemd
  unit walkthrough, OpenRC / runit / s6 init scripts, uninstaller,
  rollback, production checklist.
- **[operations/](operations/)** — `shellboto doctor`, log triage,
  idle-reap and heartbeat, user management, updating, monitoring.

### Security + internals
- **[security/](security/)** — Threat model, whitelist/RBAC,
  audit-chain mathematics, seed rotation, **every danger-matcher
  regex with a worked example and rationale**, secret redaction
  pipeline, rate limiting, root-shell blast radius.
- **[shell/](shell/)** — How the pty is allocated, the fd-3 control
  pipe, PROMPT_COMMAND boundary detection, signal handling, non-root
  drop-privs setup.
- **[audit/](audit/)** — Audit schema, every event `kind`, hash-chain
  deep dive, output-blob storage, retention/prune, every `shellboto
  audit …` subcommand.
- **[database/](database/)** — Full schema, migration policy, the
  instance-lockfile, backup, restore, vacuum.
- **[telegram/](telegram/)** — Every slash command by role, inline
  callbacks and multi-step flows, streaming message-edit pipeline,
  supernotify fan-out, file transfer.

### Building + shipping
- **[development/](development/)** — Prerequisites, build from
  source, Lefthook hooks, golangci-lint profile, Conventional
  Commits, testing strategy, GitHub Actions walkthrough, release-
  please-driven releases.
- **[packaging/](packaging/)** — Goreleaser matrix, .deb/.rpm via
  nfpm, Homebrew tap, SBOMs, download verification.

### When things go wrong
- **[runbooks/](runbooks/)** — Step-by-step procedures for bad
  release, leaked token, broken audit chain, DB corruption, stuck
  shell, disk full.
- **[troubleshooting/](troubleshooting/)** — Common error messages
  and their fixes.

### Reference
- **[reference/](reference/)** — Every CLI subcommand with every
  flag, every Telegram slash command, every env var, every config
  key, every audit event kind, every danger-matcher regex in one
  table, every file path the bot touches, every exit code.

### Miscellaneous
- **[faq.md](faq.md)** — The questions that keep coming up.

## Conventions used in these docs

- Fenced `bash` blocks are runnable as-is (copy-paste).
- `$USER_ID` placeholders are numeric Telegram user IDs (64-bit ints).
- Paths written `/etc/shellboto/…` assume the default prefix; if you
  passed `--prefix` to `install.sh`, prepend it.
- Links are relative so the docs render cleanly on GitHub, in `less`,
  and inside any offline mirror.

## Contributing to the docs

Same rules as code: Conventional Commits, `docs:` prefix, one
section or topic per PR. See [development/contributing.md](development/contributing.md).
