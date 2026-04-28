/**
 * Pure type-only helper for resolving a Provider's secretRef name.
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
  };
}

/**
 * Returns the secretRef name from a Provider's `spec.credential.secretRef`,
 * or undefined when the Provider doesn't carry one (envVar/filePath
 * credentials, or providers that don't need credentials).
 */
export function effectiveSecretRefName(
  provider: ProviderLike | null | undefined,
): string | undefined {
  return provider?.spec?.credential?.secretRef?.name;
}
