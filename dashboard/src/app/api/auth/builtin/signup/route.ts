/**
 * Built-in authentication signup endpoint.
 *
 * POST /api/auth/builtin/signup - Create a new user account
 *
 * Body:
 * - username: Desired username
 * - email: Email address
 * - password: Password
 * - displayName: Optional display name
 */

import { NextRequest, NextResponse } from "next/server";
import { getAuthConfig } from "@/lib/auth/config";
import { getSession } from "@/lib/auth/session";
import {
  getUserStore,
  getBuiltinConfig,
  validatePassword,
  validateEmail,
  validateUsername,
  generateSecureToken,
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

  const config = getBuiltinConfig();

  // Check if signup is allowed
  if (!config.allowSignup) {
    return NextResponse.json(
      { error: "User registration is not enabled" },
      { status: 403 }
    );
  }

  let body: {
    username?: string;
    email?: string;
    password?: string;
    displayName?: string;
  };
  try {
    body = await request.json();
  } catch {
    return NextResponse.json(
      { error: "Invalid request body" },
      { status: 400 }
    );
  }

  const { username, email, password, displayName } = body;

  // Validate required fields
  if (!username || !email || !password) {
    return NextResponse.json(
      { error: "Username, email, and password are required" },
      { status: 400 }
    );
  }

  // Validate username
  const usernameErrors = validateUsername(username);
  if (usernameErrors.length > 0) {
    return NextResponse.json(
      { error: usernameErrors[0], field: "username" },
      { status: 400 }
    );
  }

  // Validate email
  if (!validateEmail(email)) {
    return NextResponse.json(
      { error: "Invalid email address", field: "email" },
      { status: 400 }
    );
  }

  // Validate password
  const passwordErrors = validatePassword(password, config.minPasswordLength);
  if (passwordErrors.length > 0) {
    return NextResponse.json(
      { error: passwordErrors[0], field: "password" },
      { status: 400 }
    );
  }

  try {
    const store = await getUserStore();

    // Check if email is already taken
    const existingByEmail = await store.getUserByEmail(email);
    if (existingByEmail) {
      return NextResponse.json(
        { error: "Email is already registered", field: "email" },
        { status: 409 }
      );
    }

    // Check if username is already taken
    const existingByUsername = await store.getUserByUsername(username);
    if (existingByUsername) {
      return NextResponse.json(
        { error: "Username is already taken", field: "username" },
        { status: 409 }
      );
    }

    // Create user
    const user = await store.createUser({
      username,
      email,
      password,
      displayName,
      emailVerified: !config.verifyEmail, // Auto-verify if verification not required
    });

    // If email verification is required, create verification token
    if (config.verifyEmail) {
      const { token, hash } = generateSecureToken();
      const expiresAt = new Date(
        Date.now() + config.verificationTokenExpiration * 1000
      );
      await store.createEmailVerificationToken(user.id, hash, expiresAt);

      // TODO: Send verification email
      // For now, log the token (in production, this would be sent via email)
      console.log(
        `Email verification token for ${email}: ${token} (expires: ${expiresAt.toISOString()})`
      );

      return NextResponse.json({
        success: true,
        message:
          "Account created. Please check your email to verify your account.",
        requiresVerification: true,
      });
    }

    // Auto-login if no verification required
    const session = await getSession();
    session.user = {
      id: user.id,
      username: user.username,
      email: user.email,
      displayName: user.displayName,
      groups: [],
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
    console.error("Signup error:", error);
    return NextResponse.json(
      { error: "An error occurred during registration" },
      { status: 500 }
    );
  }
}
