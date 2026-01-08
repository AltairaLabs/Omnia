"use client";

/**
 * Login form component for OAuth authentication.
 */

import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { AlertCircle, LogIn, Shield } from "lucide-react";

interface LoginFormProps {
  providerName: string;
  error?: string;
  errorMessage?: string;
  returnTo?: string;
}

/**
 * Error messages for OAuth errors.
 */
const ERROR_MESSAGES: Record<string, string> = {
  invalid_state: "Invalid authentication state. Please try again.",
  no_code: "No authorization code received from the identity provider.",
  callback_failed: "Authentication failed. Please try again.",
  access_denied: "Access was denied by the identity provider.",
  invalid_claims: "Could not retrieve user information from the identity provider.",
  config_error: "OAuth configuration error. Please contact your administrator.",
  consent_required: "Additional consent is required. Please try again.",
  login_required: "You need to log in to your identity provider.",
  interaction_required: "Interactive login required. Please try again.",
};

export function LoginForm({
  providerName,
  error,
  errorMessage,
  returnTo,
}: Readonly<LoginFormProps>) {
  const handleLogin = () => {
    // Redirect to OAuth login endpoint with returnTo
    const loginUrl = new URL("/api/auth/login", globalThis.location.origin);
    if (returnTo) {
      loginUrl.searchParams.set("returnTo", returnTo);
    }
    globalThis.location.href = loginUrl.href;
  };

  // Get human-readable error message
  const displayError = error
    ? errorMessage || ERROR_MESSAGES[error] || `Authentication error: ${error}`
    : null;

  return (
    <Card className="w-full max-w-md">
      <CardHeader className="text-center">
        <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-primary/10">
          <Shield className="h-6 w-6 text-primary" />
        </div>
        <CardTitle className="text-2xl">Sign in to Omnia</CardTitle>
        <CardDescription>
          Continue with {providerName} to access the dashboard
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {displayError && (
          <div className="flex items-start gap-2 rounded-lg bg-destructive/10 p-3 text-sm text-destructive">
            <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
            <span>{displayError}</span>
          </div>
        )}

        <Button onClick={handleLogin} className="w-full" size="lg">
          <LogIn className="mr-2 h-4 w-4" />
          Sign in with {providerName}
        </Button>

        <p className="text-center text-xs text-muted-foreground">
          You will be redirected to {providerName} to authenticate.
          <br />
          After signing in, you&apos;ll be returned to the dashboard.
        </p>
      </CardContent>
    </Card>
  );
}
