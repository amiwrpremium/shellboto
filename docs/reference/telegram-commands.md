# Telegram command reference

Every slash command the bot accepts.

| Command | Role | Args | Effect | Rate-limit exempt |
|---------|:----:|------|--------|:-----------------:|
| `/start` | any | — | Register metadata, reply greeting | no |
| `/help` | any | — | List commands for caller's role | no |
| `<plain text>` | user+ | the command | Runs as shell command in the caller's pty | no (danger-matched first) |
| `/status` | user+ | — | idle/running + timestamps | no |
| `/cancel` | user+ | — | SIGINT to foreground process group | **yes** |
| `/kill` | user+ | — | SIGKILL to foreground process group | **yes** |
| `/reset` | user+ | — | Close + respawn the caller's pty | no |
| `/get <path>` | user+ | filesystem path | Download file from VPS | no |
| `/auditme` | user+ | — | Caller's own last 10 audit events | no |
| file attachment | user+ | (caption = dest path) | Upload file to VPS | no |
| `/users` | admin+ | — | Open inline users browser | no |
| `/adduser <id>` | admin+ | target Telegram ID | Wizard: validate + name + confirm | no |
| `/adduser <id> admin` | superadmin | target + role | Same wizard, promote to admin at end | no |
| `/deluser <id>` | admin+* | target | Confirm → soft-delete | no |
| `/audit [N]` | admin+ | optional count (default 20) | Recent audit events, all users | no |
| `/audit-out <event_id>` | admin+ | audit row id | Fetch + send captured output | no |
| `/audit-verify` | admin+ | — | Walk hash chain, reply ✅/❌ | no |
| `/role <id> admin\|user` | superadmin | target + new role | Promote or demote | no |

\*Admin can `/deluser` only `role=user` targets. Superadmin can
delete any non-superadmin.

## Inline-keyboard callbacks

Callback data prefixes (see
[../telegram/callbacks-and-flows.md](../telegram/callbacks-and-flows.md)):

| Prefix | Purpose |
|--------|---------|
| `j:c` | /cancel-equivalent button on running message |
| `j:k` | /kill-equivalent button |
| `us:open:<id>` | Open user profile in users browser |
| `us:ban:<id>` | Ban from user profile |
| `us:profile:<id>` | Refresh profile view |
| `us:close` | Close users browser |
| `pr:<id>` | Confirm promote |
| `dm:<id>` | Confirm demote |
| `ad:confirm` | Confirm in adduser wizard |
| `ad:cancel` | Cancel adduser wizard |
| `dg:run:<token>` | Run the danger-matched command |
| `dg:cancel:<token>` | Cancel it |
| `rg:confirm` | Superadmin register flow confirm |

## Audit kinds fired per command

See [audit-kinds.md](audit-kinds.md) for the full map.
