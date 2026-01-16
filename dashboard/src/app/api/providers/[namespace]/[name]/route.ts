/**
 * API route for managing individual Provider resources.
 *
 * PATCH /api/providers/:namespace/:name - Update provider (e.g., secretRef)
 */

import { NextRequest, NextResponse } from "next/server";
import { getUser } from "@/lib/auth";
import { Permission, userHasPermission } from "@/lib/auth/permissions";
import { updateProviderSecretRef, getProvider } from "@/lib/k8s/providers";

interface RouteParams {
  params: Promise<{
    namespace: string;
    name: string;
  }>;
}

/**
 * GET /api/providers/:namespace/:name
 * Get a single provider's details including whether its secret exists.
 */
export async function GET(
  request: NextRequest,
  { params }: RouteParams
): Promise<NextResponse> {
  try {
    const user = await getUser();
    if (!user || !userHasPermission(user, Permission.PROVIDERS_VIEW)) {
      return NextResponse.json({ error: "Forbidden" }, { status: 403 });
    }

    const { namespace, name } = await params;
    const provider = await getProvider(namespace, name);

    if (!provider) {
      return NextResponse.json({ error: "Provider not found" }, { status: 404 });
    }

    return NextResponse.json({ provider });
  } catch (error) {
    console.error("Failed to get provider:", error);
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "Failed to get provider" },
      { status: 500 }
    );
  }
}

/**
 * PATCH /api/providers/:namespace/:name
 * Update a provider's configuration.
 *
 * Body: { secretRef?: string | null }
 * - secretRef: name of the secret to use, or null to remove
 */
export async function PATCH(
  request: NextRequest,
  { params }: RouteParams
): Promise<NextResponse> {
  try {
    const user = await getUser();
    if (!user || !userHasPermission(user, Permission.PROVIDERS_EDIT)) {
      return NextResponse.json({ error: "Forbidden" }, { status: 403 });
    }

    const { namespace, name } = await params;

    let body: { secretRef?: string | null };
    try {
      body = await request.json();
    } catch {
      return NextResponse.json({ error: "Invalid JSON body" }, { status: 400 });
    }

    // Validate request
    if (!("secretRef" in body)) {
      return NextResponse.json(
        { error: "secretRef field is required" },
        { status: 400 }
      );
    }

    // Validate secretRef value
    if (body.secretRef !== null && typeof body.secretRef !== "string") {
      return NextResponse.json(
        { error: "secretRef must be a string or null" },
        { status: 400 }
      );
    }

    // Validate secret name format if provided
    if (body.secretRef !== null) {
      const validName = /^[a-z0-9]([a-z0-9-]*[a-z0-9])?$/.test(body.secretRef);
      if (!validName || body.secretRef.length > 253) {
        return NextResponse.json(
          { error: "Invalid secret name format (must be DNS-1123 subdomain)" },
          { status: 400 }
        );
      }
    }

    const provider = await updateProviderSecretRef(namespace, name, body.secretRef);

    return NextResponse.json({ provider });
  } catch (error) {
    console.error("Failed to update provider:", error);

    // Check for not found
    if (error instanceof Error && error.message.includes("not found")) {
      return NextResponse.json({ error: "Provider not found" }, { status: 404 });
    }

    return NextResponse.json(
      { error: error instanceof Error ? error.message : "Failed to update provider" },
      { status: 500 }
    );
  }
}
