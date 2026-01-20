/**
 * API route for license information.
 *
 * GET /api/license - Get current license information
 *
 * Returns the current license with features and limits.
 * In demo mode or when no license is found, returns the open-core license.
 */

import { NextResponse } from "next/server";
import { OPEN_CORE_LICENSE, type License } from "@/types/license";

/**
 * Check if demo mode is enabled.
 */
function isDemoMode(): boolean {
  return process.env.NEXT_PUBLIC_DEMO_MODE === "true";
}

/**
 * Mock enterprise license for testing.
 */
const MOCK_ENTERPRISE_LICENSE: License = {
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
  },
  limits: {
    maxScenarios: 0, // unlimited
    maxWorkerReplicas: 0, // unlimited
  },
  issuedAt: new Date().toISOString(),
  expiresAt: new Date(Date.now() + 365 * 24 * 60 * 60 * 1000).toISOString(),
};

/**
 * Get the operator API URL for license endpoint.
 */
function getOperatorApiUrl(): string | undefined {
  return process.env.OPERATOR_API_URL;
}

/**
 * Fetch license from the operator API.
 */
async function fetchLicenseFromOperator(): Promise<License | null> {
  const operatorUrl = getOperatorApiUrl();
  if (!operatorUrl) {
    return null;
  }

  try {
    const response = await fetch(`${operatorUrl}/api/v1/license`, {
      method: "GET",
      headers: {
        "Content-Type": "application/json",
      },
      // Don't cache license - let the hook handle caching
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

/**
 * GET /api/license
 *
 * Get the current license information.
 *
 * Priority:
 * 1. Demo mode with enterprise flag -> Enterprise license
 * 2. Operator API -> Actual license
 * 3. Fallback -> Open-core license
 */
export async function GET(): Promise<NextResponse> {
  try {
    // In demo mode, check if we should show enterprise features
    if (isDemoMode()) {
      // Check for demo enterprise flag (useful for testing enterprise UI)
      const showEnterprise = process.env.DEMO_ENTERPRISE_LICENSE === "true";
      return NextResponse.json(showEnterprise ? MOCK_ENTERPRISE_LICENSE : OPEN_CORE_LICENSE);
    }

    // Try to fetch from operator API
    const operatorLicense = await fetchLicenseFromOperator();
    if (operatorLicense) {
      return NextResponse.json(operatorLicense);
    }

    // Fall back to open-core license
    return NextResponse.json(OPEN_CORE_LICENSE);
  } catch (error) {
    console.error("Failed to get license:", error);
    // On error, return open-core license (fail open for basic features)
    return NextResponse.json(OPEN_CORE_LICENSE);
  }
}
