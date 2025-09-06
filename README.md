# Codespace Operator

Spin up **web IDE sessions** (JupyterLab, VS Code, ...) on Kubernetes using a single custom resource.

---

## What it does

- Reconciles a `Session` into Kubernetes primitives:
  - `Deployment` running your IDE container
  - `ServiceAccount`, `Service`, optional `Ingress`
  - optional PVCs for **home** and **scratch**
- Updates status fields (`status.url`, `status.phase`: `Pending` / `Ready` / `Error`).
- Ships a web **Admin UI** and a tiny HTTP **API server** for convenience.

### Example: a single Jupyter session

```yaml
apiVersion: codespace.codespace.dev/v1
kind: Session
metadata:
  name: alice
spec:
  profile:
    ide: jupyterlab
    image: jupyter/base-notebook:latest
    cmd:
      ["start-notebook.sh", "--NotebookApp.token=", "--NotebookApp.password="]
  auth:
    mode: oauth2proxy
    oidc:
      issuerURL: https://issuer.example.com/
  home:
    size: 20Gi
    storageClassName: fast-ssd
    mountPath: /home/jovyan
  scratch:
    size: 100Gi
    mountPath: /scratch
  networking:
    host: alice.lab.example.com
    tlsSecretName: alice-tls
```

---

## Install (production)

> **CRDs are released separately from the chart.** Install CRDs once, then install/upgrade the chart whenever you ship a new app version.

1. **Install CRDs** (pick from GitHub Releases)

```bash
kubectl apply -f https://github.com/codespace-operator/codespace-operator/releases/download/crd-v<CRD_VERSION>/codespace-operator-crds.yaml
```

2. **Install the Helm chart** (published as an OCI artifact)

```bash
helm registry login ghcr.io
helm install codespace-operator oci://ghcr.io/codespace-operator/charts/codespace-operator \
  --namespace codespace-operator-system \
  --create-namespace \
  --version <CHART_VERSION>
```

3. **Create your first Session**

```bash
kubectl apply -f - <<'YAML'
apiVersion: codespace.codespace.dev/v1
kind: Session
metadata:
  name: demo-session
  namespace: default
spec:
  profile:
    ide: jupyterlab
    image: jupyter/minimal-notebook:latest
    cmd: ["start-notebook.sh","--NotebookApp.token="]
  networking:
    host: demo.codespace.test
YAML
```

---

## Quick start (Developers)

### Prerequisites

- Go **1.25**
- Node **20**
- `kubectl` **1.24+**
- `kind` **v0.22+**

We use `*.codespace.test` for DNS during dev (resolves to `127.0.0.1`).

### One-command local cluster

Runs kind, installs ingress, builds/loads images, installs CRDs, deploys chart, applies a demo session.

```bash
./contrib/scripts/setup.sh
```

When it finishes:

```
UI  : http://console.codespace.test/
App : http://demo.codespace.test
```

### Manual workflow

Build UI + server:

```bash
make build-server
./bin/codespace-server
```

Build images:

```bash
make docker-build           # operator
make docker-build-server    # API server (embeds UI)
```

Install chart (manager + server + UI):

```bash
helm upgrade --install codespace-operator oci://ghcr.io/codespace-operator/charts/codespace
```

Cleanup:

```bash
./contrib/scripts/teardown.sh
```

---

## Configuration

### Helm values

See [`charts/codespace/values.yaml`](https://github.com/codespace-operator/charts/blob/main/charts/codespace/values.yaml) for all options (service accounts, RBAC, network policy, ingress, resources, etc.).

### Supported IDE profiles (defaults)

- `jupyterlab` → image `jupyter/minimal-notebook:latest`, cmd `start-notebook.sh --NotebookApp.token=`
- `vscode` → image `codercom/code-server:latest`, cmd `--bind-addr 0.0.0.0:8080 --auth none`

---

## Architecture

- **Session Controller** (`cmd/main.go`, `internal/controller/`) - reconciles `Session` CRs into `Deployment`/`Service`/`Ingress`/PVC.
- **Server** (`internal/server/`) - Core API used by the UI; serves the built UI from `/static`.
- **Web UI** (`ui/`) - PatternFly + React admin console.
- **CRDs** (`api/`, generated into `config/crd/bases/`).

---

## Releases & Versioning

This repo uses three **independent release lanes** with semantic‑release:

- **Operator (images)** - **tags**: `app-vX.Y.Z`
  Builds/pushes `ghcr.io/codespace-operator/codespace-operator:<app-version>` and `codespace-server:<app-version>`.
  Config: `release.operator.cjs`

- **CRDs** - **tags**: `crd-vX.Y.Z`
  Publishes `dist/codespace-operator-crds.yaml` and a tarball as release assets.
  Config: `release.crd.cjs`

**Commit scopes decide which lane releases:**

- Operator scopes: `operator`, `controller`, `server`, `ui`
- CRD scopes: `crd`, `api`

See **[CONTRIBUTING.md](./CONTRIBUTING.md)** for Conventional Commit rules and PR templates.

---

## Uninstall

```bash
helm -n codespace-operator-system uninstall codespace-operator || true
make uninstall || true   # deletes CRDs if you installed via `make install`
kind delete cluster --name codespace || true
```

---

## Contributing & Security

- Read **[CONTRIBUTING.md](./CONTRIBUTING.md)** for commit rules, local hooks, and release lanes.
- Security reports: please do **not** open public issues. Contact maintainers per the Security section in CONTRIBUTING.

---

## License

Apache-2.0
