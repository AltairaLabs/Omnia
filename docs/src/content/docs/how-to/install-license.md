---
title: "Install with a License"
description: "Configure Omnia with an Enterprise license key"
sidebar:
  order: 1
---

This guide covers installing Omnia with an Enterprise license to unlock advanced features like Git sources, load testing, and distributed workers.

For feature comparison between Open Core and Enterprise, see [Licensing & Features](/explanation/licensing/).

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

## License Activation

Enterprise licenses support activation tracking to prevent unauthorized sharing:

- Each license has a maximum number of cluster activations
- When installed, Omnia automatically activates with the license server
- You can view and manage activations in **Settings** → **License** → **Activations**

### Deactivate a Cluster

If you need to move your license to a new cluster:

1. Open **Settings** → **License** → **Activations**
2. Find the cluster you want to deactivate
3. Click **Deactivate**

The activation slot is now available for a new cluster.

### Offline/Air-Gapped Installations

For environments without internet access, contact support to receive a pre-activated license with your cluster fingerprint embedded.

## Troubleshooting

### License Not Recognized

If features remain locked after installing a license:

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

1. Features will degrade to Open Core functionality
2. A warning banner appears in the dashboard
3. Contact support to renew your license

### Activation Failed

If license activation fails:

1. **Check network connectivity** to `https://license.altairalabs.ai`
2. **Verify activation slots** are available (check dashboard)
3. **Deactivate unused clusters** if at the activation limit

For air-gapped environments, contact support for offline activation.

## Next Steps

- [Licensing & Features](/explanation/licensing/) - Compare Open Core vs Enterprise features
- [Configure Arena S3 Storage](/how-to/configure-arena-s3-storage/) - Set up artifact storage (Enterprise)
- [Setup Scheduled Jobs](/how-to/setup-arena-scheduled-jobs/) - Configure job scheduling (Enterprise)
