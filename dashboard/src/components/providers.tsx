"use client";

// Side-effect: configure Monaco to load from the self-hosted /monaco/vs path
// (the CSP blocks the default jsdelivr CDN). Must run before any Editor mounts.
import "@/lib/monaco-config";
import {
  QueryCache,
  MutationCache,
  QueryClient,
  QueryClientProvider,
} from "@tanstack/react-query";
import { ThemeProvider } from "next-themes";
import { useState } from "react";
import { DataServiceProvider } from "@/lib/data";
import { WorkspaceProvider } from "@/contexts/workspace-context";
import { DEFAULT_STALE_TIME } from "@/lib/query-config";
import { redirectToLogin } from "@/lib/auth/redirect-to-login";

function is401(error: unknown): boolean {
  return (
    error instanceof Error &&
    "status" in error &&
    (error as Error & { status: number }).status === 401
  );
}

export function Providers({ children }: Readonly<{ children: React.ReactNode }>) {
  const [queryClient] = useState(() => {
    function handle401(error: unknown) {
      if (is401(error)) {
        redirectToLogin();
      }
    }

    return new QueryClient({
      queryCache: new QueryCache({
        onError: handle401,
      }),
      mutationCache: new MutationCache({
        onError: handle401,
      }),
      defaultOptions: {
        queries: {
          staleTime: DEFAULT_STALE_TIME,
          refetchOnWindowFocus: false,
          gcTime: 5 * 60 * 1000, // 5 minutes — evict unused queries
        },
      },
    });
  });

  return (
    <QueryClientProvider client={queryClient}>
      <ThemeProvider
        attribute="class"
        defaultTheme="system"
        enableSystem
        disableTransitionOnChange
      >
        <WorkspaceProvider>
          <DataServiceProvider>
            {children}
          </DataServiceProvider>
        </WorkspaceProvider>
      </ThemeProvider>
    </QueryClientProvider>
  );
}
