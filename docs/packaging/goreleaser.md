# goreleaser

`.goreleaser.yaml` drives every release. Breakdown of the
sections.

## `builds`

```yaml
builds:
  - id: shellboto
    main: ./cmd/shellboto
    binary: shellboto
    env:
      - CGO_ENABLED=0
    goos: [linux, darwin]
    goarch: [amd64, arm64]
    ldflags:
      - -s -w
      - -X main.version={{.Version}}
      - -X main.gitSHA={{.ShortCommit}}
      - -X main.built={{.Date}}
```

Cross-compiles to 4 targets. CGO disabled → pure static binaries.
ldflags strip debug symbols (`-s -w`) and embed version info.

`{{.Version}}` comes from the tag (`v0.2.0` → `0.2.0`).
`{{.ShortCommit}}` is the first 7 chars of the commit SHA.
`{{.Date}}` is the build timestamp.

## `archives`

```yaml
archives:
  - id: default
    formats: [tar.gz]
    name_template: >-
      shellboto_{{.Version}}_{{.Os}}_
      {{- if eq .Arch "amd64" }}x86_64
      {{- else }}{{ .Arch }}{{ end }}
    files:
      - README.md
      - LICENSE*
      - CHANGELOG.md
      - deploy/config.example.toml
      - deploy/config.example.yaml
      - deploy/config.example.json
      - deploy/env.example
      - deploy/shellboto.service
      - src: deploy/init
        dst: init
```

`name_template` follows the Goreleaser convention: `<name>_<ver>_<os>_<arch>`.
`amd64` renamed to `x86_64` to match most distro conventions.

Archive contents: binary + supporting files. Init scripts under
`deploy/init/` are copied verbatim into an `init/` subdir.

## `nfpms`

```yaml
nfpms:
  - id: shellboto
    package_name: shellboto
    vendor: amiwrpremium
    homepage: https://github.com/amiwrpremium/shellboto
    maintainer: amiwrpremium <amiwrpremium@gmail.com>
    description: Telegram bot that gives whitelisted users a live bash shell on the VPS.
    license: MIT
    formats: [deb, rpm]
    section: admin
    priority: optional
    contents:
      - src: deploy/config.example.toml
        dst: /etc/shellboto/config.toml.example
        type: config|noreplace
        file_info: { mode: 0600 }
      - src: deploy/env.example
        dst: /etc/shellboto/env.example
        type: config|noreplace
        file_info: { mode: 0600 }
      - src: deploy/shellboto.service
        dst: /lib/systemd/system/shellboto.service
        file_info: { mode: 0644 }
    scripts:
      postinstall: packaging/postinstall.sh
      preremove: packaging/preremove.sh
    dependencies:
      - systemd
    recommends:
      - openssl
```

See [deb-rpm.md](deb-rpm.md) for what the postinstall/preremove
hooks do.

## `brews`

```yaml
brews:
  - name: shellboto
    repository:
      owner: amiwrpremium
      name: homebrew-tap
      token: "{{ .Env.HOMEBREW_TAP_GITHUB_TOKEN }}"
    directory: Formula
    homepage: https://github.com/amiwrpremium/shellboto
    description: "Telegram bot that gives whitelisted users a live bash shell on the VPS"
    license: MIT
    commit_author:
      name: shellboto-release-bot
      email: release@shellboto.invalid
    test: |
      assert_match "shellboto", shell_output("#{bin}/shellboto -version")
```

Pushes a formula file to the tap repo each release. `test` block
runs under `brew test` to verify install.

If `HOMEBREW_TAP_GITHUB_TOKEN` isn't set, goreleaser warns +
skips this step; rest of the release completes.

See [homebrew.md](homebrew.md).

## `sboms`

```yaml
sboms:
  - artifacts: archive
    cmd: syft
    args: ["$artifact", "--output", "cyclonedx-json=$document"]
```

One SBOM per archive (4 archives → 4 SBOMs). CycloneDX JSON
format. `syft` is required in the runner's PATH — the
`ci.yml` setup-go step includes it via
`scripts/install-dev-tools.sh`.

## `changelog`

```yaml
changelog:
  use: git
  sort: asc
  filters:
    exclude:
      - '^chore:'
      - '^style:'
      - Merge pull request
```

Goreleaser builds a quick changelog from git log since the
previous tag. Used for the GitHub Release body when
release-please isn't the source of truth (rare).

Normally the CHANGELOG.md on master (maintained by release-please)
is more comprehensive; goreleaser's auto-changelog is a fallback
for emergency tag pushes.

## `release`

```yaml
release:
  github:
    owner: amiwrpremium
    name: shellboto
  draft: false
  prerelease: auto
  header: |
    Install via .deb/.rpm (see assets below) or `sudo ./deploy/install.sh`
    after unpacking the archive.
  footer: |
    **Full changelog**: https://github.com/amiwrpremium/shellboto/compare/{{ .PreviousTag }}...{{ .Tag }}
```

`prerelease: auto` — tags like `v0.2.0-rc1` are auto-marked as
pre-release; plain `v0.2.0` goes to latest.

## Running locally

```bash
make release-snapshot
```

= `goreleaser release --snapshot --clean`. Produces everything in
`dist/` without publishing. Takes ~1–2 min.

```bash
make release-check
```

= `goreleaser check`. Validates `.goreleaser.yaml` without
building.

## Reading the code

- `.goreleaser.yaml`
- `packaging/postinstall.sh`
- `packaging/preremove.sh`
- `packaging/homebrew/shellboto.rb` (reference / scaffold;
  released formula is built by goreleaser)

## Read next

- [deb-rpm.md](deb-rpm.md) — nfpm output details.
- [homebrew.md](homebrew.md) — tap setup.
