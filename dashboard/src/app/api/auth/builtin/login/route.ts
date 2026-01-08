/**
 * Built-in authentication login endpoint.
 *
 * POST /api/auth/builtin/login - Authenticate with username/email and password
 *
 * Body:
 * - identity: Username or email
 * - password: Password
 * - remember: Optional boolean for extended session
 */

import { NextRequest, NextResponse } from "next/server";
import { getAuthConfig } from "@/lib/auth/config";
import { getSession } from "@/lib/auth/session";
import {
  getUserStore,
  getBuiltinConfig,
  verifyPassword,
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

  let body: { identity?: string; password?: string; remember?: boolean };
  try {
    body = await request.json();
  } catch {
    return NextResponse.json(
      { error: "Invalid request body" },
      { status: 400 }
    );
  }

  const { identity, password } = body;

  if (!identity || !password) {
    return NextResponse.json(
      { error: "Username/email and password are required" },
      { status: 400 }
    );
  }

  try {
    const store = await getUserStore();
    const config = getBuiltinConfig();

    // Find user by email or username
    let user = await store.getUserByEmail(identity);
    if (!user) {
      user = await store.getUserByUsername(identity);
    }

    if (!user) {
      // Don't reveal whether user exists
      return NextResponse.json(
        { error: "Invalid credentials" },
        { status: 401 }
      );
    }

    // Check if account is locked
    if (user.lockedUntil && user.lockedUntil > new Date()) {
      const remainingMinutes = Math.ceil(
        (user.lockedUntil.getTime() - Date.now()) / 60000
      );
      return NextResponse.json(
        {
          error: `Account is locked. Try again in ${remainingMinutes} minute(s).`,
          locked: true,
          lockedUntil: user.lockedUntil.toISOString(),
        },
        { status: 423 }
      );
    }

    // Verify password
    const isValid = await verifyPassword(password, user.passwordHash);

    if (!isValid) {
      // Record failed attempt
      await store.recordFailedLogin(user.id);

      // Check if we should lock the account
      const updatedUser = await store.getUserById(user.id);
      if (
        updatedUser &&
        updatedUser.failedLoginAttempts >= config.maxFailedAttempts
      ) {
        const lockUntil = new Date(Date.now() + config.lockoutDuration * 1000);
        await store.lockUser(user.id, lockUntil);

        return NextResponse.json(
          {
            error: `Too many failed attempts. Account locked for ${Math.ceil(config.lockoutDuration / 60)} minute(s).`,
            locked: true,
            lockedUntil: lockUntil.toISOString(),
          },
          { status: 423 }
        );
      }

      return NextResponse.json(
        { error: "Invalid credentials" },
        { status: 401 }
      );
    }

    // Check email verification if required
    if (config.verifyEmail && !user.emailVerified) {
      return NextResponse.json(
        {
          error: "Please verify your email address before logging in",
          emailNotVerified: true,
        },
        { status: 403 }
      );
    }

    // Record successful login
    await store.recordLogin(user.id);

    // Create session
    const session = await getSession();
    session.user = {
      id: user.id,
      username: user.username,
      email: user.email,
      displayName: user.displayName,
      groups: [], // Built-in auth doesn't use groups
      role: user.role,
      provider: "builtin",
    };
    session.createdAt = Date.now();

    await session.save();

    return NextResponse.json({
      success: true,
      user: {
        id: user.id,
        username: user.username,
        email: user.email,
        displayName: user.displayName,
        role: user.role,
      },
    });
  } catch (error) {
    console.error("Login error:", error);
    return NextResponse.json(
      { error: "An error occurred during login" },
      { status: 500 }
    );
  }
}
