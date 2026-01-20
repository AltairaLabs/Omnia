/**
 * API route for managing individual license activations.
 *
 * DELETE /api/license/activations/[fingerprint] - Deactivate a cluster
 */

import { NextResponse } from "next/server";

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
 * DELETE /api/license/activations/[fingerprint]
 *
 * Deactivate a specific cluster.
 */
export async function DELETE(
  request: Request,
  { params }: { params: Promise<{ fingerprint: string }> }
): Promise<NextResponse> {
  try {
    const { fingerprint } = await params;

    if (isDemoMode()) {
      // In demo mode, simulate successful deactivation
      return NextResponse.json({ success: true });
    }

    const operatorUrl = getOperatorApiUrl();
    if (!operatorUrl) {
      return NextResponse.json(
        { error: "Operator API not configured" },
        { status: 503 }
      );
    }

    const response = await fetch(
      `${operatorUrl}/api/v1/license/activations/${fingerprint}`,
      {
        method: "DELETE",
        headers: { "Content-Type": "application/json" },
      }
    );

    if (!response.ok) {
      const errorText = await response.text();
      console.warn("Failed to deactivate cluster:", response.status, errorText);
      return NextResponse.json(
        { error: "Failed to deactivate cluster" },
        { status: response.status }
      );
    }

    return NextResponse.json({ success: true });
  } catch (error) {
    console.error("Failed to deactivate cluster:", error);
    return NextResponse.json(
      { error: "Internal server error" },
      { status: 500 }
    );
  }
}
