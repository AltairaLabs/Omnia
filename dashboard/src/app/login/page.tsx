/**
 * Login page for OAuth authentication.
 *
 * Displayed when user needs to authenticate via OAuth provider.
 */

import { redirect } from "next/navigation";
import { getAuthConfig } from "@/lib/auth/config";
import { getCurrentUser } from "@/lib/auth/session";
import { getProviderDisplayName } from "@/lib/auth/oauth";
import { LoginForm } from "@/components/auth/login-form";

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

  // If not in OAuth mode, redirect to home
  if (config.mode !== "oauth") {
    redirect("/");
  }

  // If already authenticated, redirect to returnTo or home
  const user = await getCurrentUser();
  if (user && user.provider === "oauth") {
    redirect(params.returnTo || "/");
  }

  const providerName = getProviderDisplayName(config.oauth.provider);

  return (
    <div className="flex min-h-screen items-center justify-center bg-background p-4">
      <LoginForm
        providerName={providerName}
        error={params.error}
        errorMessage={params.message}
        returnTo={params.returnTo}
      />
    </div>
  );
}
