/**
 * API Key management endpoints.
 *
 * GET /api/settings/api-keys - List user's API keys
 * POST /api/settings/api-keys - Create a new API key
 */

import { NextRequest, NextResponse } from "next/server";
import { getUser } from "@/lib/auth";
import { Permission, userHasPermission } from "@/lib/auth/permissions";
import {
  getApiKeyStore,
  getApiKeyConfig,
  isApiKeyAuthEnabled,
} from "@/lib/auth/api-keys";

/**
 * List user's API keys.
 */
export async function GET() {
  if (!isApiKeyAuthEnabled()) {
    return NextResponse.json(
      { error: "API key authentication is disabled" },
      { status: 403 }
    );
  }

  const user = await getUser();

  // Check permission - users can view their own keys
  if (!userHasPermission(user, Permission.API_KEYS_VIEW_OWN)) {
    return NextResponse.json(
      { error: "Insufficient permissions" },
      { status: 403 }
    );
  }

  const config = getApiKeyConfig();
  const store = getApiKeyStore();
  const keys = await store.listByUser(user.id);

  return NextResponse.json({
    keys,
    config: {
      storeType: config.storeType,
      allowCreate: config.allowCreate,
      maxKeysPerUser: config.maxKeysPerUser,
      defaultExpirationDays: config.defaultExpirationDays,
    },
  });
}

/**
 * Create a new API key.
 */
export async function POST(request: NextRequest) {
  if (!isApiKeyAuthEnabled()) {
    return NextResponse.json(
      { error: "API key authentication is disabled" },
      { status: 403 }
    );
  }

  // Check if key creation is allowed (not allowed in file-based mode)
  const config = getApiKeyConfig();
  if (!config.allowCreate) {
    return NextResponse.json(
      {
        error: "API key creation is not available in this mode",
        message:
          "Keys are managed via Kubernetes Secret. Contact your administrator to provision keys.",
      },
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

  // Parse request body
  let body: { name?: string; expiresInDays?: number | null };
  try {
    body = await request.json();
  } catch {
    return NextResponse.json(
      { error: "Invalid request body" },
      { status: 400 }
    );
  }

  // Validate name
  const name = body.name?.trim();
  if (!name || name.length < 1 || name.length > 255) {
    return NextResponse.json(
      { error: "Name must be between 1 and 255 characters" },
      { status: 400 }
    );
  }

  // Check max keys limit
  const store = getApiKeyStore();
  const existingKeys = await store.listByUser(user.id);

  if (existingKeys.length >= config.maxKeysPerUser) {
    return NextResponse.json(
      {
        error: `Maximum number of API keys (${config.maxKeysPerUser}) reached`,
      },
      { status: 400 }
    );
  }

  // Create the key
  const expiresInDays =
    body.expiresInDays === null
      ? null
      : body.expiresInDays ?? config.defaultExpirationDays;

  const newKey = await store.create(user.id, {
    name,
    role: user.role,
    expiresInDays: expiresInDays === 0 ? null : expiresInDays,
  });

  return NextResponse.json({ key: newKey }, { status: 201 });
}
