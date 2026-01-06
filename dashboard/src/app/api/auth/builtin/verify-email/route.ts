/**
 * Built-in authentication email verification endpoint.
 *
 * POST /api/auth/builtin/verify-email - Verify email with token
 *
 * Body:
 * - token: Email verification token (from email)
 */

import { NextRequest, NextResponse } from "next/server";
import { getAuthConfig } from "@/lib/auth/config";
import { getUserStore, hashToken } from "@/lib/auth/builtin";

export async function POST(request: NextRequest) {
  const authConfig = getAuthConfig();

  // Check we're in builtin mode
  if (authConfig.mode !== "builtin") {
    return NextResponse.json(
      { error: "Built-in authentication is not enabled" },
      { status: 400 }
    );
  }

  let body: { token?: string };
  try {
    body = await request.json();
  } catch {
    return NextResponse.json(
      { error: "Invalid request body" },
      { status: 400 }
    );
  }

  const { token } = body;

  if (!token) {
    return NextResponse.json(
      { error: "Verification token is required" },
      { status: 400 }
    );
  }

  try {
    const store = await getUserStore();

    // Hash the token for lookup
    const tokenHash = hashToken(token);

    // Find the token
    const verificationToken = await store.getEmailVerificationToken(tokenHash);

    if (!verificationToken) {
      return NextResponse.json(
        { error: "Invalid or expired verification token" },
        { status: 400 }
      );
    }

    // Check if token is expired
    if (verificationToken.expiresAt < new Date()) {
      return NextResponse.json(
        { error: "Verification token has expired. Please request a new one." },
        { status: 400 }
      );
    }

    // Check if token was already used
    if (verificationToken.usedAt) {
      return NextResponse.json(
        { error: "This verification token has already been used" },
        { status: 400 }
      );
    }

    // Verify user exists
    const user = await store.getUserById(verificationToken.userId);
    if (!user) {
      return NextResponse.json(
        { error: "User not found" },
        { status: 400 }
      );
    }

    // Check if already verified
    if (user.emailVerified) {
      return NextResponse.json({
        success: true,
        message: "Email is already verified. You can log in.",
        alreadyVerified: true,
      });
    }

    // Verify the email
    await store.verifyEmail(verificationToken.id, user.id);

    return NextResponse.json({
      success: true,
      message: "Email verified successfully. You can now log in.",
    });
  } catch (error) {
    console.error("Verify email error:", error);
    return NextResponse.json(
      { error: "An error occurred verifying your email" },
      { status: 500 }
    );
  }
}
