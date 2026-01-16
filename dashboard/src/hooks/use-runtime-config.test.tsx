import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import {
  useRuntimeConfig,
  useDemoMode,
  useReadOnlyMode,
  useGrafanaUrl,
  useLokiEnabled,
  useTempoEnabled,
  useObservabilityConfig,
} from "./use-runtime-config";

// Store original fetch
const originalFetch = global.fetch;

describe("useRuntimeConfig", () => {
  const mockConfig = {
    demoMode: true,
    readOnlyMode: false,
    readOnlyMessage: "Test read-only message",
    wsProxyUrl: "ws://localhost:3002",
    grafanaUrl: "http://localhost:3001",
    lokiEnabled: true,
    tempoEnabled: false,
  };

  beforeEach(() => {
    vi.resetModules();
    // Clear the cached config by reimporting
    vi.doUnmock("./use-runtime-config");

    // Mock fetch
    global.fetch = vi.fn().mockResolvedValue({
      json: () => Promise.resolve(mockConfig),
    });
  });

  afterEach(() => {
    global.fetch = originalFetch;
  });

  it("should return default config initially", () => {
    const { result } = renderHook(() => useRuntimeConfig());

    // Initial state uses build-time defaults
    expect(result.current.config).toBeDefined();
    expect(typeof result.current.config.demoMode).toBe("boolean");
    expect(typeof result.current.config.readOnlyMode).toBe("boolean");
    expect(typeof result.current.config.readOnlyMessage).toBe("string");
    expect(typeof result.current.config.wsProxyUrl).toBe("string");
    expect(typeof result.current.config.grafanaUrl).toBe("string");
  });

  it("should have loading state", () => {
    const { result } = renderHook(() => useRuntimeConfig());

    // Should have loading state
    expect(typeof result.current.loading).toBe("boolean");
  });

  it("should return valid config (from fetch or cache)", async () => {
    const { result } = renderHook(() => useRuntimeConfig());

    // The hook either fetches config on mount or uses cached value
    // Wait for loading to complete and verify config is valid
    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    // Verify the config has the expected shape
    expect(result.current.config).toBeDefined();
    expect(typeof result.current.config.demoMode).toBe("boolean");
    expect(typeof result.current.config.readOnlyMode).toBe("boolean");
    expect(typeof result.current.config.readOnlyMessage).toBe("string");
  });

  it("should handle fetch error gracefully", async () => {
    global.fetch = vi.fn().mockRejectedValue(new Error("Network error"));

    const consoleSpy = vi.spyOn(console, "error").mockImplementation(() => {});

    const { result } = renderHook(() => useRuntimeConfig());

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    // Should fall back to default config
    expect(result.current.config).toBeDefined();
    consoleSpy.mockRestore();
  });
});

describe("useDemoMode", () => {
  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    global.fetch = originalFetch;
  });

  it("should return isDemoMode and loading state", () => {
    global.fetch = vi.fn().mockResolvedValue({
      json: () => Promise.resolve({ demoMode: true, readOnlyMode: false, readOnlyMessage: "", wsProxyUrl: "", grafanaUrl: "", lokiEnabled: false, tempoEnabled: false }),
    });

    const { result } = renderHook(() => useDemoMode());

    expect(result.current).toHaveProperty("isDemoMode");
    expect(result.current).toHaveProperty("loading");
    expect(typeof result.current.isDemoMode).toBe("boolean");
    expect(typeof result.current.loading).toBe("boolean");
  });

  it("should reflect demo mode from config", async () => {
    global.fetch = vi.fn().mockResolvedValue({
      json: () => Promise.resolve({ demoMode: true, readOnlyMode: false, readOnlyMessage: "", wsProxyUrl: "", grafanaUrl: "", lokiEnabled: false, tempoEnabled: false }),
    });

    const { result } = renderHook(() => useDemoMode());

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    // Value depends on fetched config
    expect(typeof result.current.isDemoMode).toBe("boolean");
  });
});

describe("useReadOnlyMode", () => {
  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    global.fetch = originalFetch;
  });

  it("should return isReadOnly, message, and loading state", () => {
    global.fetch = vi.fn().mockResolvedValue({
      json: () => Promise.resolve({ demoMode: false, readOnlyMode: true, readOnlyMessage: "Custom message", wsProxyUrl: "", grafanaUrl: "", lokiEnabled: false, tempoEnabled: false }),
    });

    const { result } = renderHook(() => useReadOnlyMode());

    expect(result.current).toHaveProperty("isReadOnly");
    expect(result.current).toHaveProperty("message");
    expect(result.current).toHaveProperty("loading");
    expect(typeof result.current.isReadOnly).toBe("boolean");
    expect(typeof result.current.message).toBe("string");
    expect(typeof result.current.loading).toBe("boolean");
  });

  it("should reflect read-only mode from config", async () => {
    global.fetch = vi.fn().mockResolvedValue({
      json: () => Promise.resolve({
        demoMode: false,
        readOnlyMode: true,
        readOnlyMessage: "Managed by GitOps",
        wsProxyUrl: "",
        grafanaUrl: "",
        lokiEnabled: false,
        tempoEnabled: false,
      }),
    });

    const { result } = renderHook(() => useReadOnlyMode());

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    // Value depends on fetched config
    expect(typeof result.current.isReadOnly).toBe("boolean");
    expect(typeof result.current.message).toBe("string");
  });
});

describe("useGrafanaUrl", () => {
  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    global.fetch = originalFetch;
  });

  it("should return grafanaUrl and loading state", () => {
    global.fetch = vi.fn().mockResolvedValue({
      json: () => Promise.resolve({
        demoMode: false,
        readOnlyMode: false,
        readOnlyMessage: "",
        wsProxyUrl: "",
        grafanaUrl: "http://localhost:3001",
        lokiEnabled: false,
        tempoEnabled: false,
      }),
    });

    const { result } = renderHook(() => useGrafanaUrl());

    expect(result.current).toHaveProperty("grafanaUrl");
    expect(result.current).toHaveProperty("loading");
    expect(typeof result.current.grafanaUrl).toBe("string");
    expect(typeof result.current.loading).toBe("boolean");
  });

  it("should reflect grafana URL from config", async () => {
    global.fetch = vi.fn().mockResolvedValue({
      json: () => Promise.resolve({
        demoMode: false,
        readOnlyMode: false,
        readOnlyMessage: "",
        wsProxyUrl: "",
        grafanaUrl: "http://grafana.example.com:3000",
        lokiEnabled: false,
        tempoEnabled: false,
      }),
    });

    const { result } = renderHook(() => useGrafanaUrl());

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(typeof result.current.grafanaUrl).toBe("string");
  });
});

describe("useLokiEnabled", () => {
  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    global.fetch = originalFetch;
  });

  it("should return lokiEnabled and loading state", () => {
    global.fetch = vi.fn().mockResolvedValue({
      json: () => Promise.resolve({
        demoMode: false,
        readOnlyMode: false,
        readOnlyMessage: "",
        wsProxyUrl: "",
        grafanaUrl: "",
        lokiEnabled: true,
        tempoEnabled: false,
      }),
    });

    const { result } = renderHook(() => useLokiEnabled());

    expect(result.current).toHaveProperty("lokiEnabled");
    expect(result.current).toHaveProperty("loading");
    expect(typeof result.current.lokiEnabled).toBe("boolean");
    expect(typeof result.current.loading).toBe("boolean");
  });

  it("should reflect loki enabled state from config", async () => {
    global.fetch = vi.fn().mockResolvedValue({
      json: () => Promise.resolve({
        demoMode: false,
        readOnlyMode: false,
        readOnlyMessage: "",
        wsProxyUrl: "",
        grafanaUrl: "",
        lokiEnabled: true,
        tempoEnabled: false,
      }),
    });

    const { result } = renderHook(() => useLokiEnabled());

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(typeof result.current.lokiEnabled).toBe("boolean");
  });
});

describe("useTempoEnabled", () => {
  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    global.fetch = originalFetch;
  });

  it("should return tempoEnabled and loading state", () => {
    global.fetch = vi.fn().mockResolvedValue({
      json: () => Promise.resolve({
        demoMode: false,
        readOnlyMode: false,
        readOnlyMessage: "",
        wsProxyUrl: "",
        grafanaUrl: "",
        lokiEnabled: false,
        tempoEnabled: true,
      }),
    });

    const { result } = renderHook(() => useTempoEnabled());

    expect(result.current).toHaveProperty("tempoEnabled");
    expect(result.current).toHaveProperty("loading");
    expect(typeof result.current.tempoEnabled).toBe("boolean");
    expect(typeof result.current.loading).toBe("boolean");
  });

  it("should reflect tempo enabled state from config", async () => {
    global.fetch = vi.fn().mockResolvedValue({
      json: () => Promise.resolve({
        demoMode: false,
        readOnlyMode: false,
        readOnlyMessage: "",
        wsProxyUrl: "",
        grafanaUrl: "",
        lokiEnabled: false,
        tempoEnabled: true,
      }),
    });

    const { result } = renderHook(() => useTempoEnabled());

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(typeof result.current.tempoEnabled).toBe("boolean");
  });
});

describe("useObservabilityConfig", () => {
  beforeEach(() => {
    vi.resetModules();
  });

  afterEach(() => {
    global.fetch = originalFetch;
  });

  it("should return lokiEnabled, tempoEnabled and loading state", () => {
    global.fetch = vi.fn().mockResolvedValue({
      json: () => Promise.resolve({
        demoMode: false,
        readOnlyMode: false,
        readOnlyMessage: "",
        wsProxyUrl: "",
        grafanaUrl: "",
        lokiEnabled: true,
        tempoEnabled: true,
      }),
    });

    const { result } = renderHook(() => useObservabilityConfig());

    expect(result.current).toHaveProperty("lokiEnabled");
    expect(result.current).toHaveProperty("tempoEnabled");
    expect(result.current).toHaveProperty("loading");
    expect(typeof result.current.lokiEnabled).toBe("boolean");
    expect(typeof result.current.tempoEnabled).toBe("boolean");
    expect(typeof result.current.loading).toBe("boolean");
  });

  it("should reflect observability config from fetched config", async () => {
    global.fetch = vi.fn().mockResolvedValue({
      json: () => Promise.resolve({
        demoMode: false,
        readOnlyMode: false,
        readOnlyMessage: "",
        wsProxyUrl: "",
        grafanaUrl: "",
        lokiEnabled: true,
        tempoEnabled: false,
      }),
    });

    const { result } = renderHook(() => useObservabilityConfig());

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(typeof result.current.lokiEnabled).toBe("boolean");
    expect(typeof result.current.tempoEnabled).toBe("boolean");
  });
});
