/**
 * Individual secret management endpoint.
 *
 * GET /api/secrets/{namespace}/{name}     - Get secret metadata
 * DELETE /api/secrets/{namespace}/{name}  - Delete a secret
 *
 * Security:
 * - All endpoints require authentication
 * - Never returns secret values, only metadata
 * - Only manages secrets with label omnia.altairalabs.ai/type=credentials
 */

import { NextRequest, NextResponse } from "next/server";
import { getUser } from "@/lib/auth";
import { Permission, userHasPermission } from "@/lib/auth/permissions";
import { getSecret, deleteSecret } from "@/lib/k8s/secrets";

interface RouteParams {
  params: Promise<{ namespace: string; name: string }>;
}

/**
 * Get a single secret's metadata.
 */
export async function GET(
  _request: NextRequest,
  { params }: RouteParams
) {
  const user = await getUser();

  // Check permission
  if (!userHasPermission(user, Permission.CREDENTIALS_VIEW)) {
    return NextResponse.json(
      { error: "Insufficient permissions" },
      { status: 403 }
    );
  }

  const { namespace, name } = await params;

  if (!namespace || !name) {
    return NextResponse.json(
      { error: "namespace and name are required" },
      { status: 400 }
    );
  }

  try {
    const secret = await getSecret(namespace, name);

    if (!secret) {
      return NextResponse.json(
        { error: "Secret not found" },
        { status: 404 }
      );
    }

    return NextResponse.json({ secret });
  } catch (error) {
    console.error("Failed to get secret:", error);
    return NextResponse.json(
      { error: "Failed to get secret" },
      { status: 500 }
    );
  }
}

/**
 * Delete a secret.
 * Only deletes if it has the credentials label.
 */
export async function DELETE(
  _request: NextRequest,
  { params }: RouteParams
) {
  const user = await getUser();

  // Check permission
  if (!userHasPermission(user, Permission.CREDENTIALS_DELETE)) {
    return NextResponse.json(
      { error: "Insufficient permissions" },
      { status: 403 }
    );
  }

  const { namespace, name } = await params;

  if (!namespace || !name) {
    return NextResponse.json(
      { error: "namespace and name are required" },
      { status: 400 }
    );
  }

  try {
    const deleted = await deleteSecret(namespace, name);

    if (!deleted) {
      return NextResponse.json(
        { error: "Secret not found" },
        { status: 404 }
      );
    }

    console.info("secrets.delete", {
      user: user.id,
      namespace,
      name,
    });

    return NextResponse.json({ success: true });
  } catch (error) {
    console.error("Failed to delete secret:", error);

    // Handle specific errors
    if (
      error instanceof Error &&
      error.message.includes("not a managed credential secret")
    ) {
      return NextResponse.json(
        { error: error.message },
        { status: 409 }
      );
    }

    return NextResponse.json(
      { error: "Failed to delete secret" },
      { status: 500 }
    );
  }
}
