# Releasing

Releases are driven by release-please. You don't tag by hand; you
merge a PR.

## The flow

1. **Land Conventional Commits on `master`.** Normal dev work.
2. **release-please notices** (fires on every push to master via
   `release-please.yml`):
   - Scans commits since the last release.
   - Computes the next version per the semver rules below.
   - Opens / updates a release PR titled
     `chore(master): release X.Y.Z`. Body is the auto-generated
     CHANGELOG diff.
3. **Review the release PR** like any other PR. Edit
   `CHANGELOG.md` in the PR if the auto-generated notes need
   tweaking.
4. **Merge the release PR.** release-please auto-creates the tag
   `vX.Y.Z` on the merge commit.
5. **`release.yml` fires on the tag.** goreleaser builds +
   publishes:
   - Cross-platform binaries (`linux/{amd64,arm64}`,
     `darwin/{amd64,arm64}`).
   - Archives (tar.gz).
   - `.deb` + `.rpm` via nfpm.
   - Homebrew formula (pushed to the tap if
     `HOMEBREW_TAP_GITHUB_TOKEN` is set).
   - `checksums.txt`.
   - CycloneDX SBOMs per archive.
   - GitHub Release with grouped notes.

Total time from PR merge to published artifacts: ~5–10 minutes.

## Version bump rules

Pre-1.0 (`bump-minor-pre-major: true`, `bump-patch-for-minor-pre-major:
true`):

- `feat:` → patch bump (0.1.0 → 0.1.1).
- `fix:` → patch bump.
- `feat!:` or `BREAKING CHANGE:` → minor bump (0.1.0 → 0.2.0).

Post-1.0 (after the first `1.0.0` ships):

- `fix:` → patch (1.0.0 → 1.0.1).
- `feat:` → minor (1.0.0 → 1.1.0).
- `feat!:` or `BREAKING CHANGE:` → major (1.0.0 → 2.0.0).

`chore:` / `style:` / `docs:` don't bump.

Config in `release-please-config.json`:

```json
{
  "release-type": "go",
  "bump-minor-pre-major": true,
  "bump-patch-for-minor-pre-major": true,
  ...
  "changelog-sections": [
    { "type": "feat",     "section": "Features" },
    { "type": "fix",      "section": "Bug fixes" },
    { "type": "perf",     "section": "Performance" },
    { "type": "refactor", "section": "Refactors" },
    { "type": "docs",     "section": "Documentation" },
    { "type": "test",     "section": "Tests" },
    { "type": "build",    "section": "Build" },
    { "type": "ci",       "section": "CI" },
    { "type": "chore",    "section": "Chores", "hidden": true },
    { "type": "style",    "section": "Style",  "hidden": true }
  ]
}
```

## Release-As override

Forcing a specific version on the next release:

Add a commit with this trailer:

```
chore: pin next release at 0.1.0

Release-As: 0.1.0
```

release-please picks up the trailer and proposes `0.1.0` on the
next release PR. Useful for:

- First release (release-please defaults to `1.0.0` otherwise).
- Skipping a version.
- Branding: "we want this to be `0.5.0` for compat reasons."

## Emergency manual release

If release-please is wedged and you need to ship now, and you're
a repo admin:

```bash
make release-check          # lint + test + vet + vuln + goreleaser check
git tag -a v0.2.0 -m "v0.2.0"
git push origin v0.2.0
```

`release.yml` reacts to the tag the same way either path created
it.

**Tag protection** (`.github/SETTINGS.md`) restricts `v*` pushes
to admins. Contributors can't accidentally `git push origin
v1.0.0`; only admins can.

Use this only in genuine emergencies. For normal work, the
release-please PR flow is correct.

## Dry-run locally

```bash
make release-snapshot
```

Runs goreleaser in `--snapshot --clean` mode. Produces everything
a real release would, under `dist/`. Doesn't publish.

Takes ~1–2 minutes.

## After the release lands

- Binaries on GitHub Releases page.
- `.deb` + `.rpm` in assets.
- Homebrew formula (if tap token set) committed to
  `amiwrpremium/homebrew-tap`.
- CHANGELOG.md on master has the new section.
- `.release-please-manifest.json` updated to the new version.

Announce: release notes are right there on the GitHub Release.

## Installing the new version

### From source:
```bash
git fetch --tags
git checkout v0.2.0
make build
sudo ./deploy/install.sh
```

### .deb:
```bash
wget https://github.com/amiwrpremium/shellboto/releases/download/v0.2.0/shellboto_0.2.0_linux_amd64.deb
sudo apt install ./shellboto_0.2.0_linux_amd64.deb
```

### Homebrew (CLI on macOS):
```bash
brew upgrade amiwrpremium/shellboto/shellboto
```

## Troubleshooting releases

### The release PR is stuck at 1.0.0 on a fresh repo

Add the `Release-As: 0.1.0` trailer commit (see above).

### The tag push fails with "workflow scope missing"

Your PAT is a fine-grained one without Workflows: Write. Regenerate
the token with that scope.

### goreleaser-action fails with "homebrew token permission denied"

`HOMEBREW_TAP_GITHUB_TOKEN` is invalid or expired. Regenerate a
fine-grained PAT scoped to the tap repo with Contents: Write,
paste into the shellboto repo's secret.

### The Homebrew formula pushed but `brew install` fails

Goreleaser uses a static template; the `url` + `sha256` in the
pushed formula are filled in at release time. If those don't
match the published archive (e.g. if you manually re-uploaded
artifacts), `brew install` checksum fails. Re-cut the release.

## Read next

- [../packaging/](../packaging/) — goreleaser internals.
- [ci.md](ci.md) — the workflow files.
