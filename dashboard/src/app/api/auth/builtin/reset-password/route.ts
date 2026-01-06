/**
 * Built-in authentication reset password endpoint.
 *
 * POST /api/auth/builtin/reset-password - Reset password with token
 *
 * Body:
 * - token: Password reset token (from email)
 * - password: New password
 */

import { NextRequest, NextResponse } from "next/server";
import { getAuthConfig } from "@/lib/auth/config";
import {
  getUserStore,
  getBuiltinConfig,
  hashToken,
  hashPassword,
  validatePassword,
} from "@/lib/auth/builtin";

export async function POST(request: NextRequest) {
  const authConfig = getAuthConfig();

  // Check we're in builtin mode
  if (authConfig.mode !== "builtin") {
    return NextResponse.json(
      { error: "Built-in authentication is not enabled" },
      { status: 400 }
    );
  }

  let body: { token?: string; password?: string };
  try {
    body = await request.json();
  } catch {
    return NextResponse.json(
      { error: "Invalid request body" },
      { status: 400 }
    );
  }

  const { token, password } = body;

  if (!token || !password) {
    return NextResponse.json(
      { error: "Token and new password are required" },
      { status: 400 }
    );
  }

  const config = getBuiltinConfig();

  // Validate new password
  const passwordErrors = validatePassword(password, config.minPasswordLength);
  if (passwordErrors.length > 0) {
    return NextResponse.json(
      { error: passwordErrors[0], field: "password" },
      { status: 400 }
    );
  }

  try {
    const store = await getUserStore();

    // Hash the token for lookup
    const tokenHash = hashToken(token);

    // Find the token
    const resetToken = await store.getPasswordResetToken(tokenHash);

    if (!resetToken) {
      return NextResponse.json(
        { error: "Invalid or expired reset token" },
        { status: 400 }
      );
    }

    // Check if token is expired
    if (resetToken.expiresAt < new Date()) {
      return NextResponse.json(
        { error: "Reset token has expired. Please request a new one." },
        { status: 400 }
      );
    }

    // Check if token was already used
    if (resetToken.usedAt) {
      return NextResponse.json(
        { error: "This reset token has already been used" },
        { status: 400 }
      );
    }

    // Verify user exists
    const user = await store.getUserById(resetToken.userId);
    if (!user) {
      return NextResponse.json(
        { error: "User not found" },
        { status: 400 }
      );
    }

    // Hash the new password
    const newPasswordHash = await hashPassword(password);

    // Update password and mark token as used
    await store.updatePassword(user.id, newPasswordHash);
    await store.markPasswordResetTokenUsed(resetToken.id);

    // Unlock account if it was locked
    if (user.lockedUntil) {
      await store.unlockUser(user.id);
    }

    return NextResponse.json({
      success: true,
      message: "Password has been reset successfully. You can now log in.",
    });
  } catch (error) {
    console.error("Reset password error:", error);
    return NextResponse.json(
      { error: "An error occurred resetting your password" },
      { status: 500 }
    );
  }
}
