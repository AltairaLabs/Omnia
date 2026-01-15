"use client";

import { useEffect, useState } from "react";

interface RuntimeConfig {
  demoMode: boolean;
  readOnlyMode: boolean;
  readOnlyMessage: string;
  wsProxyUrl: string;
  grafanaUrl: string;
  lokiEnabled: boolean;
  tempoEnabled: boolean;
}

// Use NEXT_PUBLIC_* as build-time defaults to avoid flash of wrong state
// The API route will provide the runtime values if different
const defaultConfig: RuntimeConfig = {
  demoMode: process.env.NEXT_PUBLIC_DEMO_MODE === "true",
  readOnlyMode: process.env.NEXT_PUBLIC_READ_ONLY_MODE === "true",
  readOnlyMessage: process.env.NEXT_PUBLIC_READ_ONLY_MESSAGE || "This dashboard is in read-only mode",
  wsProxyUrl: process.env.NEXT_PUBLIC_WS_PROXY_URL || "",
  grafanaUrl: process.env.NEXT_PUBLIC_GRAFANA_URL || "",
  lokiEnabled: process.env.NEXT_PUBLIC_LOKI_ENABLED === "true",
  tempoEnabled: process.env.NEXT_PUBLIC_TEMPO_ENABLED === "true",
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

/**
 * Get the Grafana base URL from runtime config.
 */
export function useGrafanaUrl() {
  const { config, loading } = useRuntimeConfig();
  return { grafanaUrl: config.grafanaUrl, loading };
}

/**
 * Check if Loki (logs) is enabled.
 */
export function useLokiEnabled() {
  const { config, loading } = useRuntimeConfig();
  return { lokiEnabled: config.lokiEnabled, loading };
}

/**
 * Check if Tempo (traces) is enabled.
 */
export function useTempoEnabled() {
  const { config, loading } = useRuntimeConfig();
  return { tempoEnabled: config.tempoEnabled, loading };
}

/**
 * Get observability features status (Loki and Tempo).
 */
export function useObservabilityConfig() {
  const { config, loading } = useRuntimeConfig();
  return {
    lokiEnabled: config.lokiEnabled,
    tempoEnabled: config.tempoEnabled,
    loading,
  };
}
