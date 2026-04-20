# Bad release in production

**Symptom**: the service was healthy before the last release; now
it's crashing, looping, or misbehaving after the goreleaser-
published update.

## 1. Roll back, then investigate

```bash
# Confirm the bad state.
systemctl status shellboto
journalctl -u shellboto -n 100 --no-pager

# Swap back to the previous binary. Atomic; reversible.
sudo ./deploy/rollback.sh

# Confirm the service is healthy on the previous version.
systemctl status shellboto
shellboto doctor
```

`rollback.sh` works because `install.sh` saves the previous
binary as `/usr/local/bin/shellboto.prev` on every install. See
[../deployment/rollback.md](../deployment/rollback.md).

## 2. Yank the bad release on GitHub

Optional but helpful so new installers don't pick the broken
artifact:

1. Open
   `github.com/amiwrpremium/shellboto/releases` → edit the bad
   release.
2. Tick **"Set as a pre-release"** (or mark draft).
3. Leave a short note explaining why.

The tag itself isn't deleted — future fix commits land normally;
release-please opens the next release PR once fixes are merged.

## 3. Fix forward

1. Open a PR that reverts (or fixes) the offending commit.
2. Land it on `master`. release-please will open a new release
   PR.
3. Merge the release PR → new tag → goreleaser ships the fix.

`sudo ./deploy/rollback.sh` is reversible — re-run it on the host
to flip back to the new binary once you've verified the fix.

## 4. Communicate

- Tell whoever uses the bot that something happened.
- Add a `CHANGELOG.md` note for the fix release describing what
  broke + what was fixed.

## What if rollback also fails?

The `.prev` is a working binary as of its install time. If
`rollback.sh` errors:

```bash
sudo ls -la /usr/local/bin/shellboto*
```

Check both files exist. If only `shellboto` is there (no
`.prev`), this is the **first** install — there's nothing to roll
back to. Build from a known-good tag:

```bash
cd /root/shellboto
git fetch --tags
git checkout v0.1.0          # last known good tag
make build
sudo ./deploy/install.sh
```

## After the dust settles

- Open a follow-up issue describing the regression for posterity.
- Consider whether your CI gate (`make release-check`) caught it
  or should have. Add a test that would have.
- If the regression went undetected for a while, consider a
  scheduled smoke test (cron sending `/start` from a test
  account, alerting on no-reply).

## Read next

- [../deployment/rollback.md](../deployment/rollback.md) —
  rollback mechanics.
- [../development/releasing.md](../development/releasing.md) —
  the release pipeline.
