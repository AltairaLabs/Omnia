/**
 * Login page for authentication.
 *
 * Displays the appropriate login form based on auth mode:
 * - OAuth: Shows provider login button
 * - Builtin: Shows username/password form
 */

import { redirect } from "next/navigation";
import { getAuthConfig } from "@/lib/auth/config";
import { getCurrentUser } from "@/lib/auth/session";
import { getProviderDisplayName } from "@/lib/auth/oauth";
import { LoginForm as OAuthLoginForm } from "@/components/auth/login-form";
import { BuiltinLoginForm } from "@/components/auth/builtin-login-form";

interface LoginPageProps {
  searchParams: Promise<{
    error?: string;
    message?: string;
    returnTo?: string;
  }>;
}

export default async function LoginPage({ searchParams }: LoginPageProps) {
  const config = getAuthConfig();
  const params = await searchParams;

  // If not in OAuth or builtin mode, redirect to home
  if (config.mode !== "oauth" && config.mode !== "builtin") {
    redirect("/");
  }

  // If already authenticated, redirect to returnTo or home
  const user = await getCurrentUser();
  if (user && (user.provider === "oauth" || user.provider === "builtin")) {
    redirect(params.returnTo || "/");
  }

  // OAuth mode
  if (config.mode === "oauth") {
    const providerName = getProviderDisplayName(config.oauth.provider);

    return (
      <div className="flex min-h-screen items-center justify-center bg-background p-4">
        <OAuthLoginForm
          providerName={providerName}
          error={params.error}
          errorMessage={params.message}
          returnTo={params.returnTo}
        />
      </div>
    );
  }

  // Builtin mode
  return (
    <div className="flex min-h-screen items-center justify-center bg-background p-4">
      <BuiltinLoginForm
        error={params.error}
        errorMessage={params.message}
        returnTo={params.returnTo}
      />
    </div>
  );
}
