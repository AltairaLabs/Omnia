"use client";

/**
 * Signup form component for built-in authentication.
 */

import { useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { AlertCircle, CheckCircle2, Loader2, UserPlus } from "lucide-react";

const BORDER_DESTRUCTIVE = "border-destructive";

interface SignupFormProps {
  error?: string;
  errorMessage?: string;
  returnTo?: string;
  minPasswordLength?: number;
}

export function SignupForm({
  error: initialError,
  errorMessage: initialMessage,
  returnTo,
  minPasswordLength = 8,
}: SignupFormProps) {
  const router = useRouter();
  const [username, setUsername] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [error, setError] = useState(initialMessage || initialError || "");
  const [fieldError, setFieldError] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [success, setSuccess] = useState(false);
  const [requiresVerification, setRequiresVerification] = useState(false);

  const validateForm = (): boolean => {
    if (password !== confirmPassword) {
      setError("Passwords do not match");
      setFieldError("confirmPassword");
      return false;
    }
    if (password.length < minPasswordLength) {
      setError(`Password must be at least ${minPasswordLength} characters`);
      setFieldError("password");
      return false;
    }
    return true;
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    setFieldError(null);

    if (!validateForm()) {
      return;
    }

    setIsLoading(true);

    try {
      const response = await fetch("/api/auth/builtin/signup", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ username, email, password, displayName }),
      });

      const data = await response.json();

      if (!response.ok) {
        setError(data.error || "Registration failed");
        if (data.field) {
          setFieldError(data.field);
        }
        setIsLoading(false);
        return;
      }

      // Check if email verification is required
      if (data.requiresVerification) {
        setRequiresVerification(true);
        setSuccess(true);
        setIsLoading(false);
        return;
      }

      // Auto-login successful, redirect
      setSuccess(true);
      router.push(returnTo || "/");
      router.refresh();
    } catch {
      setError("An error occurred. Please try again.");
      setIsLoading(false);
    }
  };

  // Success state with verification required
  if (success && requiresVerification) {
    return (
      <Card className="w-full max-w-md">
        <CardHeader className="text-center">
          <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-green-100">
            <CheckCircle2 className="h-6 w-6 text-green-600" />
          </div>
          <CardTitle className="text-2xl">Check your email</CardTitle>
          <CardDescription>
            We&apos;ve sent a verification link to <strong>{email}</strong>
          </CardDescription>
        </CardHeader>
        <CardContent className="text-center text-sm text-muted-foreground">
          <p>Click the link in the email to verify your account.</p>
          <p className="mt-2">
            Didn&apos;t receive the email? Check your spam folder or{" "}
            <button
              className="text-primary hover:underline"
              onClick={() => {
                setSuccess(false);
                setRequiresVerification(false);
              }}
            >
              try again
            </button>
          </p>
        </CardContent>
        <CardFooter>
          <Link href="/login" className="w-full">
            <Button variant="outline" className="w-full">
              Back to login
            </Button>
          </Link>
        </CardFooter>
      </Card>
    );
  }

  return (
    <Card className="w-full max-w-md">
      <CardHeader className="text-center">
        <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-primary/10">
          <UserPlus className="h-6 w-6 text-primary" />
        </div>
        <CardTitle className="text-2xl">Create an account</CardTitle>
        <CardDescription>
          Sign up to get started with Omnia
        </CardDescription>
      </CardHeader>
      <form onSubmit={handleSubmit}>
        <CardContent className="space-y-4">
          {error && (
            <div className="flex items-start gap-2 rounded-lg bg-destructive/10 p-3 text-sm text-destructive">
              <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
              <span>{error}</span>
            </div>
          )}

          <div className="space-y-2">
            <Label htmlFor="username">Username</Label>
            <Input
              id="username"
              type="text"
              placeholder="johndoe"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              required
              disabled={isLoading}
              autoComplete="username"
              autoFocus
              className={fieldError === "username" ? BORDER_DESTRUCTIVE : ""}
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="email">Email</Label>
            <Input
              id="email"
              type="email"
              placeholder="john@example.com"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              required
              disabled={isLoading}
              autoComplete="email"
              className={fieldError === "email" ? BORDER_DESTRUCTIVE : ""}
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="displayName">Display Name (optional)</Label>
            <Input
              id="displayName"
              type="text"
              placeholder="John Doe"
              value={displayName}
              onChange={(e) => setDisplayName(e.target.value)}
              disabled={isLoading}
              autoComplete="name"
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="password">Password</Label>
            <Input
              id="password"
              type="password"
              placeholder="Enter a strong password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              required
              disabled={isLoading}
              autoComplete="new-password"
              minLength={minPasswordLength}
              className={fieldError === "password" ? BORDER_DESTRUCTIVE : ""}
            />
            <p className="text-xs text-muted-foreground">
              Must be at least {minPasswordLength} characters
            </p>
          </div>

          <div className="space-y-2">
            <Label htmlFor="confirmPassword">Confirm Password</Label>
            <Input
              id="confirmPassword"
              type="password"
              placeholder="Confirm your password"
              value={confirmPassword}
              onChange={(e) => setConfirmPassword(e.target.value)}
              required
              disabled={isLoading}
              autoComplete="new-password"
              className={fieldError === "confirmPassword" ? BORDER_DESTRUCTIVE : ""}
            />
          </div>
        </CardContent>
        <CardFooter className="flex flex-col space-y-4">
          <Button type="submit" className="w-full" size="lg" disabled={isLoading}>
            {isLoading ? (
              <>
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                Creating account...
              </>
            ) : (
              <>
                <UserPlus className="mr-2 h-4 w-4" />
                Create account
              </>
            )}
          </Button>

          <p className="text-center text-sm text-muted-foreground">
            Already have an account?{" "}
            <Link href="/login" className="text-primary hover:underline">
              Sign in
            </Link>
          </p>
        </CardFooter>
      </form>
    </Card>
  );
}
