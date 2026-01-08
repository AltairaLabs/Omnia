/**
 * Signup page for built-in authentication.
 *
 * Allows new users to create an account when signup is enabled.
 */

import { redirect } from "next/navigation";
import { getAuthConfig } from "@/lib/auth/config";
import { getCurrentUser } from "@/lib/auth/session";
import { getBuiltinConfig } from "@/lib/auth/builtin";
import { SignupForm } from "@/components/auth/signup-form";

interface SignupPageProps {
  searchParams: Promise<{
    error?: string;
    message?: string;
    returnTo?: string;
  }>;
}

export default async function SignupPage({ searchParams }: Readonly<SignupPageProps>) {
  const config = getAuthConfig();
  const params = await searchParams;

  // Only available in builtin mode
  if (config.mode !== "builtin") {
    redirect("/");
  }

  // Check if signup is allowed
  const builtinConfig = getBuiltinConfig();
  if (!builtinConfig.allowSignup) {
    redirect("/login");
  }

  // If already authenticated, redirect to returnTo or home
  const user = await getCurrentUser();
  if (user?.provider === "builtin") {
    redirect(params.returnTo || "/");
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-background p-4">
      <SignupForm
        error={params.error}
        errorMessage={params.message}
        returnTo={params.returnTo}
        minPasswordLength={builtinConfig.minPasswordLength}
      />
    </div>
  );
}
