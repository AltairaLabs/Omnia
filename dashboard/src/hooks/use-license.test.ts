import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React from "react";
import { useLicense } from "./use-license";
import { OPEN_CORE_LICENSE, type License } from "@/types/license";

// Mock fetch
const mockFetch = vi.fn();
global.fetch = mockFetch;

// Wrapper to provide React Query context and clear cache between tests
function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  function Wrapper({ children }: { children: React.ReactNode }) {
    return React.createElement(QueryClientProvider, { client: queryClient }, children);
  }
  return Wrapper;
}

describe("useLicense", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockFetch.mockReset();
  });

  it("should return open-core license as placeholder data initially", () => {
    mockFetch.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(OPEN_CORE_LICENSE),
    });

    const { result } = renderHook(() => useLicense(), { wrapper: createWrapper() });

    // Placeholder data should be available immediately
    expect(result.current.license).toEqual(OPEN_CORE_LICENSE);
    expect(result.current.isEnterprise).toBe(false);
  });

  it("should have placeholder data while fetching", async () => {
    let resolvePromise: (value: Response) => void;
    const fetchPromise = new Promise<Response>((resolve) => {
      resolvePromise = resolve;
    });
    mockFetch.mockReturnValue(fetchPromise);

    const { result } = renderHook(() => useLicense(), { wrapper: createWrapper() });

    // With placeholderData, isLoading is false but data is the placeholder
    // This is correct React Query behavior: placeholderData prevents loading state
    expect(result.current.license).toEqual(OPEN_CORE_LICENSE);

    // Resolve the fetch
    resolvePromise!({
      ok: true,
      json: () => Promise.resolve(OPEN_CORE_LICENSE),
    } as Response);

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });
  });

  it("should return fetched license data", async () => {
    const enterpriseLicense: License = {
      id: "enterprise-123",
      tier: "enterprise",
      customer: "Test Corp",
      features: {
        gitSource: true,
        ociSource: true,
        s3Source: true,
        loadTesting: true,
        dataGeneration: true,
        scheduling: true,
        distributedWorkers: true,
      },
      limits: {
        maxScenarios: 0,
        maxWorkerReplicas: 0,
      },
      issuedAt: new Date().toISOString(),
      expiresAt: new Date(Date.now() + 365 * 24 * 60 * 60 * 1000).toISOString(),
    };

    mockFetch.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(enterpriseLicense),
    });

    const { result } = renderHook(() => useLicense(), { wrapper: createWrapper() });

    await waitFor(() => {
      expect(result.current.license.tier).toBe("enterprise");
    });

    expect(result.current.isEnterprise).toBe(true);
    expect(result.current.license.customer).toBe("Test Corp");
  });

  it("should return open-core license on fetch error", async () => {
    mockFetch.mockResolvedValue({
      ok: false,
      status: 500,
    });

    const { result } = renderHook(() => useLicense(), { wrapper: createWrapper() });

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    // Should fall back to open-core (placeholder data)
    expect(result.current.license).toEqual(OPEN_CORE_LICENSE);
    expect(result.current.isEnterprise).toBe(false);
  });

  it("should provide canUseFeature helper", async () => {
    const enterpriseLicense: License = {
      ...OPEN_CORE_LICENSE,
      tier: "enterprise",
      features: {
        ...OPEN_CORE_LICENSE.features,
        gitSource: true,
        loadTesting: false,
      },
    };

    mockFetch.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(enterpriseLicense),
    });

    const { result } = renderHook(() => useLicense(), { wrapper: createWrapper() });

    await waitFor(() => {
      expect(result.current.license.tier).toBe("enterprise");
    });

    expect(result.current.canUseFeature("gitSource")).toBe(true);
    expect(result.current.canUseFeature("loadTesting")).toBe(false);
  });

  it("should provide canUseSourceType helper", async () => {
    const enterpriseLicense: License = {
      ...OPEN_CORE_LICENSE,
      tier: "enterprise",
      features: {
        ...OPEN_CORE_LICENSE.features,
        gitSource: true,
      },
    };

    mockFetch.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(enterpriseLicense),
    });

    const { result } = renderHook(() => useLicense(), { wrapper: createWrapper() });

    await waitFor(() => {
      expect(result.current.license.tier).toBe("enterprise");
    });

    expect(result.current.canUseSourceType("configmap")).toBe(true);
    expect(result.current.canUseSourceType("git")).toBe(true);
    expect(result.current.canUseSourceType("oci")).toBe(false);
  });

  it("should provide canUseJobType helper", async () => {
    const enterpriseLicense: License = {
      ...OPEN_CORE_LICENSE,
      tier: "enterprise",
      features: {
        ...OPEN_CORE_LICENSE.features,
        loadTesting: true,
      },
    };

    mockFetch.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(enterpriseLicense),
    });

    const { result } = renderHook(() => useLicense(), { wrapper: createWrapper() });

    await waitFor(() => {
      expect(result.current.license.tier).toBe("enterprise");
    });

    expect(result.current.canUseJobType("evaluation")).toBe(true);
    expect(result.current.canUseJobType("loadtest")).toBe(true);
    expect(result.current.canUseJobType("datagen")).toBe(false);
  });

  it("should provide canUseScheduling helper", async () => {
    const enterpriseLicense: License = {
      ...OPEN_CORE_LICENSE,
      tier: "enterprise",
      features: {
        ...OPEN_CORE_LICENSE.features,
        scheduling: true,
      },
    };

    mockFetch.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(enterpriseLicense),
    });

    const { result } = renderHook(() => useLicense(), { wrapper: createWrapper() });

    await waitFor(() => {
      expect(result.current.license.tier).toBe("enterprise");
    });

    expect(result.current.canUseScheduling()).toBe(true);
  });

  it("should provide canUseWorkerReplicas helper", async () => {
    const enterpriseLicense: License = {
      ...OPEN_CORE_LICENSE,
      tier: "enterprise",
      limits: {
        maxScenarios: 0,
        maxWorkerReplicas: 10,
      },
    };

    mockFetch.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(enterpriseLicense),
    });

    const { result } = renderHook(() => useLicense(), { wrapper: createWrapper() });

    await waitFor(() => {
      expect(result.current.license.tier).toBe("enterprise");
    });

    expect(result.current.canUseWorkerReplicas(10)).toBe(true);
    expect(result.current.canUseWorkerReplicas(11)).toBe(false);
  });

  it("should provide canUseScenarioCount helper", async () => {
    const enterpriseLicense: License = {
      ...OPEN_CORE_LICENSE,
      tier: "enterprise",
      limits: {
        maxScenarios: 100,
        maxWorkerReplicas: 0,
      },
    };

    mockFetch.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(enterpriseLicense),
    });

    const { result } = renderHook(() => useLicense(), { wrapper: createWrapper() });

    await waitFor(() => {
      expect(result.current.license.tier).toBe("enterprise");
    });

    expect(result.current.canUseScenarioCount(100)).toBe(true);
    expect(result.current.canUseScenarioCount(101)).toBe(false);
  });

  it("should provide isExpired based on license expiry", async () => {
    const expiredLicense: License = {
      ...OPEN_CORE_LICENSE,
      expiresAt: new Date(Date.now() - 24 * 60 * 60 * 1000).toISOString(),
    };

    mockFetch.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(expiredLicense),
    });

    const { result } = renderHook(() => useLicense(), { wrapper: createWrapper() });

    await waitFor(() => {
      expect(result.current.isExpired).toBe(true);
    });
  });

  it("should provide refresh function", async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(OPEN_CORE_LICENSE),
    });

    const { result } = renderHook(() => useLicense(), { wrapper: createWrapper() });

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    // Clear the mock to track new calls
    mockFetch.mockClear();

    // Call refresh
    result.current.refresh();

    // Should trigger a new fetch
    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalled();
    });
  });

  it("should fetch from /api/license endpoint", async () => {
    mockFetch.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(OPEN_CORE_LICENSE),
    });

    renderHook(() => useLicense(), { wrapper: createWrapper() });

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith("/api/license");
    });
  });
});
