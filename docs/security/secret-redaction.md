# Secret redaction

Every `cmd` string and every output blob goes through a redactor
before being written to the audit log. The redactor is a list of
regex patterns, each paired with a replacement. If a match fires,
the matched span is replaced with a placeholder like
`[REDACTED-JWT]`.

Source: [`internal/redact/redact.go`](../../internal/redact/redact.go).

## API

```go
func Redact(b []byte) []byte             // returns a scrubbed copy; input unchanged
func RedactString(s string) string        // convenience wrapper
func StripTerminalEscapes(b []byte) []byte // removes ANSI + BEL
func StripTerminalEscapesString(s string) string
```

Both `Redact` and `StripTerminalEscapes` run on output before
storage. `Redact` also runs on `cmd` (the shell command itself) —
so a user who pastes `export API_KEY=xyz` has both the command and
its confirmation in logs scrubbed.

## The pattern list

In declared order (earlier patterns run first):

### 1. SSH/TLS private keys

```
(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----
```

Replacement: `[REDACTED-PRIVATE-KEY]`

Matches the full PEM block (including the header, body, footer) for
any `-----BEGIN *PRIVATE KEY-----` variant: `RSA PRIVATE KEY`,
`OPENSSH PRIVATE KEY`, `EC PRIVATE KEY`, `PRIVATE KEY` (PKCS#8),
etc. The `(?s)` dotall flag lets `.*?` cross newlines.

Example trigger:

```
-----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAACFwAAAAdzc2gtcn
[...]
-----END OPENSSH PRIVATE KEY-----
```

becomes:

```
[REDACTED-PRIVATE-KEY]
```

### 2. Database connection URIs

```
\b(postgres|postgresql|mongodb|mongodb\+srv|mysql|mariadb|redis|amqp|amqps)://[^:/\s@]+:([^@\s/]+)@
```

Replacement: `$1://[REDACTED]:[REDACTED]@`

Example: `postgres://alice:swordfish@db.example:5432/app` becomes
`postgres://[REDACTED]:[REDACTED]@db.example:5432/app`.

### 3. HTTP authorisation headers

```
(?i)(authorization:\s*(bearer|basic))\s+\S+
```

Replacement: `$1 [REDACTED]`

Example: `Authorization: Bearer eyJhbG...` becomes
`Authorization: Bearer [REDACTED]`.

### 4. JWTs (anywhere)

```
\beyJ[A-Za-z0-9_\-]+\.[A-Za-z0-9_\-]+\.[A-Za-z0-9_\-]+\b
```

Replacement: `[REDACTED-JWT]`

Any three-segment base64url blob starting with `eyJ` (the base64 of
`{"`). Catches bare JWT values anywhere in output, not just in
Authorization headers.

### 5. GitHub classic personal access tokens

```
\bghp_[A-Za-z0-9]{36}\b
```

Replacement: `[REDACTED-GH-TOKEN]`

### 6. GitHub fine-grained personal access tokens

```
\bgithub_pat_[A-Za-z0-9_]{22,}\b
```

Replacement: `[REDACTED-GH-PAT]`

### 7. GitHub OAuth tokens

```
\bgho_[A-Za-z0-9]{36}\b
```

Replacement: `[REDACTED-GH-OAUTH]`

### 8. GitHub app secrets

```
\bghs_[A-Za-z0-9]{36}\b
```

Replacement: `[REDACTED-GH-SECRET]`

### 9. GitLab personal access tokens

```
\bglpat-[A-Za-z0-9_\-]{20,}\b
```

Replacement: `[REDACTED-GL-PAT]`

### 10. Google API keys

```
\bAIza[0-9A-Za-z_\-]{35}\b
```

Replacement: `[REDACTED-GOOGLE-KEY]`

### 11. AWS access keys

```
\bAKIA[0-9A-Z]{16}\b
```

Replacement: `[REDACTED-AWS-KEY]`

AWS **secret** keys are base64-shaped 40 chars; shape alone doesn't
distinguish them from other random strings, so we don't target
those directly. Rely on key-value patterns (#14) to catch
`AWS_SECRET_ACCESS_KEY=...` assignments.

### 12. Stripe API keys

```
\b(sk|pk|rk)_(live|test)_[A-Za-z0-9]{24,}\b
```

Replacement: `[REDACTED-STRIPE]`

Matches `sk_live_*`, `pk_test_*`, `rk_live_*`, etc.

### 13. Slack tokens

```
\bxox[abopsr]-[A-Za-z0-9-]{10,}\b
```

Replacement: `[REDACTED-SLACK]`

Slack's `xoxb-` (bot), `xoxa-` (app), `xoxp-` (user), `xoxs-`
(session) etc.

### 14. `mysql -pPASSWORD` flag (≥4 chars)

```
(\s|^)-p([^\s=]{4,})
```

Replacement: `$1-p[REDACTED]`

Catches `mysql -pSecret123` but not `ls -p` (no password argument).
The 4-char minimum avoids false positives on common short flag
values. Note this runs after the GitHub patterns — a token that
starts with `-p` wouldn't word-break-match here anyway.

### 15. `--password=` and `-password=`

```
(?i)--?pass(word)?\s*=\s*\S+
```

Replacement: `--password=[REDACTED]`

Covers `--password`, `--pass`, `-password`, `-pass`, case-insensitive.

### 16. Generic `KEY=VALUE` secrets

```
(?i)(\b[A-Z0-9_]*(TOKEN|SECRET|APIKEY|API_KEY|ACCESS_KEY|PRIVATE_KEY|PASSWORD|PASSWD|PASSPHRASE|CREDENTIAL|CREDENTIALS)[A-Z0-9_]*)\s*[:=]\s*\S+
```

Replacement: `$1=[REDACTED]`

Examples:

- `AWS_SECRET_ACCESS_KEY=wJalr...`
- `MY_APP_API_KEY=abc123`
- `TELEGRAM_TOKEN: xyz`
- `DB_PASSWORD=hunter2`

Case-insensitive on the inner keyword; the surrounding name is kept
so context is preserved. Works for both `=` (shell) and `:` (YAML,
JSON, config files) delimiters.

### 17. Unix password-hash entries

```
(^|\n)([a-z_][a-z0-9_-]*):\$[1-9][aby]?\$[^:\s]+
```

Replacement: `$1$2:[REDACTED-HASH]`

Example: `root:$6$abc123$longhash:19735:0:99999:7:::` becomes
`root:[REDACTED-HASH]`.

Catches shadow-file format with modern hash IDs (`$1$` MD5,
`$2a$`/`$2b$`/`$2y$` bcrypt, `$5$` SHA256, `$6$` SHA512, `$7$`
scrypt, `$9$` argon2). Line-anchored so normal text with
`user:$value` outside shadow-file context isn't affected.

## Terminal-escape stripping

Before `Redact`, outputs also go through `StripTerminalEscapes`:

```
\x1b(?:\[[0-?]*[ -/]*[@-~]|\][^\x07\x1b]*(?:\x07|\x1b\\)|[ -/]*[0-~])|\x07
```

Matches:

- **CSI** sequences (`\x1b[…`) — colour codes, cursor movement.
- **OSC** sequences (`\x1b]…`) — window titles, hyperlinks.
- **Single-byte C1** escapes — mode switches.
- **BEL** (`\x07`) — alert beep.

Without this, `zcat audit.txt.gz` on a captured output could rewrite
your terminal title, clear the screen, or spam beeps. The stored
blob is plain text.

## Ordering matters

Patterns run in declared order. Earlier matches consume the bytes;
later patterns see the post-replacement string. So:

1. Private keys eaten first (whole PEM block).
2. Specific provider tokens (GitHub, Stripe, AWS, Google, Slack,
   GitLab) matched by shape.
3. Generic HTTP auth, DB URIs, mysql `-p`, `--password=`.
4. Generic `KEY=VALUE` last-resort.
5. Shadow hashes last.

This ordering prevents e.g. the generic `TOKEN=` catch-all from
blurring a provider-specific `ghp_...` into the opaque
`[REDACTED]` instead of the more informative `[REDACTED-GH-TOKEN]`.

## Known limitations

- **Novel secret formats are not covered.** If your system uses a
  custom token shape not in this list, it won't be caught.
- **Secrets split by user commands.** If a user pipes
  `echo TOKEN=$CRED | …`, the `TOKEN=` matches. If they write
  `echo $CRED` (no key name), it doesn't — the value is unshaped.
- **Secrets in structured data.** JSON / YAML values that don't
  match one of the provider shapes + don't have a secret-ish key
  name can slip through.
- **Partial matches.** Some patterns match only the value, some
  only the header+value (e.g. `-p[REDACTED]` replaces the value
  but keeps `-p` visible). Parse the placeholder pattern if you
  need to know which kind you've got.

If you need stronger guarantees: `audit_output_mode = never`. The
blob isn't stored at all. The hash chain still records the output
SHA-256, so you retain integrity (given a journald mirror you can
correlate against) without storage exposure.

## Testing the redactor

There's no `shellboto simulate-redact` command. The redactor is
covered by `internal/redact/redact_test.go` with ~12 tables of
positive and negative cases. To test a custom input:

```bash
git clone https://github.com/amiwrpremium/shellboto.git
cd shellboto
cat > /tmp/r.go <<'EOF'
package main

import (
    "fmt"
    "github.com/amiwrpremium/shellboto/internal/redact"
)

func main() {
    input := "AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
    fmt.Println(string(redact.Redact([]byte(input))))
}
EOF
go run /tmp/r.go
```

Prints: `AWS_SECRET_ACCESS_KEY=[REDACTED]`.

## Contributing a new pattern

If you find a secret shape we miss:

1. Add the regex + replacement to the `patterns` slice in
   `internal/redact/redact.go`.
2. Add a positive test case (matches something realistic) and a
   negative case (doesn't false-positive on similar-shaped non-
   secrets) to `redact_test.go`.
3. `go test ./internal/redact/...` must pass.
4. Open a PR with `feat(redact): add <provider> token pattern` as
   the Conventional Commit title.

## Read next

- [audit-chain.md](audit-chain.md) — the chain that attests to the
  redacted content.
- [../configuration/audit-output-modes.md](../configuration/audit-output-modes.md)
  — the storage-exposure trade-off.
