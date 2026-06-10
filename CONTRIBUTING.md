# Contributing to Fenix

## Release Strategy

Fenix uses [Semantic Versioning](https://semver.org/) (`MAJOR.MINOR.PATCH`) with a branch-per-release model and [GoReleaser](https://goreleaser.com/) for automated builds.

### Version numbering

| Increment | When |
|-----------|------|
| `MAJOR`   | Breaking changes |
| `MINOR`   | New features, backwards compatible |
| `PATCH`   | Bug fixes |

Pre-release suffixes: `v0.2.0-rc.1`, `v0.2.0-rc.2`, `v0.2.0`

### Release flow

```
main
 │
 │   [features merged to main during development]
 │
 ├── release/0.2.0        ← cut when feature-complete
 │     ├─ tag v0.2.0-rc.1   → pre-release on GitHub, binaries built automatically
 │     ├─ (bugfixes only)
 │     ├─ tag v0.2.0-rc.2
 │     └─ tag v0.2.0         → full release on GitHub
 │
 └── merge release/0.2.0 back to main
```

New feature work continues on `main` (or feature branches) while the release branch is being stabilised. Never merge new features into a release branch.

### Step-by-step

**1. Create the release branch** (when the feature set for this version is complete):

```bash
git checkout main
git pull
git checkout -b release/0.2.0
git push origin release/0.2.0
```

**2. Tag a release candidate** — GitHub Actions builds it automatically and marks it as pre-release:

```bash
git tag v0.2.0-rc.1
git push origin v0.2.0-rc.1
```

**3. Fix & iterate** — commit bugfixes to the release branch, tag new RCs as needed:

```bash
git commit -m "correct pagination in Observation endpoint"
git tag v0.2.0-rc.2
git push origin v0.2.0-rc.2
```

**4. Ship** — when a candidate is validated:

```bash
git tag v0.2.0
git push origin v0.2.0
```

**5. Merge back to main:**

```bash
git checkout main
git merge release/0.2.0
git push origin main
```

### What happens on tag push

GoReleaser runs automatically via `.github/workflows/release.yaml`:

- Runs `go test ./...`
- Builds `fenix` for Linux, macOS, and Windows (amd64 + arm64)
- Creates a GitHub Release with changelog (merged PRs only, via GitHub's release notes API)
- Tags with `v*.*.*-rc.*` → marked as **pre-release**
- Tags with `v*.*.*` → marked as **full release**

The changelog is generated from **PR titles** — keep PR titles clear and descriptive, as they are what end up in the release notes.
