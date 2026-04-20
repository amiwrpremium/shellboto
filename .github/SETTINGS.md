# GitHub repo settings — one-time setup

All of these are UI-only. Apply once; none are enforced by files in
this repo. Sequence: create repo → do **Before first push** → push
code → do **After first push**.

## Before first push

### Repository creation

Creating `amiwrpremium/shellboto` on GitHub:

- [ ] **Visibility**: Private (flip to Public when you're ready).
- [ ] **Do NOT** tick any of "Add README", ".gitignore", or "Add
      license". You have those locally; the pre-created files collide
      with your push.

### Default branch — General → Default branch

- [ ] Change default branch to **`master`** (GitHub creates `main` by
      default). Easiest to do at first push time: `git push -u origin
      master` and confirm the prompt.

### Pull request behaviour — General → Pull Requests

- [ ] **Allow squash merging** (only). Uncheck merge commits and
      rebase merges to enforce linear history aligned with
      Conventional Commits.
- [ ] Default commit message: "Pull request title and description".
- [ ] **Allow auto-merge** (required for
      `dependabot-auto-merge.yml` to work).
- [ ] **Automatically delete head branches** after merge.

### Workflow permissions — Actions → General

- [ ] Workflow permissions: **"Read and write permissions"**.
      release-please needs to push CHANGELOG commits + create tags.
- [ ] **Allow GitHub Actions to create and approve pull requests.**
      release-please opens PRs; dependabot opens PRs.

### Code security — Code security and analysis

- [ ] Dependabot alerts: **on**.
- [ ] Dependabot security updates: **on**.
- [ ] Secret scanning: **on**.
- [ ] Secret scanning **push protection**: **on**. Defence in depth
      with the local gitleaks pre-commit hook.

## Push code

```bash
cd /root/shellboto
git init
git branch -m master           # if current branch is 'main'
git add -A
git commit -m "chore: initial import"
git remote add origin https://github.com/amiwrpremium/shellboto.git
git push -u origin master
```

Wait for CI + CodeQL to run at least once so the status check names
appear. Then continue below.

## After first push

### Branch protection — Branches → Add rule for `master`

- [ ] Require a pull request before merging
  - [ ] Require approvals: **1** (skip while solo).
  - [ ] Dismiss stale approvals on new commits.
  - [ ] Require conversation resolution before merge.
- [ ] Require status checks to pass (select from dropdown once they
      have run at least once):
  - [ ] `lint` (ci.yml)
  - [ ] `test (go 1.26)` + `test (go stable)` (ci.yml)
  - [ ] `vet` (ci.yml)
  - [ ] `vuln` (ci.yml)
  - [ ] `shellcheck` (ci.yml)
  - [ ] `gitleaks` (ci.yml)
  - [ ] `commit-lint` (ci.yml — appears only once a PR has run)
  - [ ] `goreleaser-check` (ci.yml)
  - [ ] `analyze (go)` (codeql.yml)
  - [ ] Require branches to be up to date before merging.
- [ ] Require linear history.
- [ ] Restrict who can force-push: **no one**.
- [ ] Restrict who can delete: **no one**.
- [ ] *(Optional)* Require signed commits — turn on if your local
      GPG/SSH signing key is already configured in GitHub, otherwise
      skip. The other protections are plenty without this.

### Tag protection — Tags → Protected tags → add `v*`

- [ ] Pattern: **`v*`**.
- [ ] Restrict who can push matching tags: **admins only**.
  - release-please runs with the workflow `GITHUB_TOKEN`
    (admin-level) and can push.
  - Contributors can't `git push origin vX.Y.Z` by mistake.
  - Admin override remains for true emergencies.

## Secrets (when needed) — Secrets and variables → Actions

- [ ] `HOMEBREW_TAP_GITHUB_TOKEN` *(optional, add when ready to
      publish Homebrew)*:
  - Use your existing `amiwrpremium/homebrew-tap` repo (a single
    tap can host every formula across all your projects). If you
    don't have one yet, create an empty public repo with that
    name; goreleaser populates it on first release.
  - Generate a fine-grained PAT scoped to **only that one repo**,
    **Contents: Read and write**.
  - Paste as the secret value.
  - Without the secret, goreleaser silently skips the tap push; the
    rest of the release completes.

## Notes

- **CodeQL first run may surface pre-existing issues.** Normal — the
  first scan against an existing codebase is expected to flag things
  that have been there for a while. Review under Security → Code
  scanning and either fix or dismiss.
- **Dependabot's first week is noisy.** It opens an initial batch of
  PRs against every outdated dep. After that, weekly cadence evens
  out.
- **Signed-commit requirement**: only turn on after confirming your
  local signing flow works against this repo — otherwise you'll
  block yourself.
- **Rotate the `HOMEBREW_TAP_GITHUB_TOKEN`** periodically. Fine-
  grained PATs have a required expiration; when it fires, releases
  silently skip the brew push until you replace it.
