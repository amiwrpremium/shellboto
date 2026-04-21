# Changelog

## [0.1.1](https://github.com/amiwrpremium/shellboto/compare/v0.1.0...v0.1.1) (2026-04-21)


### Bug fixes

* **ci:** commit-msg regex accepts multiple parenthesised scopes ([#10](https://github.com/amiwrpremium/shellboto/issues/10)) ([fa8a3ff](https://github.com/amiwrpremium/shellboto/commit/fa8a3ff3346a1205c1ac046195a840189e328256))
* **codeql:** check Close() on writable file handles ([#13](https://github.com/amiwrpremium/shellboto/issues/13)) ([43c0b80](https://github.com/amiwrpremium/shellboto/commit/43c0b804c81186a8736ffd78d6e068855868832f))


### CI

* **release-please:** authenticate via RELEASE_PLEASE_TOKEN PAT ([#14](https://github.com/amiwrpremium/shellboto/issues/14)) ([7434055](https://github.com/amiwrpremium/shellboto/commit/7434055efa403f41154d9f994bed832cd52b13de))

## 0.1.0 (2026-04-20)


### Features

* **cli:** bot entrypoint and ops subcommands ([5012b86](https://github.com/amiwrpremium/shellboto/commit/5012b86a9fc569341a8b120668b40457870bc6bb))
* **config:** multi-format config loader (TOML/YAML/JSON) ([ab4e959](https://github.com/amiwrpremium/shellboto/commit/ab4e959af5ae783ca56843ebfd5294abde7cae85))
* **danger:** dangerous-command regex matcher ([9517eb5](https://github.com/amiwrpremium/shellboto/commit/9517eb564db3c293b1789ba9af3f86cb8e790463))
* **db:** SQLite state store with hash-chained audit log ([dabbb55](https://github.com/amiwrpremium/shellboto/commit/dabbb557ec0d6860e28b645809dbbb4ff13a0fe8))
* **deploy:** installer, uninstaller, rollback, systemd + init scripts ([1255035](https://github.com/amiwrpremium/shellboto/commit/12550354c465d3fb99e406e33d3158b82d410db4))
* **files:** file upload / download helpers ([0a5ee1d](https://github.com/amiwrpremium/shellboto/commit/0a5ee1d223ad0fd07f4d0453a56c57881514f6ba))
* **logging:** zap-based structured logger wiring ([feb1e59](https://github.com/amiwrpremium/shellboto/commit/feb1e596a7ca391acc284a90773f5b46eea71f71))
* **redact:** secret scrubber for audit storage ([152e9cc](https://github.com/amiwrpremium/shellboto/commit/152e9cc569efd41b09b56fbb4784f566d30f25e5))
* **shell:** pty-backed bash subprocess management ([0d60e61](https://github.com/amiwrpremium/shellboto/commit/0d60e61b320ee5ded8cb2f89f0b49ee0148c2ce1))
* **stream:** Telegram message-edit stream buffer ([1e80408](https://github.com/amiwrpremium/shellboto/commit/1e80408e5872d587090ae9a94a7c5c1275c386aa))
* **telegram:** bot core with commands, callbacks, rbac, middleware ([a70aef2](https://github.com/amiwrpremium/shellboto/commit/a70aef2cf2fd562ecbd2e2e36d1135b0762f8d4d))


### Bug fixes

* **release:** point Homebrew formula at existing amiwrpremium/homebrew-tap ([f0b9a70](https://github.com/amiwrpremium/shellboto/commit/f0b9a701bf0392c3dfd2d37025afbc17d82abee2))


### Documentation

* add architecture section ([c9f754b](https://github.com/amiwrpremium/shellboto/commit/c9f754b7acbf25de20486d030a1582e94e5ca39a))
* add audit section ([08f457a](https://github.com/amiwrpremium/shellboto/commit/08f457a017c3f4d7feba03b755a20c748c52a143))
* add configuration section ([216adfa](https://github.com/amiwrpremium/shellboto/commit/216adfa4cdf2455dad5eb0799d98a0f26e5f7dcc))
* add database section ([4335de9](https://github.com/amiwrpremium/shellboto/commit/4335de9cde362c5525b26f73805a162fdffda319))
* add deployment section ([d448a99](https://github.com/amiwrpremium/shellboto/commit/d448a99ba7377bb3f2978295a0be4b662aa952a2))
* add development section ([eea1f30](https://github.com/amiwrpremium/shellboto/commit/eea1f3048f91f388be7872509bb9106489dac546))
* add operations section ([043642f](https://github.com/amiwrpremium/shellboto/commit/043642f46d0a924bdb2619b38aa2772cdef7c028))
* add packaging section ([c446712](https://github.com/amiwrpremium/shellboto/commit/c4467125410f45a327b21f2afddc83b545693a6c))
* add README, contributor guide, changelog seed, issue templates ([e67e877](https://github.com/amiwrpremium/shellboto/commit/e67e877ac2d55827f47aa47f2ce5dc8086b6e2d0))
* add reference section ([81b764c](https://github.com/amiwrpremium/shellboto/commit/81b764cedf58de4e161324b3f559809bb7beaaef))
* add security section including full danger-matcher regex table ([5c5ebc3](https://github.com/amiwrpremium/shellboto/commit/5c5ebc3bd463385d2df9f7937417162866c697b5))
* add SETTINGS checklist and RUNBOOKS incident procedures ([a8fa680](https://github.com/amiwrpremium/shellboto/commit/a8fa680cf85315893d6a4cb162530bc44123dc96))
* add shell section ([5f1b642](https://github.com/amiwrpremium/shellboto/commit/5f1b642642f357bb14a3e17ad706ab538e7df1f6))
* add telegram section ([e31485d](https://github.com/amiwrpremium/shellboto/commit/e31485d6a7a637c0540af356ed6a88f3655fcf03))
* add top-level index + getting-started section ([5fd0c59](https://github.com/amiwrpremium/shellboto/commit/5fd0c5931ed546001f3ae31fa876f947cfae971d))
* add troubleshooting section ([228ae34](https://github.com/amiwrpremium/shellboto/commit/228ae34716072ccc50802e22669d7948a481697f))
* relocate RUNBOOKS.md into docs/runbooks/ tree, expand ([278a4f8](https://github.com/amiwrpremium/shellboto/commit/278a4f81dca863319b023a26d011483248f1ef01))


### Build

* **deps:** bump Go toolchain to 1.26 ([59c0bdc](https://github.com/amiwrpremium/shellboto/commit/59c0bdc0dd7b379ab35ba32cc0175ac34d19aacd))
* **make:** exclude .git from tarball target ([723fea9](https://github.com/amiwrpremium/shellboto/commit/723fea921b8a63214f4647118d34cb5068a3a79f))
* **release:** goreleaser config, nfpm packaging, homebrew scaffold ([881d631](https://github.com/amiwrpremium/shellboto/commit/881d6311c388927b7988a9cdac14086bcec13105))


### CI

* adopt release-please for CHANGELOG automation, retire git-chglog ([798aed3](https://github.com/amiwrpremium/shellboto/commit/798aed3efd9be40306ea6d492d5b176fbb5fbb05))
* CodeQL Go security scan ([fe1dfa7](https://github.com/amiwrpremium/shellboto/commit/fe1dfa7725f1df8eb737b749c852abb4983d56ca))
* Dependabot config with auto-merge for patch bumps ([80b11a5](https://github.com/amiwrpremium/shellboto/commit/80b11a564bfa92635e8140738baaa54d2f9bacd4))
* fix CI — silence SC1091 on `source` + pin golangci-lint v2 ([b99a0d3](https://github.com/amiwrpremium/shellboto/commit/b99a0d3c4a5ae1d4a36f55ae1bf52b4a61ee8b71))
* git-chglog template for CHANGELOG generation ([538ff79](https://github.com/amiwrpremium/shellboto/commit/538ff795ce24efa4b852aa5e42a1e7fdb63a0750))
* GitHub Actions workflows for CI and release ([7dbd986](https://github.com/amiwrpremium/shellboto/commit/7dbd986ea13bd79477b7ccb46e4923cf6d4ded77))
* install golangci-lint via goinstall so it builds with runner Go ([a843ab6](https://github.com/amiwrpremium/shellboto/commit/a843ab65ebdf4a6f377ca4970f0a7f83d9825672))
* pre-commit / pre-push / commit-msg quality gate ([1d20b4c](https://github.com/amiwrpremium/shellboto/commit/1d20b4c6725995029502a5675877497a464e65b5))


### Chores

* pin first release at 0.1.0 ([3ecb47d](https://github.com/amiwrpremium/shellboto/commit/3ecb47da29fdc78ff1a85b2229acaffc0153e3cc))

## CHANGELOG

This file is maintained by [release-please](https://github.com/googleapis/release-please).
**Do not hand-edit on `master`.** release-please opens a "release PR"
every time conventional commits worth releasing land on `master`; the
PR's diff is where corrections go — edit the CHANGELOG entry in the
release PR before merging it.

The file will be populated on the first release PR merge.
