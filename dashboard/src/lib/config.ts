/**
 * Runtime configuration fetching.
 *
 * Fetches configuration from the /api/config endpoint at runtime.
 * This is necessary for values that can't be baked in at build time
 * (e.g., when config comes from Kubernetes ConfigMaps).
 */

export interface RuntimeConfig {
  demoMode: boolean;
  readOnlyMode: boolean;
  readOnlyMessage: string;
  wsProxyUrl: string;
  grafanaUrl: string;
}

let cachedConfig: RuntimeConfig | null = null;
let configPromise: Promise<RuntimeConfig> | null = null;

/**
 * Fetch runtime configuration from the server.
 * Results are cached to avoid repeated fetches.
 */
export async function getRuntimeConfig(): Promise<RuntimeConfig> {
  // Return cached config if available
  if (cachedConfig) {
    return cachedConfig;
  }

  // If a fetch is already in progress, wait for it
  if (configPromise) {
    return configPromise;
  }

  // Fetch config from the API
  configPromise = fetch("/api/config")
    .then((res) => {
      if (!res.ok) {
        throw new Error(`Failed to fetch config: ${res.status}`);
      }
      return res.json();
    })
    .then((config: RuntimeConfig) => {
      cachedConfig = config;
      return config;
    })
    .catch((err) => {
      console.error("Failed to fetch runtime config:", err);
      // Return defaults on error
      return {
        demoMode: false,
        readOnlyMode: false,
        readOnlyMessage: "This dashboard is in read-only mode",
        wsProxyUrl: "",
        grafanaUrl: "",
      };
    })
    .finally(() => {
      configPromise = null;
    });

  return configPromise;
}

/**
 * Get the WebSocket proxy URL from runtime config.
 * Returns empty string if not configured (use default behavior).
 */
export async function getWsProxyUrl(): Promise<string> {
  const config = await getRuntimeConfig();
  return config.wsProxyUrl;
}
