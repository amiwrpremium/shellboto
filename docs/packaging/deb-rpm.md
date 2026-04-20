# `.deb` and `.rpm`

goreleaser uses embedded nfpm to produce Debian and RPM packages.
Same inputs per-format; different output layouts.

## What's in the package

- `/usr/local/bin/shellboto` (mode 0755).
- `/etc/shellboto/config.toml.example` (mode 0600, marked
  `config|noreplace` — upgrades don't clobber).
- `/etc/shellboto/env.example` (mode 0600, `config|noreplace`).
- `/lib/systemd/system/shellboto.service` (mode 0644).

Note: the `.example` suffix is deliberate. nfpm **doesn't** drop
a live `config.toml` — it drops the example. The `postinstall.sh`
hook copies example → live if no live file exists.

## Installation

### .deb

```bash
sudo apt install ./shellboto_0.2.0_linux_amd64.deb
# or
sudo dpkg -i ./shellboto_0.2.0_linux_amd64.deb
```

### .rpm

```bash
sudo dnf install ./shellboto-0.2.0-1.x86_64.rpm
# or
sudo rpm -ivh ./shellboto-0.2.0-1.x86_64.rpm
```

Both run the postinstall hook.

## postinstall

`packaging/postinstall.sh` runs on install + upgrade. It:

1. Creates `/etc/shellboto/` (0700, root-owned) if absent.
2. Copies `config.toml.example` → `config.toml` if no config
   exists yet (fresh install). On upgrades, the existing config
   is preserved.
3. Same for `env.example` → `env`.
4. `systemctl daemon-reload`.
5. Prints first-use hints:
   ```
   Edit /etc/shellboto/env with your token + superadmin ID.
   Run: sudo systemctl enable --now shellboto
   Run: shellboto doctor
   ```

It does **not** auto-start the service. That's a conscious step
for the operator. If you want auto-start on install, add a
`systemctl preset enable shellboto` to your own post-install
tooling.

## preremove

`packaging/preremove.sh` runs before package removal:

1. Stop the service (`systemctl stop shellboto` if active).
2. Disable the unit (`systemctl disable shellboto`).

It does **not** remove config or state. Both `/etc/shellboto/`
and `/var/lib/shellboto/` are preserved across `apt remove` /
`dnf remove`.

For full purge:

```bash
sudo apt purge shellboto
sudo rm -rf /etc/shellboto /var/lib/shellboto
```

(or the equivalent on dnf).

## Dependencies

Declared in `.goreleaser.yaml`:

- **Dependencies**: `systemd` (for the `.service` file to be
  meaningful).
- **Recommends**: `openssl` (for `shellboto mint-seed` flow and
  installer's own seed generation).

## Package section

- `section: admin` — both formats. Matches similar tools
  (fail2ban, monit, watchdog). Shows up under the admin category
  in package-browser UIs.

## `config|noreplace`

nfpm's flag for "treat this file as config; don't clobber on
upgrade if the user edited it." For `.deb` this maps to Debian
conffiles; for `.rpm` to `%config(noreplace)`. Upgrades that
change `config.toml.example` don't touch the user's live
`config.toml`.

If the bot ships a new config key with a default, old configs
still work (unknown keys are accepted; missing keys use defaults).
If the bot ships a **breaking** config change, CHANGELOG flags
it; operator upgrades manually via a separate `install.sh` run
after reviewing.

## Hosting a repo

Goreleaser can optionally push .deb/.rpm to various apt/yum
repositories. We don't configure that — GitHub Releases is the
distribution endpoint. Your CI / Ansible pulls the file from the
release page.

If you want a proper repo, wire your own via `reprepro` (apt) or
`createrepo` (yum) + nginx; point your hosts at it.

## Signing

Neither `.deb` nor `.rpm` are signed by goreleaser. `dpkg` and
`rpm` will accept unsigned packages unless you've configured
strict verification.

To sign, you'd add `signs:` block to `.goreleaser.yaml` + manage
GPG keys in CI. Not shipped today; the TODO in
`.goreleaser.yaml` tracks it.

## Reading the code

- `packaging/postinstall.sh`
- `packaging/preremove.sh`
- `.goreleaser.yaml` `nfpms:` section.

## Read next

- [homebrew.md](homebrew.md) — the other distribution path.
- [../deployment/installer.md](../deployment/installer.md) — the
  install.sh path, which does things .deb/.rpm can't (prompts,
  audit seed generation).
