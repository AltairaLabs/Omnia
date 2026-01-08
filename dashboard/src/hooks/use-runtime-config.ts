"use client";

import { useEffect, useState } from "react";

interface RuntimeConfig {
  demoMode: boolean;
  readOnlyMode: boolean;
  readOnlyMessage: string;
}

// Use NEXT_PUBLIC_DEMO_MODE as build-time default to avoid flash of wrong service
// The API route will provide the runtime value if different
const defaultConfig: RuntimeConfig = {
  demoMode: process.env.NEXT_PUBLIC_DEMO_MODE === "true",
  readOnlyMode: process.env.NEXT_PUBLIC_READ_ONLY_MODE === "true",
  readOnlyMessage: process.env.NEXT_PUBLIC_READ_ONLY_MESSAGE || "This dashboard is in read-only mode",
};

let cachedConfig: RuntimeConfig | null = null;

/**
 * Hook to fetch runtime configuration from the server.
 * This allows config values to be set via environment variables
 * at runtime (e.g., from Kubernetes ConfigMaps).
 */
export function useRuntimeConfig() {
  const [config, setConfig] = useState<RuntimeConfig>(cachedConfig || defaultConfig);
  const [loading, setLoading] = useState(!cachedConfig);

  useEffect(() => {
    if (cachedConfig) {
      return;
    }

    fetch("/api/config")
      .then((res) => res.json())
      .then((data: RuntimeConfig) => {
        cachedConfig = data;
        setConfig(data);
        setLoading(false);
      })
      .catch((err) => {
        console.error("Failed to fetch runtime config:", err);
        setLoading(false);
      });
  }, []);

  return { config, loading };
}

/**
 * Check if demo mode is enabled.
 */
export function useDemoMode() {
  const { config, loading } = useRuntimeConfig();
  return { isDemoMode: config.demoMode, loading };
}

/**
 * Check if read-only mode is enabled.
 */
export function useReadOnlyMode() {
  const { config, loading } = useRuntimeConfig();
  return {
    isReadOnly: config.readOnlyMode,
    message: config.readOnlyMessage,
    loading,
  };
}
