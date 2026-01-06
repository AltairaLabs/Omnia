/**
 * Forgot password page for built-in authentication.
 *
 * Allows users to request a password reset email.
 */

import { redirect } from "next/navigation";
import { getAuthConfig } from "@/lib/auth/config";
import { ForgotPasswordForm } from "@/components/auth/forgot-password-form";

export default async function ForgotPasswordPage() {
  const config = getAuthConfig();

  // Only available in builtin mode
  if (config.mode !== "builtin") {
    redirect("/");
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-background p-4">
      <ForgotPasswordForm />
    </div>
  );
}
