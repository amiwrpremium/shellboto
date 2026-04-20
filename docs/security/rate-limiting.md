# Rate limiting

Two distinct rate limiters, with different purposes.

## Post-auth: per-user command budget

Protects the bot from a single whitelisted user issuing commands
so fast they starve other users or overwhelm downstream services.

**Config:**

```toml
rate_limit_burst = 10              # bucket capacity
rate_limit_refill_per_sec = 1.0    # tokens per second
```

Default settles at:

- 10 tokens immediately available.
- Refills at 1 token/sec → 60/min steady state.
- Short spikes (up to 10 in quick succession) allowed.

Keyed by `telegram_id` after authentication succeeds.

### Exempt commands

`/cancel` and `/kill` (plus their inline-button equivalents `j:c`
and `j:k`) bypass the limiter. Rationale: if you've rate-limited
yourself by spamming, the one command you want to always work is
"stop what's running."

### Over-limit behaviour

The dispatch reply: `⚠ rate limited (please slow down)`. No
handler invocation; no DB write beyond the rate-limit-exceeded
metric in the zap log.

Tokens regenerate while the user waits; no persistent penalty.

### Tuning

- **`0`** disables entirely. Do this only if you're the only user
  and trust yourself to self-limit.
- **`burst=30, refill=3.0`** — bursty but high sustained throughput
  (ops-heavy team).
- **`burst=5, refill=0.25`** — tight rein (~15/min) for a large
  whitelist where any one user burning bandwidth is noticeable.

## Pre-auth: audit-write DoS protection

This is the important one.

**Config:**

```toml
auth_reject_burst = 5
auth_reject_refill_per_sec = 0.05
```

Default settles at:

- 5 auth-reject rows for an attacker immediately.
- Refills at 0.05/sec → 1 row every 20 seconds steady state.
- Daily rate per attacker-id: ~4300 rows, ~2 MB DB growth.

Keyed by Telegram `from_id` regardless of whitelist membership.

### Why you need it

Telegram bots are *by design* discoverable — once you set
`@BotFather /setcommands` or someone shares the bot link, anyone
can DM it. For shellboto, non-whitelisted DMs generate
`auth_reject` audit rows.

Without limiting, an attacker can:

- Send 30 updates/sec (Telegram's per-bot ceiling for Bot API).
- Each creates a DB row + a journald log line.
- Over 24h: 30 × 60 × 60 × 24 = 2.6M rows × ~50 bytes = **130 MB
  audit DB growth per attacker-id per day**.
- Multiplied across several attacker accounts, the DB grows fast
  enough to fill disk.

With the default limiter: ~4300 rows/day × attacker accounts. An
attacker using 100 distinct IDs still only writes 430k rows/day
(~20 MB) — noisy, but bounded.

### How it applies

```
update arrives
    │
    ▼
pre-auth check: is this from_id over-budget?
    ├── yes: silent drop; no DB write, no log, no reply.
    └── no: consume 1 token; proceed to whitelist lookup.
            │
            ▼
        whitelist lookup
            ├── not found / disabled → write auth_reject row + log
            └── found → dispatch
```

Critically, the check happens **before** the auth_reject row is
written. An attacker past their pre-auth quota gets literal
silence — no audit row, no journald line.

### Silent drop reasoning

If we logged over-limit drops, the log is still growing linearly
with their spam. Silent keeps log size bounded.

You **do** see that there's an attacker — via the normal
`auth_reject` rows that make it through before hitting the cap.
You'll see ~5 initial rows, then periodic rows at 1/20s = 3/min.
That's an obvious attacker signal and small enough not to matter.

### Can't disable in production

`auth_reject_burst=0` disables the pre-auth limiter entirely. Every
hostile message writes a row. Don't:

- Public Telegram bots see crawler traffic regardless of how
  obscure the username is.
- Crawlers DM every bot on a list of usernames; shellboto sees the
  DM, tries to authenticate, writes `auth_reject`.
- Bounded log size matters for disk + forensic signal-to-noise.

The installer doesn't let you set `0` in `-y` mode without an extra
`--i-really-want-unbounded-audit-writes` flag that doesn't exist
because nobody should need it.

### Tuning

Defaults are reasonable for any production deployment. Raise if:

- You're testing and want the full reject stream in logs (but
  you're already raising from a known test ID).
- You have a specific compliance need for every attempt to be
  logged (but then you probably need the journald mirror more than
  the DB row — journald rotates).

Lower if:

- Your VPS is disk-tight and you're seeing attacker spikes. Ratchet
  `auth_reject_refill_per_sec` down to `0.01` (1 row every 100s).

## Observing rate-limit activity

Every rate-limited attempt is counted in zap. Search journald for
structured fields:

```bash
sudo journalctl -u shellboto --since "1h ago" | grep rate_limit
```

Or in shellboto's own metrics (if you export them to your own
Prometheus via a sidecar): not built-in today. PRs welcome.

## Interaction with supernotify

Supernotify (the superadmin fan-out for important events) is not
rate-limited — it sends a DM per event. Spamming the bot with
whitelisted-user operations (promotions, demotions, adds) is
bounded by the post-auth rate limiter above. A legitimate operator
can't reach a volume where supernotify becomes noisy.

## Read next

- [whitelist-and-rbac.md](whitelist-and-rbac.md) — why the DB-side
  writes are gated on the pre-auth limiter.
- [audit-chain.md](audit-chain.md) — what's stored once writes do
  happen.
