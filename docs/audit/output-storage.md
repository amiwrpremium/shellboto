# Output storage

How command stdout/stderr ends up in `audit_outputs` when
`audit_output_mode` permits.

## Pipeline

```
bash stdout+stderr → pty → Job.buf  (capped at max_output_bytes)
        │
        │   (command finishes; Job finalised)
        ▼
redact.StripTerminalEscapes  — ANSI / BEL removed
        │
        ▼
redact.Redact                 — secrets scrubbed
        │
        ▼
sha256(redacted) → output_sha256 column on audit_events
        │
        ▼
check audit_output_mode
        ├── never: stop; blob discarded
        ├── errors_only + exit=0: stop; blob discarded
        └── else: continue
        ▼
gzip (stdlib, default level)
        │
        ▼
check audit_max_blob_bytes
        ├── over cap: discard; audit_events.detail += "output_oversized"
        └── within cap: continue
        ▼
INSERT INTO audit_outputs (audit_event_id, blob)
```

## What's in the blob

- **Text, with escape codes and secrets stripped.** Not raw bash
  output.
- **UTF-8.** Invalid UTF-8 bytes from the pty are preserved as-is;
  Go's `bytes` operations are byte-safe.
- **Gzipped.** Level: default (6). `compress/gzip` stdlib.

When you fetch a blob:

```bash
sudo sqlite3 /var/lib/shellboto/state.db \
    "SELECT blob FROM audit_outputs WHERE audit_event_id = 1234;" \
    | zcat
```

or from Telegram (admin+):

```
/audit-out 1234
```

The bot decompresses and shows the text (or uploads as
`output.txt` if it exceeds Telegram's message cap).

## The output_sha256

Computed over the **redacted** output (post-strip, post-redact)
before gzip. Why:

- Redaction + strip are deterministic functions of the original
  output. Two identical commands produce identical redacted
  bytes, hence identical SHA.
- The chain's canonical form includes `output_sha256`. If anyone
  rewrites the blob, the chain doesn't match unless they also
  rewrite every subsequent row's chain values.

## Why gzip, not zstd

Stdlib. No extra dep. Outputs are usually <50 KB; the
compression-ratio difference is negligible. Ops tools universally
understand gzip.

## Why we strip ANSI before storing

Consider:

```
$ ls --color=always /tmp
\e[1m\e[34mfoo\e[0m \e[32mbar\e[0m
```

Raw blob has colour codes. If an operator later runs `zcat
audit-N.txt.gz` to inspect, their terminal gets the codes and
colours render (fine, maybe). But other sequences can:

- Clear the screen.
- Rewrite earlier lines.
- Retitle the window.
- Beep endlessly.
- Send OSC hyperlinks that point at attacker-chosen URLs.

Stripping is a hygiene measure — audit blobs are plain text,
safe to `cat` in any terminal.

## Why we redact before storing

Command output frequently leaks secrets:

```
$ aws sts get-caller-identity
{
  "Arn": "...",
  "Account": "..."
}

$ gh auth status
✓ Token: ghp_abcdefghijklmnopqrstuvwxyz...

$ curl -H "Authorization: Bearer $TOKEN" api.example.com
```

Without redaction, the DB becomes a trove of bearer tokens, API
keys, and SSH keys. The redactor catches ~17 common shapes (see
[../security/secret-redaction.md](../security/secret-redaction.md))
before the audit write.

Not a guarantee — novel shapes slip through. If your workflows
surface high-sensitivity secrets, use `audit_output_mode = never`.

## Blob retention

Pruner deletes `audit_events` rows older than `audit_retention`
(default 90 days). `audit_outputs` rows cascade via
`ON DELETE CASCADE`.

No separate blob-retention knob; the blob lives as long as its
event row does.

## Size tuning

Typical blob sizes:

- `ls /tmp`: < 100 bytes (compressed).
- `dmesg`: 10–100 KB.
- Multi-thousand-line log tail: 100 KB – 2 MB.
- `find /` with many matches: can hit 10s of MB.

The 50 MiB default cap (`audit_max_blob_bytes`) is generous for
most workloads. Tighten if:

- Disk is tight.
- You want audit rows for big commands but the blob itself is
  uninteresting.
- Commands routinely emit binary data (core dumps, hex dumps)
  that inflates blobs past useful ratios.

## Reading the code

- `internal/db/repo/audit.go:Log` — the redact → hash → gzip →
  insert pipeline.
- `internal/redact/redact.go` — strip + redact.

## Read next

- [retention.md](retention.md) — how the pruner works.
- [../configuration/audit-output-modes.md](../configuration/audit-output-modes.md)
  — the config switch that controls all of this.
