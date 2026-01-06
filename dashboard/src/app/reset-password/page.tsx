/**
 * Reset password page for built-in authentication.
 *
 * Allows users to set a new password using a reset token.
 */

import { redirect } from "next/navigation";
import { getAuthConfig } from "@/lib/auth/config";
import { getBuiltinConfig } from "@/lib/auth/builtin";
import { ResetPasswordForm } from "@/components/auth/reset-password-form";

interface ResetPasswordPageProps {
  searchParams: Promise<{
    token?: string;
    error?: string;
  }>;
}

export default async function ResetPasswordPage({
  searchParams,
}: ResetPasswordPageProps) {
  const config = getAuthConfig();
  const params = await searchParams;

  // Only available in builtin mode
  if (config.mode !== "builtin") {
    redirect("/");
  }

  // Token is required
  if (!params.token) {
    redirect("/forgot-password");
  }

  const builtinConfig = getBuiltinConfig();

  return (
    <div className="flex min-h-screen items-center justify-center bg-background p-4">
      <ResetPasswordForm
        token={params.token}
        error={params.error}
        minPasswordLength={builtinConfig.minPasswordLength}
      />
    </div>
  );
}
