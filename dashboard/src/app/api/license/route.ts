/**
 * API route for license information.
 *
 * GET  /api/license - Get the current resolved license (features + limits).
 * POST /api/license - Upload a license JWT, writing the operator's arena-license
 *                     Secret. Multipart form-data with a `license` file field.
 *
 * License resolution for GET (demo mode, operator fetch, open-core fallback)
 * lives in lib/license/resolve-server so it can be shared with /api/config. The
 * operator re-reads the Secret every 5 minutes, so an uploaded license can take
 * up to that long to show up in GET.
 */

import { NextRequest, NextResponse } from "next/server";
import { getUser } from "@/lib/auth";
import { Permission, userHasPermission } from "@/lib/auth/permissions";
import { getEffectiveLicense } from "@/lib/license/resolve-server";
import { parseLicenseJwt, writeLicenseSecret } from "@/lib/k8s/license-secret";
import { secretErrorResponse } from "@/lib/k8s/secret-error";

export async function GET(): Promise<NextResponse> {
  return NextResponse.json(await getEffectiveLicense());
}

/**
 * Read the uploaded license JWT out of the multipart form. Returns the trimmed
 * token, or null when no usable file/field was provided.
 */
async function readUploadedLicense(request: NextRequest): Promise<string | null> {
  const form = await request.formData();
  const field = form.get("license");
  if (field === null) {
    return null;
  }
  const text = typeof field === "string" ? field : await field.text();
  const trimmed = text.trim();
  return trimmed.length > 0 ? trimmed : null;
}

export async function POST(request: NextRequest): Promise<NextResponse> {
  const user = await getUser();
  if (!userHasPermission(user, Permission.SETTINGS_EDIT)) {
    return NextResponse.json({ error: "Insufficient permissions" }, { status: 403 });
  }

  let jwt: string | null;
  try {
    jwt = await readUploadedLicense(request);
  } catch {
    return NextResponse.json(
      { error: "Invalid request: expected multipart form-data with a 'license' file" },
      { status: 400 }
    );
  }
  if (!jwt) {
    return NextResponse.json({ error: "No license provided" }, { status: 400 });
  }

  let claims;
  try {
    claims = parseLicenseJwt(jwt);
  } catch (error) {
    const message = error instanceof Error ? error.message : "Invalid license";
    return NextResponse.json({ error: message }, { status: 400 });
  }

  try {
    await writeLicenseSecret(jwt);
  } catch (error) {
    console.error("Failed to write license secret:", error);
    return secretErrorResponse(error, "Failed to store license");
  }

  console.info("license.upload", {
    user: user.id,
    tier: claims.tier,
    customer: claims.customer,
    expiresAt: claims.expiresAt,
  });

  return NextResponse.json(
    {
      tier: claims.tier,
      customer: claims.customer,
      expiresAt: claims.expiresAt,
      message:
        "License stored. It may take up to 5 minutes for the operator to apply it.",
    },
    { status: 200 }
  );
}
