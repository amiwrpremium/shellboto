# Updating

Upgrading to a newer shellboto version.

## Source-build + installer (most common)

```bash
cd /root/shellboto     # or wherever your checkout is
git fetch
git checkout v0.2.0    # or `master` if you track main line
make build
sudo ./deploy/install.sh
```

The installer:

- Detects the existing env + config, keeps them.
- Saves the old binary to `shellboto.prev` (rollback target).
- Installs the new one.
- Restarts the service.
- Runs `shellboto doctor`.

Typical downtime: ~5 seconds during the restart.

## `.deb` / `.rpm` upgrade

```bash
sudo apt install ./shellboto_0.2.0_linux_amd64.deb
# or
sudo dnf install ./shellboto-0.2.0-1.x86_64.rpm
```

Package system handles stop → install → daemon-reload. You still
need to `systemctl start shellboto` (postinstall doesn't
auto-enable, by design — forces conscious "activate" step).

If upgrading:

```bash
sudo apt install --only-upgrade shellboto=0.2.0-1
sudo systemctl restart shellboto
```

## Homebrew (macOS — CLI only)

```bash
brew update
brew upgrade shellboto
```

Note: the macOS build is CLI subcommands only (`audit verify`,
`db backup`, etc.). The bot itself runs on Linux only.

## Pre-upgrade steps (recommended)

1. Back up the DB:

   ```bash
   sudo shellboto db backup /var/backups/shellboto/pre-upgrade-$(date -I).db
   ```

2. Note the current version:

   ```bash
   shellboto -version
   ```

3. Review `CHANGELOG.md` for breaking changes between your
   current version and the target.

## During upgrade

- Users will see their shells tear down.
- New messages during the restart window get queued by
  Telegram's Bot API and delivered once the bot comes back (via
  `getUpdates` offset).
- No messages are lost.

## Post-upgrade steps

```bash
sudo systemctl status shellboto
sudo shellboto doctor
sudo shellboto audit verify
```

Then a test message to the bot.

## If the upgrade breaks things

```bash
sudo ./deploy/rollback.sh
```

Swaps back to `shellboto.prev`. Service restart. Done in seconds.

See [rollback.md](../deployment/rollback.md).

## Schema-migration considerations

shellboto's schema changes are additive-only (see
[../database/migrations.md](../database/migrations.md)).
Upgrades:

- Add new columns on open.
- Won't drop columns, so downgrading (via rollback) to an older
  binary against a newer schema is safe — the older binary just
  ignores columns it doesn't know about.

The one thing to watch: if an upgrade added a column that the new
code writes and then you rollback, new writes from the new
binary's brief window aren't visible to the old binary (it can't
read the new column). Acceptable; the audit hash chain still
verifies.

## Automated upgrade via CI

release-please publishes tags → goreleaser builds → your infra
picks up:

- Pull from GitHub Releases API.
- Run `./install.sh --skip-build` with the new binary.

Or host a local apt/dnf repo mirroring the goreleaser-built
packages; configure unattended-upgrades with holding rules so
shellboto doesn't update mid-business-hours without approval.

## Staging → prod

If you run multiple VPSes (e.g. dev + prod):

1. Tag a release.
2. Dev host picks up automatically (unattended-upgrades or CI).
3. Verify: run `shellboto doctor` + send test messages.
4. Promote to prod: `apt install shellboto=<new-version>` on
   prod.

Don't auto-upgrade prod without a soak period.

## When to hold back on upgrades

- Within the first hour of a new release — let others find the
  bugs first.
- During incidents — focus on the incident; don't introduce new
  variables.
- Before critical ops windows — if you need the bot to work at
  2 AM Thursday, don't upgrade at 1 AM Thursday.

## Read next

- [../deployment/rollback.md](../deployment/rollback.md) — the
  undo.
- [monitoring.md](monitoring.md) — post-upgrade, watch for
  anomalies.
