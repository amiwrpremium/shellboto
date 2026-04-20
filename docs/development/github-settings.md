# GitHub settings

One-time UI checklist for configuring the GitHub repo. The
authoritative version lives at [`.github/SETTINGS.md`](../../.github/SETTINGS.md)
in the repo root — that's what operators should follow.

This doc is a pointer + quick rationale.

## Why there's a separate doc

Branch protection, required status checks, tag protection,
dependabot alerts, secret-scan push-protection, and the Homebrew
tap secret are all **operator decisions**. They differ by
deployment, can't be version-controlled as code (GitHub's API
surface is inconsistent), and are set once at repo creation.

`.github/SETTINGS.md` walks through the clicks. `docs/` just
reminds you to do it.

## The high-level list

1. **Create the repo** (private at first; flip to public when
   ready). Don't let GitHub add a README / gitignore / license —
   they collide with the local ones.
2. **Branches → Add rule for `master`**:
   - Require a PR before merging.
   - Require status checks (see
     [ci.md](ci.md#status-check-names) for names).
   - Require linear history (matches our squash-merge policy).
   - No force-push, no delete.
3. **Tags → Protected tags pattern `v*`** — admins only. Prevents
   accidental `git push origin v1.0.0` from contributors;
   release-please's workflow token still works.
4. **Code security**:
   - Dependabot alerts ON.
   - Dependabot security updates ON.
   - Secret scanning ON.
   - Push protection ON.
5. **Actions → General**:
   - "Read and write permissions" (release-please needs it).
   - "Allow GitHub Actions to create and approve pull requests"
     (release-please + dependabot open PRs).
6. **Pull Requests**:
   - Squash-only (disable merge commits + rebase-merge).
   - "Pull request title and description" as default commit
     message.
   - Allow auto-merge (for dependabot-auto-merge.yml).
   - Auto-delete head branches.
7. **Optional: Homebrew tap secret**:
   - Create the `amiwrpremium/homebrew-tap` repo (empty,
     public).
   - Generate a fine-grained PAT: Contents: R/W on that one
     repo, no other perms.
   - Paste as `HOMEBREW_TAP_GITHUB_TOKEN` secret on shellboto.
   - Rotate before the PAT expires.

## Why each matters

- **Branch protection** keeps `master` shippable. Every commit
  there has passed CI.
- **Tag protection** keeps the release path gate-kept — only
  admins or the workflow can push a `v*` tag, and tagging is
  what triggers a release.
- **Dependabot** + **security updates** keep dependencies
  patched. Auto-merge handles patch-only; humans review
  minor/major.
- **Secret scanning** + **push protection** catch accidental
  secret commits at push time (belt-and-braces alongside our
  local gitleaks hook).
- **Workflow permissions + allow-PR** let release-please +
  dependabot open their PRs without us manually granting on
  every run.
- **Squash-only merges** keep history linear; Conventional
  Commits map 1:1 with merges.

## First-run expectations

- **CodeQL will flag pre-existing issues.** Normal. Triage under
  Security → Code scanning; fix what matters, dismiss the rest
  with rationale.
- **Dependabot's first week is noisy.** It scans everything
  outdated and opens a batch of PRs. Merge the patch ones;
  triage the minor/major ones. Subsequent weeks are quiet.

## Signed commits

Optional. Not shipped as a requirement.

If you want signed commits on `master`:

- Configure GPG / SSH signing locally (`git config commit.gpgsign
  true` + key).
- In branch protection for `master`, tick "Require signed commits."
- Make sure release-please's bot commits are signed too — that's
  done via the `sign-commits` input on the action.

## Read next

- [`.github/SETTINGS.md`](../../.github/SETTINGS.md) — the
  click-by-click checklist.
- [ci.md](ci.md) — the CI workflows those branch-protection
  toggles gate.
