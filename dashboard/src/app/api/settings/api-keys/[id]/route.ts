/**
 * Individual API Key management endpoint.
 *
 * DELETE /api/settings/api-keys/[id] - Revoke an API key
 */

import { NextRequest, NextResponse } from "next/server";
import { getUser } from "@/lib/auth";
import { Permission, userHasPermission } from "@/lib/auth/permissions";
import { getApiKeyStore, isApiKeyAuthEnabled } from "@/lib/auth/api-keys";

interface RouteParams {
  params: Promise<{ id: string }>;
}

/**
 * Delete (revoke) an API key.
 */
export async function DELETE(
  _request: NextRequest,
  { params }: RouteParams
) {
  if (!isApiKeyAuthEnabled()) {
    return NextResponse.json(
      { error: "API key authentication is disabled" },
      { status: 403 }
    );
  }

  const user = await getUser();

  // Check permission
  if (!userHasPermission(user, Permission.API_KEYS_MANAGE_OWN)) {
    return NextResponse.json(
      { error: "Insufficient permissions" },
      { status: 403 }
    );
  }

  const { id } = await params;

  if (!id) {
    return NextResponse.json(
      { error: "Key ID is required" },
      { status: 400 }
    );
  }

  const store = getApiKeyStore();
  const deleted = await store.delete(id, user.id);

  if (!deleted) {
    return NextResponse.json(
      { error: "API key not found" },
      { status: 404 }
    );
  }

  return NextResponse.json({ success: true });
}
