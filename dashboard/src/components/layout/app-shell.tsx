"use client";

import { usePathname } from "next/navigation";
import { type ReactNode } from "react";
import { Sidebar } from "./sidebar";
import { ReadOnlyBanner } from "./read-only-banner";
import { DemoModeBanner } from "./demo-mode-banner";
import { AnonymousModeBanner } from "./anonymous-mode-banner";
import { LicenseExpiryBanner } from "./license-expiry-banner";
import { DevModeLicenseBanner } from "./dev-mode-license-banner";
import { WorkspaceContent } from "./workspace-content";
import { ErrorBoundary } from "@/components/error-boundary";
import { isChromelessPath } from "@/lib/routes";

interface AppShellProps {
  children: ReactNode;
}

export function AppShell({ children }: Readonly<AppShellProps>) {
  const pathname = usePathname();

  if (isChromelessPath(pathname)) {
    return <ErrorBoundary>{children}</ErrorBoundary>;
  }

  return (
    <div className="flex h-screen">
      <Sidebar />
      <div className="flex-1 flex flex-col overflow-hidden">
        <AnonymousModeBanner />
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
