/**
 * Page-level error boundary with navigation-aware fallback UI.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

"use client";

import React from "react";
import Link from "next/link";
import { ErrorBoundary } from "@/components/error-boundary";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";

function PageErrorFallback() {
  return (
    <div className="flex flex-col flex-1">
      <div className="flex items-center justify-center flex-1 p-8">
        <Card className="w-full max-w-md">
          <CardHeader>
            <CardTitle>Page error</CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            <p className="text-sm text-muted-foreground">
              This page encountered an unexpected error. You can try going back
              or returning to the dashboard.
            </p>
            <div className="flex gap-2">
              <Button variant="outline" asChild>
                <Link href="/">Go to dashboard</Link>
              </Button>
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}

interface PageErrorBoundaryProps {
  children: React.ReactNode;
}

export function PageErrorBoundary({ children }: Readonly<PageErrorBoundaryProps>) {
  return (
    <ErrorBoundary fallback={<PageErrorFallback />}>
      {children}
    </ErrorBoundary>
  );
}
