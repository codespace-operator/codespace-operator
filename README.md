# codespace-operator

Spin up authenticated, per-user **web IDE sessions** (JupyterLab, VS Code, etc.) on Kubernetes with a single CR:

```yaml
apiVersion: codespace.codespace.dev/v1
kind: Session
metadata:
  name: alice
spec:
  profile:
    ide: jupyterlab
    image: jupyter/base-notebook:latest
    cmd: ["start-notebook.sh","--NotebookApp.token=","--NotebookApp.password="]
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

## What it does

- Reconciles a `Session` into:
  - a `Deployment` running your IDE (optional `oauth2-proxy` sidecar when `auth.mode=oauth2proxy`)
  - `ServiceAccount`, `Service`, `Ingress`
  - optional PVCs for **home** and **scratch**
- Sets `status.url` and `status.phase` (`Pending` / `Ready`).

---

## Quick start (developers)

### Prerequisites

- Go **1.25.0**
- Node **20**
- `kubectl` **1.24+**
- `kind` **v0.22+**
- `helm` **v3**

> We use `*.codespace.test` for DNS during dev. It automatically resolves to `127.0.0.1` (no `/etc/hosts` edits needed).

### 0) Repo layout

```
cmd/
  manager/     # operator manager (binary)
  gateway/     # tiny HTTP JSON API + serves the UI (binary)
ui/            # React (Vite) admin UI
helm/          # Helm chart for manager + (optional) gateway + CRDs
hack/tests/    # local one-liner setup script for kind
internal/      # controllers, helpers
api/           # CRD Go types
```

### 1) One-command local cluster (recommended)

This creates a kind cluster, installs ingress-nginx, builds/loads images, installs CRDs, deploys via Helm, and applies a demo `Session`.

```bash
./hack/tests/setup.sh
```

When it finishes:

- **Admin UI**: http://console.codespace.test/
- **Demo session** (if applied): `kubectl get sessions -n default` → open the `status.url`

### 2) Manual (if you prefer)

Build images:

```bash
# Manager (root Dockerfile)
docker build -t ghcr.io/codespace-operator/codespace-operator:dev .

# Gateway (builds UI inside the image and embeds it)
docker build -t ghcr.io/codespace-operator/codespace-server:dev -f ui/Dockerfile .
```

Create cluster & load images:

```bash
kind create cluster --name codespace --config hack/tests/kind.yaml
kind load docker-image ghcr.io/codespace-operator/codespace-operator:dev --name codespace
kind load docker-image ghcr.io/codespace-operator/codespace-server:dev --name codespace

# Ingress controller for local testing
kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/main/deploy/static/provider/kind/deploy.yaml
kubectl -n ingress-nginx rollout status deploy/ingress-nginx-controller --timeout=180s
```

Install CRDs (dev):

```bash
make install
```

Deploy with Helm (manager + gateway + UI):

```bash
helm upgrade --install codespace-operator ./helm \
  --namespace codespace-operator-system --create-namespace \
  --set image.repository=ghcr.io/codespace-operator/codespace-operator \
  --set image.tag=dev \
  --set server.enabled=true \
  --set server.image.repository=ghcr.io/codespace-operator/codespace-server \
  --set server.image.tag=dev \
  --set server.ingress.enabled=true \
  --set server.ingress.hosts[0].host=console.codespace.test \
  --set server.ingress.hosts[0].path=/
```

Wait for pods:

```bash
kubectl -n codespace-operator-system rollout status deploy/codespace-operator-controller-manager --timeout=180s
kubectl -n codespace-operator-system rollout status deploy/codespace-operator-server --timeout=180s
```

Open the **Admin UI**: http://console.codespace.test/

Create a demo `Session`:

```bash
cat <<'YAML' | kubectl apply -f -
apiVersion: codespace.codespace.dev/v1
kind: Session
metadata:
  name: demo-session
  namespace: my-session
spec:
  profile:
    ide: jupyterlab
    image: jupyter/minimal-notebook:latest
    cmd: ["start-notebook.sh","--NotebookApp.token="]
  networking:
    host: demo.codespace.test
YAML

kubectl -n default wait --for=jsonpath='{.status.phase}'=Ready session/demo --timeout=2m || true
kubectl -n default get session/demo -o=jsonpath='{.status.url}'; echo
```

---

## Dev loop tips

### Operator (manager)

Run against your current kube context (out-of-cluster):

```bash
go run ./cmd/manager
```

Logs while running in cluster:

```bash
kubectl -n codespace-operator-system logs -f deploy/codespace-operator-controller-manager
```

### Gateway + UI

**Fastest UI editing loop** (proxy to in-cluster gateway):

1) Port-forward the gateway:

```bash
kubectl -n codespace-operator-system port-forward svc/codespace-operator-server 8080:8080
```

2) In `ui/vite.config.ts`, add a dev proxy (only for `npm run dev`):

```ts
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

export default defineConfig({
  plugins: [react()],
  server: {
    proxy: { "/api": "http://localhost:8080" }
  }
});
```

3) Run the dev server:

```bash
cd ui
npm i
npm run dev
# browse http://localhost:5173 (UI), which proxies /api to gateway
```

**Rebuild the in-cluster gateway** after Go/UI changes:

```bash
docker build -t ghcr.io/codespace-operator/codespace-server:dev -f cmd/server/Dockerfile .
kind load docker-image ghcr.io/codespace-operator/codespace-server:dev --name codespace
helm upgrade codespace-operator ./helm -n codespace-operator-system --reuse-values
```

> Note: `cmd/server` embeds files from `cmd/server/static/`. We keep a tiny placeholder so `go:embed` works locally; the Docker build overwrites it with the real `ui/dist/` bundle.

---

## Create & manage sessions

```bash
kubectl get sessions -A
kubectl describe session demo -n default
kubectl get deploy,svc,ing -l app=cs-demo -n default
```

If `spec.networking.host` is set and your Ingress + DNS/TLS are configured, open `status.url`.

### Auth notes (OIDC via oauth2-proxy)

- Set `spec.auth.mode: oauth2proxy` and `spec.auth.oidc.issuerURL`.
- In real deployments, map client ID/secret from `Secret` refs (the sample keeps it simple).
- Use TLS on ingress for OIDC flows.

### Storage

Optional `home` / `scratch` PVCs:

- `size`: `\\d+(Gi|Mi)`
- `storageClassName`: optional
- `mountPath`: required

---

## Troubleshooting

- **`npm run build` fails on Vite/top-level await**  
  Use Node **18+** (e.g. `nvm use 20`).

- **`@vitejs/plugin-react` not found**  
  `npm i -D @vitejs/plugin-react` (already in `devDependencies`).

- **`go:embed ... contains no embeddable files`** (gateway)  
  Ensure `cmd/server/static/index.html` placeholder exists (committed). Docker build replaces it with `ui/dist/*`.

- **Manager Dockerfile fails with `cmd/main.go`**  
  We build `./cmd/manager` now. Use Go **1.22** base image.

- **Helm YAML parse in gateway Deployment**  
  Use block-style YAML for probes/templating (chart templates already adjusted).

- **`*.codespace.test` works on your host only**  
  Inside cluster, `demo.codespace.test → 127.0.0.1` (the pod itself). For in-cluster curls, hit Services (e.g., `http://codespace-operator-gateway:8080/`) or the Service’s ClusterIP.

---

## Uninstall / cleanup

```bash
helm -n codespace-operator-system uninstall codespace-operator || true
make uninstall || true   # CRDs (if installed via `make install`)
kind delete cluster --name codespace || true
```

---

## License

Apache-2.0
