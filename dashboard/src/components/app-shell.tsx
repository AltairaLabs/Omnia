"use client";

import { usePathname } from "next/navigation";
import {
  Sidebar,
  ReadOnlyBanner,
  DemoModeBanner,
  LicenseExpiryBanner,
  DevModeLicenseBanner,
  WorkspaceContent,
} from "@/components/layout";
import { ErrorBoundary } from "@/components/error-boundary";

// Paths that render without the authenticated app chrome (sidebar, banners).
// Must match the middleware PUBLIC_ROUTES user-facing entries.
const AUTH_PATHS = [
  "/login",
  "/signup",
  "/forgot-password",
  "/reset-password",
  "/verify-email",
];

function isAuthPath(pathname: string): boolean {
  return AUTH_PATHS.some((p) => pathname === p || pathname.startsWith(`${p}/`));
}

export function AppShell({ children }: Readonly<{ children: React.ReactNode }>) {
  const pathname = usePathname();

  if (isAuthPath(pathname)) {
    return (
      <main className="min-h-screen bg-background">
        <ErrorBoundary>{children}</ErrorBoundary>
      </main>
    );
  }

  return (
    <div className="flex h-screen">
      <Sidebar />
      <div className="flex-1 flex flex-col overflow-hidden">
        <DemoModeBanner />
        <ReadOnlyBanner />
        <LicenseExpiryBanner />
        <DevModeLicenseBanner />
        <main className="flex-1 overflow-auto bg-background">
          <ErrorBoundary>
            <WorkspaceContent>{children}</WorkspaceContent>
          </ErrorBoundary>
        </main>
      </div>
    </div>
  );
}
