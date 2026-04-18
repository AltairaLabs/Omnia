# Omnia chart — values examples

Worked values files for common deployment shapes. Each is intended as a starting point — copy, edit the `# EDIT:` markers, deploy.

| File | Scenario |
|---|---|
| `values-minimal.yaml` | Smallest possible install. In-cluster dev-postgres, NFS, anonymous dashboard. Works on kind / k3d / Docker Desktop. |
| `values-prod-oauth.yaml` | OSS production: OAuth, external Postgres, external RWX storage. Multi-replica operator + dashboard. |
| `values-prod-enterprise.yaml` | Enterprise production: license + Arena + Redis queue. Layer on top of `values-prod-oauth.yaml`. |
| `values-azure-kv-csi.yaml` | Azure Workload Identity + Key Vault via CSI secret-store driver. Overlay — combine with `values-prod-oauth.yaml`. |
| `values-aws-irsa.yaml` | AWS IRSA + S3 cold archive + Secrets Manager CSI. Overlay. |
| `values-gke-workload-identity.yaml` | GKE Workload Identity + Cloud SQL + GCS. Overlay. |
| `values-observability-all-in.yaml` | Enables Prometheus, Grafana, Loki, Tempo, Alloy subcharts. Overlay. |
| `values-istio-ambient.yaml` | Istio ambient mode + Gateway API. Overlay. |

## How to combine

Helm applies `-f` arguments left-to-right; later files override earlier ones.

```bash
# Dev (local)
helm install omnia oci://ghcr.io/altairalabs/charts/omnia \
  -f charts/omnia/examples/values-minimal.yaml

# OSS prod on AWS with IRSA
helm install omnia oci://ghcr.io/altairalabs/charts/omnia \
  -f charts/omnia/examples/values-prod-oauth.yaml \
  -f charts/omnia/examples/values-aws-irsa.yaml

# Enterprise on Azure with KV + observability
helm install omnia oci://ghcr.io/altairalabs/charts/omnia \
  -f charts/omnia/examples/values-prod-oauth.yaml \
  -f charts/omnia/examples/values-prod-enterprise.yaml \
  -f charts/omnia/examples/values-azure-kv-csi.yaml \
  -f charts/omnia/examples/values-observability-all-in.yaml
```

## Secrets

These examples **do not embed secrets** — every place a credential would go is marked `EDIT:` or references a `*-existingSecret` field. Use one of:

- `kubectl create secret` (quick start)
- Sealed Secrets / SOPS / External Secrets Operator (GitOps)
- CSI secret-store driver from your cloud (see `values-azure-kv-csi.yaml`, `values-aws-irsa.yaml`)

Never commit real credentials.

## Testing

Before shipping:

```bash
helm template omnia charts/omnia -f charts/omnia/examples/<file>.yaml | kubeconform -strict -ignore-missing-schemas
```

The [#895 W6](https://github.com/AltairaLabs/Omnia/issues/895) CI gates will run this on every PR once landed.
