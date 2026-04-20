# Packaging

How shellboto gets distributed: goreleaser builds, .deb/.rpm
packages, Homebrew formula, SBOMs, verification.

| File | What it covers |
|------|----------------|
| [goreleaser.md](goreleaser.md) | `.goreleaser.yaml` walkthrough |
| [deb-rpm.md](deb-rpm.md) | nfpm-produced `.deb`/`.rpm`, postinstall / preremove hooks |
| [homebrew.md](homebrew.md) | Tap repo, formula, `HOMEBREW_TAP_GITHUB_TOKEN` |
| [sbom.md](sbom.md) | CycloneDX SBOM per archive via syft |
| [verifying-downloads.md](verifying-downloads.md) | checksums.txt usage + future cosign |

## What a release produces

On every tag push, goreleaser builds and publishes:

- **Binaries**: `linux_amd64`, `linux_arm64`, `darwin_amd64`,
  `darwin_arm64` (stripped, static, CGO disabled).
- **Tar archives**: `shellboto_<ver>_<os>_<arch>.tar.gz` each
  containing binary + README + LICENSE + CHANGELOG + config +
  env + unit + init scripts.
- **Linux packages**: `.deb` + `.rpm` for amd64 + arm64.
- **Homebrew formula**: pushed to
  `amiwrpremium/homebrew-shellboto` tap repo (conditional).
- **Checksums file**: `checksums.txt` (SHA-256 per artifact).
- **SBOMs**: CycloneDX per archive.
- **GitHub Release**: page with Conventional-Commit-grouped notes.

## Read next

- [goreleaser.md](goreleaser.md) — the config that does all this.
