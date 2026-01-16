/**
 * Provider credentials (secrets) management API.
 *
 * GET /api/secrets?namespace={ns}  - List secrets (filtered by label)
 * POST /api/secrets                - Create or update secret
 *
 * Security:
 * - All endpoints require authentication
 * - Never returns secret values, only metadata
 * - Only manages secrets with label omnia.altairalabs.ai/type=credentials
 */

import { NextRequest, NextResponse } from "next/server";
import { getUser } from "@/lib/auth";
import { Permission, userHasPermission } from "@/lib/auth/permissions";
import {
  listSecrets,
  createOrUpdateSecret,
  type SecretWriteRequest,
} from "@/lib/k8s/secrets";

/**
 * Validate the secret write request body.
 * Returns an error message if invalid, null if valid.
 */
function validateSecretRequest(body: unknown): string | null {
  if (!body || typeof body !== "object") {
    return "Invalid request body";
  }

  const request = body as Record<string, unknown>;

  if (!request.namespace || typeof request.namespace !== "string") {
    return "namespace is required";
  }

  if (!request.name || typeof request.name !== "string") {
    return "name is required";
  }

  if (!request.data || typeof request.data !== "object") {
    return "data is required";
  }

  // Validate secret name (DNS-1123 subdomain)
  const nameRegex = /^[a-z0-9]([-a-z0-9]*[a-z0-9])?$/;
  if (!nameRegex.test(request.name) || request.name.length > 253) {
    return "Invalid secret name. Must be a valid DNS subdomain name (lowercase, alphanumeric, hyphens allowed, max 253 chars)";
  }

  // Validate keys (alphanumeric, underscores, hyphens, dots)
  const keyRegex = /^[a-zA-Z0-9_.-]+$/;
  const keys = Object.keys(request.data as Record<string, unknown>);

  for (const key of keys) {
    if (!keyRegex.test(key) || key.length > 253) {
      return `Invalid key name: ${key}. Keys must be alphanumeric with underscores, hyphens, or dots (max 253 chars)`;
    }
  }

  // Check for at least one key
  if (keys.length === 0) {
    return "At least one key-value pair is required";
  }

  return null;
}

/**
 * List secrets with the credentials label.
 * Returns metadata only - never secret values.
 */
export async function GET(request: NextRequest) {
  const user = await getUser();

  // Check permission
  if (!userHasPermission(user, Permission.CREDENTIALS_VIEW)) {
    return NextResponse.json(
      { error: "Insufficient permissions" },
      { status: 403 }
    );
  }

  // Get optional namespace filter
  const searchParams = request.nextUrl.searchParams;
  const namespace = searchParams.get("namespace") || undefined;

  try {
    const secrets = await listSecrets(namespace);

    console.info("secrets.list", {
      user: user.id,
      namespace: namespace || "all",
      count: secrets.length,
    });

    return NextResponse.json({ secrets });
  } catch (error) {
    console.error("Failed to list secrets:", error);
    return NextResponse.json(
      { error: "Failed to list secrets" },
      { status: 500 }
    );
  }
}

/**
 * Create or update a secret.
 */
export async function POST(request: NextRequest) {
  const user = await getUser();

  // Parse request body
  let body: unknown;
  try {
    body = await request.json();
  } catch {
    return NextResponse.json(
      { error: "Invalid request body" },
      { status: 400 }
    );
  }

  // Validate request
  const validationError = validateSecretRequest(body);
  if (validationError) {
    return NextResponse.json({ error: validationError }, { status: 400 });
  }

  // Type assertion after validation
  const secretRequest = body as SecretWriteRequest;

  // Check permission - need CREATE for new secrets, EDIT for existing
  const canCreate = userHasPermission(user, Permission.CREDENTIALS_CREATE);
  const canEdit = userHasPermission(user, Permission.CREDENTIALS_EDIT);

  if (!canCreate && !canEdit) {
    return NextResponse.json(
      { error: "Insufficient permissions" },
      { status: 403 }
    );
  }

  try {
    const secret = await createOrUpdateSecret(secretRequest);

    console.info("secrets.createOrUpdate", {
      user: user.id,
      namespace: secretRequest.namespace,
      name: secretRequest.name,
      keys: Object.keys(secretRequest.data),
      providerType: secretRequest.providerType,
    });

    return NextResponse.json({ secret }, { status: 201 });
  } catch (error) {
    console.error("Failed to create/update secret:", error);

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
      { error: "Failed to create/update secret" },
      { status: 500 }
    );
  }
}
