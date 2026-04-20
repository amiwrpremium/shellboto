# Homebrew

Distributes the macOS CLI binary via a Homebrew tap. Linux users
install via the installer or `.deb` / `.rpm`.

## What gets distributed

Only the CLI subcommands — `shellboto doctor`, `audit verify`,
`db backup`, etc. The bot itself doesn't run on macOS
(`internal/shell/` uses Linux-specific syscalls).

Useful when you administer a Linux VPS from a macOS workstation
— `brew install` + use the CLI against backups or via SSH.

## Tap setup (one-time)

For the operator:

1. Create an empty public repo `amiwrpremium/homebrew-shellboto`
   on GitHub. Don't add any files; goreleaser populates it.
2. Generate a fine-grained PAT:
   - Resource: `amiwrpremium/homebrew-shellboto` only.
   - Permissions: Contents: R/W.
   - Expiration: ≤ 1 year.
3. Add as `HOMEBREW_TAP_GITHUB_TOKEN` repo secret on the
   `shellboto` repo.

Once set, every release pushes a new formula file to the tap.

## Install (user-facing)

```bash
brew tap amiwrpremium/shellboto
brew install shellboto
```

Or in one line (when the formula name matches the tap's repo):

```bash
brew install amiwrpremium/shellboto/shellboto
```

## Upgrade

```bash
brew update
brew upgrade shellboto
```

## Formula

Goreleaser writes `Formula/shellboto.rb` to the tap. Generated
per-release from `.goreleaser.yaml`'s `brews:` block + the
scaffold in `packaging/homebrew/shellboto.rb`. The real `url` +
`sha256` fields are filled in by goreleaser based on the
archive it just built.

Example of the generated formula:

```ruby
class Shellboto < Formula
  desc "Telegram bot that gives whitelisted users a live bash shell on the VPS"
  homepage "https://github.com/amiwrpremium/shellboto"
  url "https://github.com/amiwrpremium/shellboto/releases/download/v0.2.0/shellboto_0.2.0_darwin_arm64.tar.gz"
  sha256 "abc123..."
  license "MIT"
  version "0.2.0"

  def install
    bin.install "shellboto"
  end

  test do
    assert_match "shellboto", shell_output("#{bin}/shellboto -version")
  end
end
```

Homebrew picks the matching archive for the user's OS + arch.

## PAT rotation

Fine-grained PATs have mandatory expiration. When it lapses,
`release.yml` logs "homebrew token permission denied" and
releases still publish — just without the Homebrew push.

Rotation procedure:

1. Generate a new PAT (same scope).
2. Replace the `HOMEBREW_TAP_GITHUB_TOKEN` secret in shellboto.
3. Revoke the old PAT.
4. Next release, the formula push resumes.

No urgency if the tap goes stale — you just can't `brew upgrade`
until it's fixed.

## Without the tap secret

Release still happens. The brew step in goreleaser logs:

```
• homebrew_casks: skipping because HOMEBREW_TAP_GITHUB_TOKEN is not set
```

And moves on. `.deb` / `.rpm` / archive assets all still publish.

## Linux on Homebrew?

Technically Homebrew works on Linux too (Linuxbrew). Nothing
prevents `brew install shellboto` from working on a Linux box,
but the `install.sh` installer is a better path for Linux:

- Sets up systemd unit.
- Prompts for token + superadmin.
- Configures `/etc/shellboto/env`.

Homebrew on Linux gets you just the binary in `~/.linuxbrew/bin/`.

## What's NOT in the formula

- **systemd unit** — Homebrew on macOS has no systemd. If you
  want launchd, write your own plist; shellboto doesn't ship
  one.
- **config template** — `brew install` is binary-only. Run
  `shellboto` with `-config /path/to/manually/created/config`.
- **bash completions** — could be generated via
  `bin.install_symlink`; not shipped today.

## Reading the code

- `.goreleaser.yaml` `brews:` block.
- `packaging/homebrew/shellboto.rb` — reference scaffold (not
  what gets shipped).

## Read next

- [goreleaser.md](goreleaser.md) — the config that drives this.
- [../development/releasing.md](../development/releasing.md) —
  where the push fits in the release flow.
