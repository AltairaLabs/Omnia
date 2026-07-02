"use client";

import "./globals.css";
import { useEffect } from "react";
import { Button } from "@/components/ui/button";
import { BrandProvider } from "@/components/branding/brand-provider";
import { BrandedBoundary } from "@/components/branding/branded-boundary";

/**
 * Branded catastrophic-error boundary. Next.js replaces the root layout with
 * this component when the layout itself throws, so it renders its own
 * <html>/<body> and its own BrandProvider to stay white-labeled.
 */
export default function GlobalError({
  error,
  reset,
}: Readonly<{ error: Error & { digest?: string }; reset: () => void }>) {
  useEffect(() => {
    console.error(error);
  }, [error]);

  return (
    <html lang="en">
      <body>
        <BrandProvider>
          <BrandedBoundary
            code="500"
            title="Something went wrong"
            description="A critical error occurred. Try again, or return to the dashboard."
            action={<Button onClick={reset}>Try again</Button>}
          />
        </BrandProvider>
      </body>
    </html>
  );
}
