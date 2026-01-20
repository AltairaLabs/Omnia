/**
 * API route for license activations.
 *
 * GET /api/license/activations - List all cluster activations
 */

import { NextResponse } from "next/server";

/**
 * Cluster activation info.
 */
export interface ClusterActivation {
  fingerprint: string;
  clusterName: string;
  activatedAt: string;
  lastSeen: string;
}

/**
 * Check if demo mode is enabled.
 */
function isDemoMode(): boolean {
  return process.env.NEXT_PUBLIC_DEMO_MODE === "true";
}

/**
 * Get the operator API URL.
 */
function getOperatorApiUrl(): string | undefined {
  return process.env.OPERATOR_API_URL;
}

/**
 * Mock activations for demo mode.
 */
const MOCK_ACTIVATIONS: ClusterActivation[] = [
  {
    fingerprint: "demo-cluster-1",
    clusterName: "production-us-east",
    activatedAt: new Date(Date.now() - 30 * 24 * 60 * 60 * 1000).toISOString(),
    lastSeen: new Date(Date.now() - 5 * 60 * 1000).toISOString(),
  },
  {
    fingerprint: "demo-cluster-2",
    clusterName: "staging-eu-west",
    activatedAt: new Date(Date.now() - 7 * 24 * 60 * 60 * 1000).toISOString(),
    lastSeen: new Date(Date.now() - 60 * 60 * 1000).toISOString(),
  },
];

/**
 * GET /api/license/activations
 *
 * List all cluster activations for the license.
 */
export async function GET(): Promise<NextResponse> {
  try {
    if (isDemoMode()) {
      return NextResponse.json({ activations: MOCK_ACTIVATIONS });
    }

    const operatorUrl = getOperatorApiUrl();
    if (!operatorUrl) {
      return NextResponse.json({ activations: [] });
    }

    const response = await fetch(`${operatorUrl}/api/v1/license/activations`, {
      method: "GET",
      headers: { "Content-Type": "application/json" },
      cache: "no-store",
    });

    if (!response.ok) {
      console.warn("Failed to fetch activations:", response.status);
      return NextResponse.json({ activations: [] });
    }

    return NextResponse.json(await response.json());
  } catch (error) {
    console.error("Failed to list activations:", error);
    return NextResponse.json({ activations: [] });
  }
}
