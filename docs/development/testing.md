# Testing

## The rule

**Don't mock the database.** Use a real SQLite via `t.TempDir()`.

From `CONTRIBUTING.md`:

> Don't mock the database in integration tests. Use a temp
> SQLite via the existing `newTestRepo(t)` helpers.

Mock databases hide real bugs — schema changes, constraint
violations, index-missing-slow queries, driver quirks. Real
databases in tests catch them.

## The helpers

`internal/db/repo` exposes a `newTestRepo(t)` helper that returns
a freshly-initialised temp SQLite + both repos wired up:

```go
func newTestRepo(t *testing.T) (*AuditRepo, *UserRepo, *gorm.DB) {
    t.Helper()
    dir := t.TempDir()
    gormDB, err := db.Open(filepath.Join(dir, "test.db"))
    if err != nil { t.Fatal(err) }
    t.Cleanup(func() { _ = db.Close(gormDB) })
    if err := db.AutoMigrate(gormDB); err != nil { t.Fatal(err) }
    seed := make([]byte, 32)
    audit := repo.NewAuditRepo(gormDB, seed, zap.NewNop().Named("audit"), "always", 0)
    users := repo.NewUserRepo(gormDB)
    return audit, users, gormDB
}
```

Use it. Don't write your own.

## Shell pty tests

`internal/shell/shell_test.go` forks real bash processes:

```go
func TestShellRun(t *testing.T) {
    if runtime.GOOS != "linux" {
        t.Skip("pty tests Linux-only")
    }
    mgr := shell.NewManager(...)
    sh, err := mgr.GetOrSpawn(42)
    if err != nil { t.Fatal(err) }
    defer sh.Close()

    job, err := sh.Run("echo hello")
    if err != nil { t.Fatal(err) }

    select {
    case exit := <-job.Done:
        if exit != 0 { t.Fatalf("exit=%d", exit) }
    case <-time.After(5*time.Second):
        t.Fatal("timeout")
    }

    out, _ := job.Snapshot()
    if !bytes.Contains(out, []byte("hello")) {
        t.Fatalf("bad output: %q", out)
    }
}
```

Tests the full path: spawn, write, PROMPT_COMMAND, ctrl pipe,
finalise. Real bash catches kernel-level corner cases that a
mocked `pty` would miss.

## Concurrency in tests

`go test -race ./...` on every CI run. Catches data races
deterministically — if a test passes locally without `-race`
but would race, CI catches it.

Don't use `time.Sleep` for synchronisation in tests. Use
channels + `select` + `time.After(5*time.Second)` as a timeout
fallback:

```go
select {
case <-eventChan:
    // ok
case <-time.After(5 * time.Second):
    t.Fatal("timed out waiting for event")
}
```

## Table-driven tests

Common pattern, especially for `danger`, `redact`, `config`:

```go
func TestDangerMatch(t *testing.T) {
    cases := []struct {
        name    string
        cmd     string
        want    bool
    }{
        {"rm -rf /", "rm -rf /", true},
        {"rm -rf path", "rm -rf /tmp/foo", true},
        {"ls", "ls -la", false},
        {"unicode rm", "rm\u200Brf /", false},  // zero-width space
    }
    m, _ := danger.New(nil)
    for _, c := range cases {
        t.Run(c.name, func(t *testing.T) {
            _, got := m.Match(c.cmd)
            if got != c.want { t.Errorf("got %v, want %v", got, c.want) }
        })
    }
}
```

## Fixtures

For the redactor tests, real-shaped secrets are used (fake values
but real regex shape). The `.gitleaks.toml` allowlist has
`internal/redact/redact_test.go` in its paths so gitleaks doesn't
fire on these fixtures.

## Shell-script tests

`deploy/lib_test.sh` is a bespoke bash test runner for
`deploy/lib.sh` helpers. Run:

```bash
make test-deploy
```

## Integration tests

No separate `integration/` directory. Integration tests live
alongside unit tests, tagged with build constraints where
needed:

```go
//go:build integration

package db_test
```

Then `go test -tags integration ./...` runs them. CI runs without
`-tags integration` by default (these tend to be slow / flaky).

## Benchmarks

Sparse. Add them when reasoning about perf:

```go
func BenchmarkRedactLarge(b *testing.B) {
    input := generateLargeInput()
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _ = redact.Redact(input)
    }
}
```

Run:

```bash
go test -bench=. ./internal/redact/
```

## What we deliberately DON'T test

- **The Telegram Bot API.** No mock; no fake Telegram. Handler
  logic is tested against `deps.Deps` with stubbed helpers.
- **The production DB schema through ORM round-trip.** Caught by
  GORM's auto-migrator at startup — if a struct is wrong, the
  bot fails on boot.
- **systemd's own behaviour.** `systemctl start shellboto` works
  because systemd works; we don't own the test surface.

## Coverage

```bash
go test -coverprofile=/tmp/cov ./...
go tool cover -html=/tmp/cov -o /tmp/cov.html
firefox /tmp/cov.html
```

No coverage gate in CI. Coverage is a signal, not a goal.
Meaningful tests > high coverage.

## Read next

- [build-from-source.md](build-from-source.md) — `make test`.
- [ci.md](ci.md) — what CI runs.
