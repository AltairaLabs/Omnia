"use client";

import Link from "next/link";
import { Button } from "@/components/ui/button";
import { BrandedBoundary } from "@/components/branding/branded-boundary";

/** Branded 404 page — replaces the unbranded Next.js default. */
export default function NotFound() {
  return (
    <BrandedBoundary
      code="404"
      title="Page not found"
      description="The page you're looking for doesn't exist or has moved."
      action={
        <Button asChild>
          <Link href="/">Back to dashboard</Link>
        </Button>
      }
    />
  );
}
