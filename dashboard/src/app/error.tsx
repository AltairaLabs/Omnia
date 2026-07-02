"use client";

import { useEffect } from "react";
import { Button } from "@/components/ui/button";
import { BrandedBoundary } from "@/components/branding/branded-boundary";

/** Branded route-level error boundary — replaces the unbranded Next.js default. */
export default function RouteError({
  error,
  reset,
}: Readonly<{ error: Error & { digest?: string }; reset: () => void }>) {
  useEffect(() => {
    console.error(error);
  }, [error]);

  return (
    <BrandedBoundary
      code="500"
      title="Something went wrong"
      description="An unexpected error occurred. Try again, or return to the dashboard."
      action={
        <Button onClick={reset}>Try again</Button>
      }
    />
  );
}
