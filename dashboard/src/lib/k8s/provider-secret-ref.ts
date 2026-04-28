/**
 * Pure type-only helper for resolving a Provider's effective secretRef.
 *
 * Lives in a separate file from `providers.ts` because that module
 * imports `@kubernetes/client-node`, which depends on Node-only modules
 * (`dns`, `net`, `tls`) that webpack can't bundle for the browser.
 * Client components ("use client") that just need to *read* a Provider's
 * secret name should import from here, not `./providers`, otherwise the
 * whole k8s SDK gets dragged into the client bundle and the build fails
 * with "Module not found: Can't resolve 'dns'".
 */

interface ProviderLike {
  spec?: {
    credential?: { secretRef?: { name?: string } };
    secretRef?: { name?: string };
  };
}

/**
 * Returns the effective secretRef name for a Provider, checking the new
 * `spec.credential.secretRef` first and falling back to legacy
 * `spec.secretRef`. Returns undefined when neither is set.
 *
 * Mirrors the operator's pkg/k8s/EffectiveSecretRef so the dashboard
 * shows the same secret the runtime uses regardless of which shape the
 * Provider author chose.
 */
export function effectiveSecretRefName(
  provider: ProviderLike | null | undefined,
): string | undefined {
  return (
    provider?.spec?.credential?.secretRef?.name ?? provider?.spec?.secretRef?.name
  );
}
