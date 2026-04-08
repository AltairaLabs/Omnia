/**
 * Built-in authentication forgot password endpoint.
 *
 * POST /api/auth/builtin/forgot-password - Request password reset
 *
 * Body:
 * - email: Email address of the account
 */

import { NextRequest, NextResponse } from "next/server";
import { getAuthConfig } from "@/lib/auth/config";
import {
  getUserStore,
  getBuiltinConfig,
  generateSecureToken,
} from "@/lib/auth/builtin";
import { sendEmail } from "@/lib/email/sender";

export async function POST(request: NextRequest) {
  const authConfig = getAuthConfig();

  // Check we're in builtin mode
  if (authConfig.mode !== "builtin") {
    return NextResponse.json(
      { error: "Built-in authentication is not enabled" },
      { status: 400 }
    );
  }

  let body: { email?: string };
  try {
    body = await request.json();
  } catch {
    return NextResponse.json(
      { error: "Invalid request body" },
      { status: 400 }
    );
  }

  const { email } = body;

  if (!email) {
    return NextResponse.json(
      { error: "Email is required" },
      { status: 400 }
    );
  }

  try {
    const store = await getUserStore();
    const config = getBuiltinConfig();

    // Find user by email
    const user = await store.getUserByEmail(email);

    // Always return success to prevent email enumeration
    // Even if user doesn't exist, we pretend we sent an email
    if (!user) {
      return NextResponse.json({
        success: true,
        message: "If an account exists with this email, a reset link has been sent.",
      });
    }

    // Clean up any existing tokens for this user
    await store.deleteExpiredPasswordResetTokens();

    // Generate reset token
    const { token, hash } = generateSecureToken();
    const expiresAt = new Date(
      Date.now() + config.resetTokenExpiration * 1000
    );

    await store.createPasswordResetToken(user.id, hash, expiresAt);

    const resetUrl = `${process.env.OMNIA_BASE_URL || "http://localhost:3000"}/reset-password?token=${token}`;
    await sendEmail({
      to: user.email,
      subject: "Password Reset Request",
      text: `Click the following link to reset your password: ${resetUrl}\n\nThis link expires at ${expiresAt.toISOString()}.`,
      html: `<p>Click <a href="${resetUrl}">here</a> to reset your password.</p><p>This link expires at ${expiresAt.toISOString()}.</p>`,
    });

    return NextResponse.json({
      success: true,
      message: "If an account exists with this email, a reset link has been sent.",
    });
  } catch (error) {
    console.error("Forgot password error:", error);
    return NextResponse.json(
      { error: "An error occurred processing your request" },
      { status: 500 }
    );
  }
}
