"use client";

import { useEffect, useState } from "react";
import { getRuntimeConfig, type RuntimeConfig } from "@/lib/config";

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
  enterpriseEnabled: process.env.NEXT_PUBLIC_ENTERPRISE_ENABLED === "true",
  hideEnterprise: process.env.NEXT_PUBLIC_HIDE_ENTERPRISE === "true",
};

// Track if config has been fetched to avoid duplicate requests
let configFetched = false;
let cachedConfig: RuntimeConfig | null = null;

/**
 * Hook to fetch runtime configuration from the server.
 * This allows config values to be set via environment variables
 * at runtime (e.g., from Kubernetes ConfigMaps).
 *
 * Uses the centralized getRuntimeConfig() which has its own deduplication.
 */
export function useRuntimeConfig() {
  // Initialize with cached config if available, avoiding unnecessary fetches
  const [config, setConfig] = useState<RuntimeConfig>(cachedConfig || defaultConfig);
  const [loading, setLoading] = useState(!configFetched);

  useEffect(() => {
    // Skip fetch if config was already cached (state initialized correctly above)
    if (configFetched) {
      return;
    }

    // Use centralized getRuntimeConfig which deduplicates concurrent requests
    getRuntimeConfig()
      .then((data) => {
        cachedConfig = data;
        configFetched = true;
        setConfig(data);
        setLoading(false);
      })
      .catch((err) => {
        console.error("Failed to fetch runtime config:", err);
        configFetched = true;
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

/**
 * Get enterprise features configuration.
 *
 * Returns:
 * - enterpriseEnabled: Whether enterprise features are deployed (infrastructure level)
 * - hideEnterprise: Whether to hide enterprise features completely
 * - showUpgradePrompts: Whether to show upgrade prompts for unavailable features
 *   (true when enterprise is not enabled but hideEnterprise is false)
 */
export function useEnterpriseConfig() {
  const { config, loading } = useRuntimeConfig();
  return {
    /** Whether enterprise CRDs and controllers are deployed */
    enterpriseEnabled: config.enterpriseEnabled,
    /** Whether to hide enterprise features completely in the UI */
    hideEnterprise: config.hideEnterprise,
    /** Whether to show upgrade prompts (enterprise not enabled, but not hidden) */
    showUpgradePrompts: !config.enterpriseEnabled && !config.hideEnterprise,
    loading,
  };
}
