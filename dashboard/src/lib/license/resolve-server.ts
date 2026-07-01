/**
 * Server-side license resolution.
 *
 * Single source of truth for "what license is in effect", shared by the
 * /api/license and /api/config route handlers. Priority:
 *   1. demo mode + DEMO_ENTERPRISE_LICENSE -> mock enterprise license
 *   2. demo mode                            -> open-core
 *   3. operator API                         -> the real license
 *   4. fallback / error                     -> open-core (fail open)
 */

import { OPEN_CORE_LICENSE, type License } from "@/types/license";

function isDemoMode(): boolean {
  return process.env.NEXT_PUBLIC_DEMO_MODE === "true";
}

/** Mock enterprise license used in demo mode for testing enterprise UI. */
export const MOCK_ENTERPRISE_LICENSE: License = {
  id: "demo-enterprise",
  tier: "enterprise",
  customer: "Demo Enterprise User",
  features: {
    gitSource: true,
    ociSource: true,
    s3Source: true,
    loadTesting: true,
    dataGeneration: true,
    scheduling: true,
    distributedWorkers: true,
    whiteLabel: true,
  },
  limits: {
    maxScenarios: 0, // unlimited
    maxWorkerReplicas: 0, // unlimited
  },
  issuedAt: new Date().toISOString(),
  expiresAt: new Date(Date.now() + 365 * 24 * 60 * 60 * 1000).toISOString(),
};

async function fetchLicenseFromOperator(): Promise<License | null> {
  const operatorUrl = process.env.OPERATOR_API_URL;
  if (!operatorUrl) {
    return null;
  }
  try {
    const response = await fetch(`${operatorUrl}/api/v1/license`, {
      method: "GET",
      headers: { "Content-Type": "application/json" },
      cache: "no-store",
    });
    if (!response.ok) {
      console.warn("Failed to fetch license from operator:", response.status);
      return null;
    }
    return response.json();
  } catch (error) {
    console.warn("Error fetching license from operator:", error);
    return null;
  }
}

/** Resolve the license currently in effect (fail-open to open-core). */
export async function getEffectiveLicense(): Promise<License> {
  try {
    if (isDemoMode()) {
      const showEnterprise = process.env.DEMO_ENTERPRISE_LICENSE === "true";
      return showEnterprise ? MOCK_ENTERPRISE_LICENSE : OPEN_CORE_LICENSE;
    }
    return (await fetchLicenseFromOperator()) ?? OPEN_CORE_LICENSE;
  } catch (error) {
    console.error("Failed to get license:", error);
    return OPEN_CORE_LICENSE;
  }
}
