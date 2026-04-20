# Verifying downloads

Every release includes `checksums.txt` for integrity. Check it.

## checksums.txt

Location: GitHub Release page, among other assets:

```
checksums.txt
shellboto_0.2.0_linux_amd64.deb
shellboto_0.2.0_linux_x86_64.tar.gz
shellboto_0.2.0_linux_x86_64.tar.gz.sbom.json
... etc
```

Contents: `<sha256>  <filename>` per line.

```
a1b2c3d4e5f6...  shellboto_0.2.0_linux_x86_64.tar.gz
e7f8a9b0c1d2...  shellboto_0.2.0_linux_amd64.deb
...
```

## Verifying

Download your artifact + the checksums file:

```bash
curl -LO https://github.com/amiwrpremium/shellboto/releases/download/v0.2.0/shellboto_0.2.0_linux_x86_64.tar.gz
curl -LO https://github.com/amiwrpremium/shellboto/releases/download/v0.2.0/checksums.txt
```

Verify:

```bash
sha256sum --check --ignore-missing checksums.txt
# shellboto_0.2.0_linux_x86_64.tar.gz: OK
```

`--ignore-missing` ignores lines for files you didn't download.

## Running it via the installer

`deploy/install.sh --skip-build` expects you to already have
`bin/shellboto` in place. If you downloaded from a release:

```bash
tar xzf shellboto_0.2.0_linux_x86_64.tar.gz
sha256sum --check --ignore-missing checksums.txt   # verify first
cp shellboto bin/shellboto
chmod +x bin/shellboto
sudo ./deploy/install.sh --skip-build
```

## Future: cosign signatures

Not shipped today. The `.goreleaser.yaml` has a commented-out
`signs:` TODO. When we turn it on:

- Artifacts get a `.sig` alongside them.
- `cosign verify-blob --key cosign.pub --signature file.sig file`
  attests the file hasn't been tampered + was signed by our
  release key.
- Public key would live alongside checksums.txt on the release
  page + maybe in the repo itself.

See the TODO in `.goreleaser.yaml`. Revisit when release cadence
and operator demand for signed artifacts justify.

## What the checksums do NOT attest

- **Provenance.** The checksum proves file contents match; it
  doesn't prove who built them or whether the build process
  was legit. A compromised CI could generate a backdoored
  binary + matching checksum.
- **Freshness.** A stale checksum for an old release passes
  trivially; you need to confirm you're reading the right
  `checksums.txt` for the version you want.
- **License / policy compliance.** SBOM does that
  ([sbom.md](sbom.md)), not checksums.

For end-to-end attestation, the future signing work is the path.

## Read next

- [sbom.md](sbom.md) — the other integrity artifact.
- [../development/releasing.md](../development/releasing.md) —
  how these files get published.
