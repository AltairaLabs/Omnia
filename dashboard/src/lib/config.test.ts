import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

describe("config", () => {
  const mockFetch = vi.fn();
  const originalFetch = globalThis.fetch;

  beforeEach(() => {
    globalThis.fetch = mockFetch;
    vi.resetModules();
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
    mockFetch.mockReset();
  });

  describe("getRuntimeConfig", () => {
    it("fetches and caches config from API", async () => {
      const mockConfig = {
        demoMode: true,
        readOnlyMode: false,
        readOnlyMessage: "Test message",
        wsProxyUrl: "ws://localhost:3002",
        grafanaUrl: "http://localhost:3001",
      };

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockConfig),
      });

      // Re-import to get fresh module with no cache
      const { getRuntimeConfig: getConfig } = await import("./config");
      const config = await getConfig();

      expect(config).toEqual(mockConfig);
      expect(mockFetch).toHaveBeenCalledWith("/api/config");
    });

    it("returns default config on fetch error", async () => {
      mockFetch.mockRejectedValueOnce(new Error("Network error"));

      // Re-import to get fresh module with no cache
      const { getRuntimeConfig: getConfig } = await import("./config");
      const config = await getConfig();

      expect(config).toEqual({
        demoMode: false,
        readOnlyMode: false,
        readOnlyMessage: "This dashboard is in read-only mode",
        wsProxyUrl: "",
        grafanaUrl: "",
        lokiEnabled: false,
        tempoEnabled: false,
      });
    });

    it("returns default config on non-ok response", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
      });

      // Re-import to get fresh module with no cache
      const { getRuntimeConfig: getConfig } = await import("./config");
      const config = await getConfig();

      expect(config).toEqual({
        demoMode: false,
        readOnlyMode: false,
        readOnlyMessage: "This dashboard is in read-only mode",
        wsProxyUrl: "",
        grafanaUrl: "",
        lokiEnabled: false,
        tempoEnabled: false,
      });
    });

    it("returns cached config on subsequent calls", async () => {
      const mockConfig = {
        demoMode: true,
        readOnlyMode: false,
        readOnlyMessage: "Test message",
        wsProxyUrl: "ws://localhost:3002",
        grafanaUrl: "http://localhost:3001",
      };

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockConfig),
      });

      // Re-import to get fresh module with no cache
      const { getRuntimeConfig: getConfig } = await import("./config");

      // First call - fetches from API
      const config1 = await getConfig();
      expect(config1).toEqual(mockConfig);
      expect(mockFetch).toHaveBeenCalledTimes(1);

      // Second call - returns cached value
      const config2 = await getConfig();
      expect(config2).toEqual(mockConfig);
      expect(mockFetch).toHaveBeenCalledTimes(1); // Still only 1 call
    });

    it("deduplicates concurrent requests", async () => {
      const mockConfig = {
        demoMode: true,
        readOnlyMode: false,
        readOnlyMessage: "Test",
        wsProxyUrl: "",
        grafanaUrl: "",
      };

      // Use a delayed response to ensure concurrent calls
      mockFetch.mockImplementationOnce(() =>
        new Promise(resolve =>
          setTimeout(() => resolve({
            ok: true,
            json: () => Promise.resolve(mockConfig),
          }), 10)
        )
      );

      // Re-import to get fresh module with no cache
      const { getRuntimeConfig: getConfig } = await import("./config");

      // Make concurrent calls
      const [config1, config2] = await Promise.all([
        getConfig(),
        getConfig(),
      ]);

      expect(config1).toEqual(mockConfig);
      expect(config2).toEqual(mockConfig);
      expect(mockFetch).toHaveBeenCalledTimes(1); // Only 1 fetch despite 2 calls
    });
  });

  describe("getWsProxyUrl", () => {
    it("returns wsProxyUrl from config", async () => {
      const mockConfig = {
        demoMode: false,
        readOnlyMode: false,
        readOnlyMessage: "",
        wsProxyUrl: "ws://test:8080",
        grafanaUrl: "",
      };

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockConfig),
      });

      // Re-import to get fresh module with no cache
      const { getWsProxyUrl: getUrl } = await import("./config");
      const url = await getUrl();

      expect(url).toBe("ws://test:8080");
    });
  });
});
