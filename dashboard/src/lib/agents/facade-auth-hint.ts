import type { ExternalAuth } from "@/types/agent-runtime";

export interface AuthHint {
  label: string;
  detail?: string;
}

/**
 * Derives a human-readable auth hint from spec.externalAuth.
 * Returns the first matching auth mode in precedence order.
 */
export function facadeAuthHint(externalAuth: ExternalAuth | undefined): AuthHint {
  if (!externalAuth) {
    return { label: "Management-plane only" };
  }

  if (externalAuth.sharedToken) {
    const secretName = externalAuth.sharedToken.secretRef.name;
    return {
      label: "Bearer token",
      detail: secretName ? `Secret \`${secretName}\`` : undefined,
    };
  }

  if (externalAuth.apiKeys) {
    return { label: "API key (Bearer)" };
  }

  if (externalAuth.oidc) {
    return { label: "OIDC", detail: externalAuth.oidc.issuer };
  }

  if (externalAuth.edgeTrust) {
    return { label: "Edge-trusted headers" };
  }

  return { label: "Management-plane only" };
}
