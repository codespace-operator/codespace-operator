# codespace-operator

Spin up authenticated, per-user **web IDE sessions** (JupyterLab, VS Code, etc.) on Kubernetes with a single CR:

```yaml
apiVersion: codespace.codespace.dev/v1alpha1
kind: Session
metadata:
  name: alice
spec:
  profile:
    ide: jupyterlab
    image: jupyter/base-notebook:latest
    cmd: ["start-notebook.sh","--NotebookApp.token=''","--NotebookApp.password=''"]
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

- Reconciles `Session` CRs into:
  - `Deployment` with your chosen IDE container (and optional `oauth2-proxy` sidecar)
  - `ServiceAccount`, `Service`, `Ingress`
  - Optional PVCs for **home** and **scratch**
- Writes a handy `status.url` and `status.phase` (Pending/Ready).

## Create a session

```bash
kubectl apply -f examples/session-jupyter.yaml

# Watch it come up
kubectl get sessions
kubectl describe session alice
kubectl get deploy,svc,ing -l app=cs-alice
```

If `spec.networking.host` is set and your Ingress controller + DNS/TLS are configured, connect at `status.url`.

## Auth notes (OIDC via oauth2-proxy)

- Set `spec.auth.mode: oauth2proxy` and `spec.auth.oidc.issuerURL`.
- In a real deployment, point `OIDC_CLIENT_ID`/`OIDC_CLIENT_SECRET` to `Secret` keys (the example uses simple envs to keep the sample small).
- Ensure your Ingress is terminating TLS if you use OIDC.

## Storage

- Optional `home` and `scratch` PVCs:
  - `size` → e.g., `20Gi`
  - `storageClassName` (optional)
  - `mountPath` inside the IDE container

## Local dev

tbd

## Testing

tbd

## Uninstall / cleanup

License: Apache-2.0
