# Output buffering

How per-command output is captured, capped, and fed to the
streamer.

## The Job

When a command starts, `Shell.Run(cmd)` creates a `Job`:

```go
type Job struct {
    Cmd     string
    Started time.Time
    mu      sync.Mutex
    buf     bytes.Buffer          // accumulated output
    version uint64                // increments each write
    Done    chan int              // buffered capacity 1
    exitCode, finishedAt atomic.Int64
    termSet atomic.Pointer[string]  // canceled / killed / timeout / completed / truncated
    MaxBytes  int                 // per-Job cap; 0 = unlimited
    truncated atomic.Bool         // set when MaxBytes hit
}
```

`Shell.current.Store(j)` installs it as the active Job. `Run`
writes the user's command + `\n` to the pty.

## Appending bytes

`Shell.readPty` loops:

```go
for {
    n, err := s.pty.Read(buf)
    if n > 0 {
        s.lastAct.Store(time.Now().UnixNano())
        s.flushOutput(buf[:n])
    }
    if err != nil {
        s.Close()
        return
    }
}
```

`flushOutput` calls `currentJob.Write(bytes)` which:

1. Takes the Job's mutex.
2. Appends to `buf`.
3. Increments `version`.
4. Releases the mutex.

Streamer goroutine in parallel calls `Job.Snapshot()`:

```go
func (j *Job) Snapshot() ([]byte, uint64) {
    j.mu.Lock()
    defer j.mu.Unlock()
    out := make([]byte, j.buf.Len())
    copy(out, j.buf.Bytes())
    return out, j.version
}
```

Returns a copy, so streamer can render without holding the
lock.

## The `MaxBytes` cap (`max_output_bytes`)

Default 50 MiB. Set at Job creation from `Shell.maxOutputBytes`,
which comes from config.

Inside `Job.Write`:

```go
if j.MaxBytes > 0 {
    remaining := j.MaxBytes - j.buf.Len()
    if remaining <= 0 {
        j.truncated.Store(true)
        return true                // truncated flag set; caller kills
    }
    if len(p) > remaining {
        j.buf.Write(p[:remaining]) // partial write
        j.truncated.Store(true)
        return true
    }
}
j.buf.Write(p)
```

Return `truncated=true` signals the reader: buffer is full. The
reader goroutine then calls `Shell.SigKill()` to SIGKILL the
foreground process group — stops the flood.

`killOnOverflow` atomic ensures we only send SIGKILL once per Job:

```go
if truncated && j.killOnOverflow.CompareAndSwap(false, true) {
    go s.SigKill()
}
```

The `go` is because `SigKill` can block briefly on the ioctl; we
don't want to stall the reader loop.

## Why a cap matters

Without it, `cat /dev/urandom` or `yes` would grow the buffer
unbounded until the process OOMs. 50 MiB is:

- Enough for any realistic output (log tails, large directory
  listings, big `ps -ef` dumps, etc).
- Small enough that one runaway command can't take the bot out.

## Interaction with `audit_max_blob_bytes`

`max_output_bytes` is the **runtime** cap (Job.Write). Once the
Job finalises:

- `audit_max_blob_bytes` is the **storage** cap (applied after
  redaction + gzip, before the audit blob write).
- Blob > cap → dropped; audit row keeps metadata + `detail:
  output_oversized`.

Typical config:

```
max_output_bytes    = 52428800   # 50 MiB — runtime
audit_max_blob_bytes = 52428800  # 50 MiB — storage
```

Tighter storage than runtime:

```
max_output_bytes    = 52428800   # 50 MiB allowed during run
audit_max_blob_bytes = 10485760  # 10 MiB — but only store 10 MiB
```

10 MiB of blob stored; the other 40 MiB was streamed to the user
but not persisted. Good for privacy-minded deployments that still
want forensics on small outputs.

## Truncation → audit row

On finalise:

- If `Job.truncated.Load()` is true: audit row's `termination` is
  `truncated`.
- The Telegram message footer shows `⚠ output capped`.
- `BytesOut` in the audit row reflects the full output size
  including the truncated portion (the bytes that tried to write
  but couldn't are counted too, so forensics knows the scale).

## Snapshot vs. final

The streamer takes snapshots (copies) of the buffer on each tick.
After `Job.Done`, shellboto takes a **final** snapshot and uses
that for the audit blob. The final snapshot sees the full buffer
at finalise-time.

Edge case: if output arrives after Job finalisation (shouldn't
happen past the 30 ms drain window, but in theory), it's lost —
the Job is considered sealed.

## Reading the code

- `internal/shell/shell.go:Job` — struct + methods.
- `shell.go:Job.Write` — the append path + truncation logic.
- `shell.go:Job.Snapshot` / `Job.finish` — streamer-facing API.

## Read next

- [../telegram/streaming-output.md](../telegram/streaming-output.md)
  — how the buffer contents become Telegram messages.
- [signals.md](signals.md) — what "SIGKILL on overflow" actually
  does.
