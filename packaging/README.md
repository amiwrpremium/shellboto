# packaging/

Packaging assets consumed by [goreleaser] at release time:

- `postinstall.sh` / `preremove.sh` — lifecycle scripts for `.deb` /
  `.rpm`, wired via `.goreleaser.yaml`'s `nfpms.scripts` block.
- `homebrew/shellboto.rb` — reference formula. The actual tap copy is
  pushed automatically by goreleaser on each release (see the `brews:`
  block in `.goreleaser.yaml`). Keep this file in sync as
  documentation; goreleaser rewrites the tap-side copy verbatim.

[goreleaser]: https://goreleaser.com/

## Build flow

```bash
make release-snapshot     # local dry-run; artifacts land in dist/
make release-check        # lint + test + vet + vuln + goreleaser check
```

Real releases are driven by release-please — merge the release PR on
master and the tag it creates fires `release.yml` → goreleaser.
See `CONTRIBUTING.md` for the full flow.

After a release, `dist/` locally (or the GitHub release assets
remotely) will contain:

- `shellboto_<ver>_linux_amd64.tar.gz` + `_arm64`
- `shellboto_<ver>_darwin_amd64.tar.gz` + `_arm64`  (CLI-only)
- `shellboto_<ver>_linux_amd64.deb` + `_arm64`
- `shellboto_<ver>_linux_amd64.rpm` + `_arm64`
- `checksums.txt` (SHA-256)
- CycloneDX SBOM per archive

## Package contents

Both `.deb` and `.rpm` install:

- `/usr/local/bin/shellboto` (0755)
- `/etc/shellboto/config.toml.example` + `/etc/shellboto/env.example`
  (0600, marked `config|noreplace` so operator edits survive upgrades)
- `/lib/systemd/system/shellboto.service` (0644)

`postinstall.sh` creates `/etc/shellboto/` at 0700, copies the examples
into the live config paths on fresh installs, and runs `systemctl
daemon-reload`. Does NOT start the service (env placeholders need
filling first).

`preremove.sh` stops + disables the unit before removal. Preserves
`/etc/shellboto/env` and `/var/lib/shellboto/` (audit DB) so re-install
keeps history.

## Inspecting local artifacts

```bash
dpkg -I dist/shellboto*.deb           # metadata
dpkg -c dist/shellboto*.deb           # file list
rpm -qi --package dist/shellboto*.rpm # metadata
rpm -qlp --package dist/shellboto*.rpm
```

## Publishing

goreleaser creates the GitHub release with artifacts attached. Hosting
an `apt` or `yum` repository that serves them (reprepro, createrepo,
Cloudsmith, gemfury, self-hosted) is a downstream infra decision — the
files are ready to drop into whichever you choose.

## Homebrew tap

The `brews:` block in `.goreleaser.yaml` pushes `shellboto.rb` to
`amiwrpremium/homebrew-tap` on each release, **if** the
`HOMEBREW_TAP_GITHUB_TOKEN` repo secret is set with `repo` scope on
the tap repo. Without the secret, the rest of the release still
completes; only the tap push is skipped.

shellboto is Linux-first; the formula builds on macOS but runtime
features that depend on Linux-specific syscalls (non-root shell
isolation via `Credential{Uid}`, the TIOCGPGRP foreground-process
signal, flock on the instance lock) aren't portable. Use under
**linuxbrew** for a functional install; macOS is build-only.
