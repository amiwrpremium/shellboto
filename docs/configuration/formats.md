# Config file formats

shellboto supports three formats. Pick one; stick with it.

| Format | File extension | Parser | When to use |
|--------|----------------|--------|-------------|
| TOML | `.toml` | [BurntSushi/toml](https://github.com/BurntSushi/toml) | Default. Commented examples, human-writable, unambiguous syntax. |
| YAML | `.yaml` or `.yml` | [gopkg.in/yaml.v3](https://pkg.go.dev/gopkg.in/yaml.v3) | Your org runs on YAML config everywhere else. |
| JSON | `.json` | stdlib `encoding/json` | Machine-generated. Infrastructure-as-code outputs JSON and you'd rather not transcode. |

The format is picked from the file extension. Unknown extensions
fail fast with a clear error — no silent fallback.

## The schema is identical

All three formats parse into the same Go struct. A TOML and a YAML
config with the same keys + values are behaviourally identical.

TOML:

```toml
db_path = "/var/lib/shellboto/state.db"
audit_retention = "2160h"
default_timeout = "5m"

audit_output_mode = "always"
extra_danger_patterns = [
  '\bnewgrp\b',
  '\bvisudo\b',
]
```

Equivalent YAML:

```yaml
db_path: /var/lib/shellboto/state.db
audit_retention: 2160h
default_timeout: 5m

audit_output_mode: always
extra_danger_patterns:
  - '\bnewgrp\b'
  - '\bvisudo\b'
```

Equivalent JSON:

```json
{
  "db_path": "/var/lib/shellboto/state.db",
  "audit_retention": "2160h",
  "default_timeout": "5m",
  "audit_output_mode": "always",
  "extra_danger_patterns": [
    "\\bnewgrp\\b",
    "\\bvisudo\\b"
  ]
}
```

Note JSON requires escaping backslashes in regex strings.

## Durations: `"5m"`, `"30s"`, `"2h"`

Go's `time.Duration` string format works across all three parsers:
`500ms`, `30s`, `5m`, `2h`, `72h`, `2160h`, etc. Invalid duration
strings fail at config-load time with a clear error pointing at the
key.

Not supported: `"5 minutes"`, `"PT5M"`, bare integer seconds.

## Booleans and numbers

| Format | Booleans | Integers | Floats |
|--------|----------|----------|--------|
| TOML | `true` / `false` | `42` | `1.5` |
| YAML | `true` / `false` | `42` | `1.5` |
| JSON | `true` / `false` | `42` | `1.5` |

YAML's old true-synonyms (`yes`, `on`, `y`) are **not** accepted by
`yaml.v3` in the default decoder. Write `true`/`false` and move on.

## Arrays / lists

Used for `extra_danger_patterns`. Already shown above.

TOML supports both inline arrays (`[ "a", "b" ]`) and multi-line:

```toml
extra_danger_patterns = [
  '\bnewgrp\b',
  '\bvisudo\b',
]
```

YAML uses the `- ` prefix. JSON uses `[]`.

## No env-var substitution in the config file

You cannot write:

```toml
db_path = "${STATE_DIR}/state.db"     # not a thing
```

The file is parsed as-is. Use the actual path.

## Examples shipped with the installer

The installer materialises one of these to
`/etc/shellboto/config.{ext}` based on your format choice:

- `deploy/config.example.toml`
- `deploy/config.example.yaml`
- `deploy/config.example.json`

All three are heavily commented (where the format allows — JSON
doesn't support comments, so the JSON example is minimal; check the
TOML/YAML for annotations).

## Converting between formats

If you started with TOML and want to flip to YAML:

1. Copy the file to `/etc/shellboto/config.yaml`.
2. Translate the syntax (any online converter works — or write it
   by hand; the schema is small).
3. `shellboto config check /etc/shellboto/config.yaml` — confirms
   the new file parses identically.
4. Remove the old `config.toml`.
5. Restart the service.

`shellboto doctor` won't be confused by a leftover `config.toml` if
your unit file's `-config` points at `config.yaml`, but leaving
both lying around is asking for confusion later. Delete the old.

## Read next

- [schema.md](schema.md) — every key, value, and default.
- [environment.md](environment.md) — the three env vars.
