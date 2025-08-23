# Contributing Guide

Thanks for helping improve **codespace-operator**! This project is a mono‑repo with three independent release lanes and a few conventions that keep releases clean and predictable.

---

## TL;DR

1. Fork → feature branch → small, focused MR/PR.
2. Use **Conventional Commits** with a **required scope** (see below).
3. Run `make build` (Go), `make build-ui` (UI), and `make manifests` (CRDs) locally as needed.
4. Ensure hooks/linters pass (`prettier`, `gofmt`, commitlint).
5. Open an MR/PR with the template and checklist. Prefer **squash merge**.

> Releases only happen on `main` via CI. MRs never publish artifacts.

---

## Repo Areas & Release Lanes

- **Operator Images ("App" lane)** — operator/controller/server/UI
  - Tag prefix: `app-vX.Y.Z`
  - Config: `release.operator.cjs`
  - Scope(s): `operator`, `controller`, `server`, `ui`

- **Helm Chart ("Chart" lane)**
  - Tag prefix: `chart-vX.Y.Z`
  - Config: `release.helm.cjs`
  - Scope(s): `chart`, `helm`
  - `Chart.yaml:appVersion` is auto‑synced by the App lane. Don’t edit manually.

- **CRDs ("CRD" lane)**
  - Tag prefix: `crd-vX.Y.Z`
  - Config: `release.crd.cjs`
  - Scope(s): `crd`, `api`

Each lane uses `releaseRules` so it **only** versions on matching scopes. A single MR can touch multiple areas; the lanes release independently when merged.

---

## Conventional Commits (required)

**Format**: `type(scope): message`

**Types**: `feat`, `fix`, `perf`, `refactor`, `docs`, `chore`, `ci`, `build`, `test`
**Scopes (required)**:

- App: `operator`, `controller`, `server`, `ui`
- Chart: `chart`, `helm`
- CRDs: `crd`, `api`
- Misc (no release): `docs`, `build`, `ci`, `deps`, `release`, `test`

**Versioning**

- `feat(scope): …` → **minor**
- `fix(scope): …`, `perf(scope): …` → **patch**
- `feat!(scope): …` or body contains `BREAKING CHANGE:` → **major**
- `docs/chore/ci/build/test` → no version bump

**Examples**

- `feat(server): add health endpoint`
- `fix(chart): correct service port name`
- `feat(api)!: rename field spec.workspaceId`
  Body:

  ```
  BREAKING CHANGE: spec.workspaceID → spec.workspaceId
  ```

> If using **squash merge**, the **PR title** must be a valid Conventional Commit (that becomes the merge commit).

---

## Development Setup

- **Go**: 1.x as in `go.mod` (CI uses that). Run:
  - `make build` → builds `bin/session-controller`
  - `make test` → unit tests
  - `make lint` → `golangci-lint`

- **UI**: under `ui/`
  - `npm ci && npm run build`
  - Or repo root: `make build-ui` (builds UI and server binary)

- **CRDs**:
  - `make manifests` → regenerate CRDs
  - `./bin/kustomize build config/crd > dist/codespace-operator-crds.yaml`

- **Docker**:
  - Operator: `make docker-buildx IMG=ghcr.io/codespace-operator/codespace-operator:dev`
  - Server: `make docker-build-server SERVER_IMG=ghcr.io/codespace-operator/codespace-server:dev`

---

## Linting & Hooks

- **Commit messages** are validated by commitlint (scopes required).
- **pre-commit** runs `lint-staged` with `gofmt` and `prettier` on staged files.
- **Bypass (emergency)**: `HUSKY=0 git commit -m "savepoint"`

See `commitlint.config.cjs`, `.lintstagedrc.json`, and `.husky/*` for details.

---

## Making Changes by Area

### Operator / Server / UI

- Use scopes: `operator`, `controller`, `server`, `ui`.
- Building:

  ```bash
  make build            # Go
  make build-ui         # UI + server
  ```

- Tests: `make test` (unit), `make test-e2e` (kind-based e2e)

### Helm Chart

- Use scopes: `chart`, `helm`.
- **Do not** change `Chart.yaml:appVersion` manually; it is set by App releases.
- If adding values, provide sane defaults, schema comments, and update README usage snippets.

### CRDs

- Use scopes: `crd`, `api`.
- After changing API types, run `make manifests` and verify generated validation.
- Prefer strict validation (enums, patterns, required fields) where possible.

---

## CI & Releases

- CI runs on every MR/PR: build + lint + tests + commit‑message checks.
- After merge to `main`, semantic‑release runs per lane:
  - **Operator**: `release.operator.cjs` builds & pushes images; syncs `Chart.yaml:appVersion`; tag `app-v*`; writes `CHANGELOG.app.md`.
  - **Chart**: `release.helm.cjs` bumps chart `version` and publishes to GHCR (OCI); tag `chart-v*`; writes `CHANGELOG.chart.md`.
  - **CRDs**: `release.crd.cjs` regenerates and publishes `dist/codespace-operator-crds.yaml` + tarball; tag `crd-v*`; writes `CHANGELOG.crd.md`.

> Lanes only bump versions when at least one commit in the release range has a matching scope per `releaseRules`.

---

## MR/PR Guidelines

- Keep changes **small and focused**
- Add tests when fixing bugs or adding behavior.
- Update docs/values/examples when changing user‑visible behavior.
- Draft MRs early for feedback; convert to Ready when green.
- Prefer **squash merge** with a Conventional PR title.

### Labels & Areas

Use one or more:

- `area:operator` `area:server` `area:ui`
- `area:chart`
- `area:crd`

### Reviewer Checklist

- [ ] Commit messages follow Conventional Commits with scopes.
- [ ] CRD changes accompanied by `make manifests` output.
- [ ] Chart changes documented in values and README snippets.
- [ ] Tests and linters pass.
- [ ] No manual bump of `Chart.yaml:appVersion`.

---

## PR/MR Template (copy one)

**GitHub** → save as `.github/pull_request_template.md`
**GitLab** → save as `.gitlab/merge_request_templates/Default.md` and set as default template.

```md
### Summary

<!-- What does this change? Why? -->

### Type

- [ ] feat
- [ ] fix
- [ ] perf
- [ ] refactor
- [ ] docs/chore/ci/build/test

### Scope (required)

- [ ] operator
- [ ] controller
- [ ] server
- [ ] ui
- [ ] chart / helm
- [ ] crd / api

### Breaking changes

- [ ] Yes (describe below)
- [ ] No

If breaking, include `BREAKING CHANGE:` in the commit body or `type!` in the title.

### Testing

- [ ] unit
- [ ] e2e
- [ ] manual (describe)

### Checklist

- [ ] Conventional Commit(s) with required scope
- [ ] Docs/values updated if user‑visible
- [ ] CRDs regenerated if API changed (`make manifests`)
- [ ] No manual edits to `Chart.yaml:appVersion`
```

---

## Security

Please do **not** open a public issue for security reports. Email the maintainers or use the project’s security policy if available.

---

## DCO / CLA

This project uses the Developer Certificate of Origin. Include a Signed‑off‑by line in your commits:

```
Signed-off-by: Your Name <you@example.com>
```

Add automatically:

```
git commit -s -m "feat(server): add health endpoint"
```

Thanks you!
