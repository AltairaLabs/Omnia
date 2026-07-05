---
title: "Install with a License"
description: "Configure Omnia with an Enterprise license key"
sidebar:
  order: 1
---

This guide covers installing Omnia with an Enterprise license to unlock advanced features like Git sources, load testing, and distributed workers.

For feature comparison between Open Core and Enterprise, see [Licensing & Features](/explanation/platform/licensing/).

---

## Prerequisites

- A Kubernetes cluster (v1.25+)
- Helm 3.x installed
- An Omnia license key (contact [sales@altairalabs.ai](mailto:sales@altairalabs.ai) or request a [free trial](https://omnia.altairalabs.ai/trial))

## Install with License Key

The simplest way to install with a license is to pass it directly to Helm:

```bash
helm install omnia oci://ghcr.io/altairalabs/charts/omnia \
  --namespace omnia-system \
  --create-namespace \
  --set license.key="eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9..."
```

This creates the `arena-license` Secret automatically.

## Install with Existing Secret

If you prefer to manage the license Secret separately (recommended for GitOps):

1. Create the Secret:

```bash
kubectl create secret generic arena-license \
  --namespace omnia-system \
  --from-literal=license="eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9..."
```

2. Reference it in your Helm values:

```yaml
# values.yaml
license:
  existingSecret: "arena-license"
```

3. Install Omnia:

```bash
helm install omnia oci://ghcr.io/altairalabs/charts/omnia \
  --namespace omnia-system \
  --create-namespace \
  -f values.yaml
```

## Verify License Status

Check that your license is active:

```bash
# View license status in the dashboard
kubectl port-forward svc/omnia-dashboard 3000:3000 -n omnia-system
# Open http://localhost:3000/settings and check the License section

# Or check operator logs
kubectl logs -l app.kubernetes.io/name=omnia -n omnia-system | grep -i license
```

## Update an Existing License

To update your license key:

### If using `license.key`:

```bash
helm upgrade omnia oci://ghcr.io/altairalabs/charts/omnia \
  --namespace omnia-system \
  --set license.key="eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.NEW_KEY..."
```

### If using `license.existingSecret`:

```bash
kubectl create secret generic arena-license \
  --namespace omnia-system \
  --from-literal=license="eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.NEW_KEY..." \
  --dry-run=client -o yaml | kubectl apply -f -
```

The operator will detect the new license within 5 minutes (or restart it for immediate effect).

## Upload via Dashboard

You can also upload a license through the dashboard:

1. Open the Omnia dashboard
2. Navigate to **Settings** → **License**
3. Click **Upload License**
4. Paste your license key and click **Save**

## How the License Is Validated

Omnia validates the license **offline**. The key is a cryptographically signed
RS256 JWT; the operator verifies it against an embedded public key and re-reads
the Secret every 5 minutes. No internet connection is needed to install, validate,
or run Enterprise features.

Enforcement is honour-system. Each Enterprise component logs a one-time startup
reminder when it runs without a valid license, but the features keep working. The
license genuinely gates only dashboard white-labelling and Arena Fleet
source/job/limit checks — see
[License enforcement](/explanation/platform/licensing/#license-enforcement).

## License Activation (Optional)

For enterprise-tier licenses, the operator can register the cluster with the
Altaira Labs license server to **count activations** for the sales relationship:

- Each license has a maximum number of cluster activations
- On install the operator activates once and heartbeats every 24 hours
- You can view and manage activations in **Settings** → **License** → **Activations**

Activation is telemetry, not a gate: if the phone-home fails or the activation
limit is exceeded, Omnia records a Kubernetes warning event but does **not**
disable any feature.

### Deactivate a Cluster

If you want to free an activation slot when moving a license to a new cluster:

1. Open **Settings** → **License** → **Activations**
2. Find the cluster you want to deactivate
3. Click **Deactivate**

The activation slot is now available for a new cluster.

### Offline/Air-Gapped Installations

No special license is required for air-gapped clusters — validation is fully
offline. The optional activation phone-home simply logs a warning event when it
cannot reach the license server, and Enterprise features continue to run.

## Troubleshooting

### License Not Recognized

Enterprise features run on the `enterprise.enabled` flag, so they are not "locked"
by a missing license — but white-labelling and Arena Fleet limits *are* license-gated.
If the dashboard still shows the default theme or Arena rejects Git/OCI sources
after installing a license:

1. **Check the Secret exists**:
   ```bash
   kubectl get secret arena-license -n omnia-system
   ```

2. **Verify the Secret has the correct key**:
   ```bash
   kubectl get secret arena-license -n omnia-system -o jsonpath='{.data.license}' | base64 -d | head -c 50
   ```
   Should show the start of your JWT: `eyJhbGciOiJSUzI1NiI...`

3. **Check operator logs for validation errors**:
   ```bash
   kubectl logs -l app.kubernetes.io/name=omnia -n omnia-system | grep -i "license\|validation"
   ```

### License Expired

If your license has expired:

1. Your agents and the enterprise memory/privacy/policy services keep running; a
   startup license reminder is logged
2. Dashboard white-labelling reverts to the Omnia default theme, and Arena
   admission webhooks reject new enterprise-tier ArenaSource / ArenaJob resources
3. A warning banner appears in the dashboard
4. Contact support to renew your license

### Activation Failed

Activation is optional telemetry (see [License Activation](#license-activation-optional)),
so a failed activation does **not** disable any feature — Omnia just records a
Kubernetes warning event. If you want activation tracking to succeed:

1. **Check network connectivity** to `https://license.altairalabs.ai`
2. **Verify activation slots** are available (check dashboard)
3. **Deactivate unused clusters** if at the activation limit

Air-gapped clusters need no offline activation — validation is fully offline and
features run regardless of the phone-home.

## Next Steps

- [Licensing & Features](/explanation/platform/licensing/) - Compare Open Core vs Enterprise features
- [Configure Arena S3 Storage](/how-to/evaluation/configure-arena-s3-storage/) - Set up artifact storage (Enterprise)
- [Setup Scheduled Jobs](/how-to/evaluation/setup-arena-scheduled-jobs/) - Configure job scheduling (Enterprise)
