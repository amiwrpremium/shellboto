# Audit output modes

`audit_output_mode` controls whether captured command stdout/stderr
is persisted to the audit DB. Three settings with distinct privacy
and forensic trade-offs.

## The three modes

| Mode | Output stored? | When used |
|------|---------------|-----------|
| `always` (default) | ✅ every command | Maximum forensics. You want to replay exactly what happened. |
| `errors_only` | ✅ when exit != 0 | Balance: failures captured for debugging, successful output (often routine) not kept. |
| `never` | ❌ never | Most private. Audit metadata only (timestamp, kind, cmd, exit, bytes, SHA-256). |

## What's stored in each mode

Every mode stores:

- Timestamp
- User ID
- Kind (e.g. `command_run`)
- Command (redacted — secrets scrubbed before this row is written)
- Exit code, duration, bytes emitted
- Termination reason (completed, canceled, killed, timeout, truncated)
- SHA-256 of the output blob
- `prev_hash` + `row_hash` (the chain)

Only `audit_outputs.blob` varies by mode. That's the gzipped,
redacted, ANSI-stripped version of the command's combined
stdout+stderr.

## `always`

The forensic default. Every command's output is captured:

1. Collected in the Job buffer (capped at `max_output_bytes`).
2. ANSI terminal escapes stripped (so `zcat` in 5 years doesn't
   clear your screen).
3. Run through the redactor — AWS keys, tokens, JWTs, private
   keys, common password patterns, etc. See
   [../security/secret-redaction.md](../security/secret-redaction.md).
4. Gzipped.
5. Checked against `audit_max_blob_bytes`. Oversized → blob
   dropped, audit row keeps metadata + `detail: output_oversized`.
6. Inserted into `audit_outputs`.

Reading back:

```bash
shellboto audit search --limit 5
# pick an ID, then
sudo sqlite3 /var/lib/shellboto/state.db \
    "SELECT length(blob) FROM audit_outputs WHERE audit_event_id = 1234;"
# or via the bot:
/audit-out 1234
```

## `errors_only`

Blob stored only when `exit_code != 0`.

Successful commands (by far the majority) take only metadata.
Failed commands get full forensic blob.

Good for teams that run a lot of routine successful commands whose
output is boring (ls, ps, df, grep in logs) and don't want to
persist all of it — but do want failures for post-mortems.

## `never`

Metadata only. No blob ever stored.

The audit row still has `output_sha256` — computed over the empty
blob? No: over the **redacted** output, before drop. That means:

- You can still verify later (via a separate journald log with
  `log_format=json`, `log_level=info` — every command prints a
  structured log line that could be grep'd and hashed against
  `output_sha256`) that a given chunk of output matches what the
  audit row claims.
- But you can't **recover** the output from the DB.

Use when:

- Commands routinely contain high-sensitivity data that redaction
  might miss (novel token formats, PII, regulated data).
- You accept losing forensic replay in exchange.
- You probably still want `log_format=json` → journald for an
  ephemeral mirror that rotates.

## The SHA-256 is always computed

Regardless of mode, `output_sha256` is computed from the redacted
output bytes (the same bytes that would have been written, had
mode allowed it). This means:

- The hash chain still attests to output integrity — it's part of
  the canonical-form hash.
- If you later upgrade an `errors_only` or `never` deployment to
  `always`, *future* rows get blobs, but **past rows are still
  verifiable** against their SHA-256 if you happen to have a
  journald archive of what was emitted.

## `audit_max_blob_bytes` — the post-redact cap

Even in `always` mode, a single captured output might be huge —
e.g. `find /` on a box with millions of files. `audit_max_blob_bytes`
(default 50 MiB, same as `max_output_bytes`) caps the stored blob:

- Blob ≤ cap: stored.
- Blob > cap: dropped; audit row still has metadata +
  `detail: output_oversized`.
- **`0`**: no separate cap (relies solely on `max_output_bytes`).

Recommendation: set `audit_max_blob_bytes` a bit smaller than
`max_output_bytes` — runtime OOM protection at one cap, DB-bloat
protection at a tighter one.

## Interaction with `max_output_bytes`

`max_output_bytes` is a **runtime** buffer cap. When the bot's
Job buffer hits it, bash's foreground process is SIGKILL'd and the
command row gets `termination=truncated`.

`audit_max_blob_bytes` is a **storage** cap. Applied after the
buffer is final.

Typical values:

```toml
max_output_bytes = 52428800        # 50 MiB — runtime
audit_max_blob_bytes = 52428800    # 50 MiB — storage
```

If you want stricter storage:

```toml
max_output_bytes = 52428800        # 50 MiB — allow the command to run to 50 MiB
audit_max_blob_bytes = 10485760    # 10 MiB — but only store up to 10 MiB
```

Everything between 10 MiB and 50 MiB is captured, streamed to the
user, then discarded before the audit write.

## Changing mode

Config change + restart:

```bash
sudo vi /etc/shellboto/config.toml
# set audit_output_mode = "errors_only"
sudo systemctl restart shellboto
```

Effect is **prospective**. Past rows are unchanged. The hash chain
carries over cleanly — it doesn't care about mode changes.

## Doctor check

```bash
shellboto doctor | grep audit_output
# audit_output_mode = always   ✅
```

Prints your current setting.

## Read next

- [../security/audit-chain.md](../security/audit-chain.md) — what
  the hash chain buys you.
- [../audit/output-storage.md](../audit/output-storage.md) — the
  storage path in detail.
