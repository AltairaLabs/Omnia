import { renderHook, waitFor, act } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React from "react";

const { mockGetConsent, mockUpdateConsent, mockUseAuth } = vi.hoisted(() => ({
  mockGetConsent: vi.fn(),
  mockUpdateConsent: vi.fn(),
  mockUseAuth: vi.fn(),
}));

vi.mock("@/lib/data/consent-service", () => ({
  ConsentService: class MockConsentService {
    getConsent = mockGetConsent;
    updateConsent = mockUpdateConsent;
  },
}));

vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: () => ({
    currentWorkspace: { name: "test-workspace" },
  }),
}));

vi.mock("@/hooks/use-auth", () => ({
  useAuth: () => mockUseAuth(),
}));

// Import after mocks
import { useConsent, useUpdateConsent } from "./use-consent";

const mockConsentResponse = {
  grants: ["analytics"],
  defaults: ["essential"],
  denied: ["marketing"],
};

// Default: an authenticated user whose memory identity is their user id.
function authedAuth() {
  return { memoryUserId: "user-123", hasMemoryIdentity: true };
}

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  function TestQueryProvider({ children }: { children: React.ReactNode }) {
    return React.createElement(QueryClientProvider, { client: queryClient }, children);
  }
  return TestQueryProvider;
}

describe("useConsent", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseAuth.mockReturnValue(authedAuth());
    mockGetConsent.mockResolvedValue(mockConsentResponse);
  });

  it("fetches consent for the current workspace and memory identity", async () => {
    const { result } = renderHook(() => useConsent(), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(mockGetConsent).toHaveBeenCalledWith("test-workspace", "user-123");
    expect(result.current.data?.grants).toEqual(["analytics"]);
    expect(result.current.data?.defaults).toEqual(["essential"]);
    expect(result.current.data?.denied).toEqual(["marketing"]);
  });

  it("scopes anonymous consent to the deviceId, not the literal 'anonymous'", async () => {
    // Anonymous user: memoryUserId is the per-device pseudonym (deviceId), the
    // same identity anonymous memory uses. Consent must key on it so anonymous
    // users don't share one "anonymous" bucket. Regression for #1269.
    mockUseAuth.mockReturnValue({ memoryUserId: "device-abc123", hasMemoryIdentity: true });

    const { result } = renderHook(() => useConsent(), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(mockGetConsent).toHaveBeenCalledWith("test-workspace", "device-abc123");
    expect(mockGetConsent).not.toHaveBeenCalledWith("test-workspace", "anonymous");
  });

  it("is disabled (no fetch) when there is no memory identity", async () => {
    mockUseAuth.mockReturnValue({ memoryUserId: undefined, hasMemoryIdentity: false });

    const { result } = renderHook(() => useConsent(), { wrapper: createWrapper() });

    // Query is disabled — it never fetches.
    expect(result.current.fetchStatus).toBe("idle");
    expect(mockGetConsent).not.toHaveBeenCalled();
  });

  it("handles loading state", () => {
    mockGetConsent.mockReturnValue(new Promise(() => {}));
    const { result } = renderHook(() => useConsent(), { wrapper: createWrapper() });
    expect(result.current.isLoading).toBe(true);
  });

  it("handles error state", async () => {
    mockGetConsent.mockRejectedValue(new Error("Failed to fetch consent"));
    const { result } = renderHook(() => useConsent(), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error).toBeInstanceOf(Error);
  });
});

describe("useUpdateConsent", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseAuth.mockReturnValue(authedAuth());
    mockUpdateConsent.mockResolvedValue({
      grants: ["analytics", "personalization"],
      defaults: ["essential"],
      denied: [],
    });
    mockGetConsent.mockResolvedValue(mockConsentResponse);
  });

  it("calls updateConsent with workspace, memory identity, and request", async () => {
    const { result } = renderHook(() => useUpdateConsent(), { wrapper: createWrapper() });

    await act(async () => {
      await result.current.mutateAsync({ grants: ["analytics", "personalization"] });
    });

    expect(mockUpdateConsent).toHaveBeenCalledWith(
      "test-workspace",
      "user-123",
      { grants: ["analytics", "personalization"] }
    );
  });

  it("writes anonymous consent under the deviceId, not 'anonymous'", async () => {
    mockUseAuth.mockReturnValue({ memoryUserId: "device-abc123", hasMemoryIdentity: true });
    const { result } = renderHook(() => useUpdateConsent(), { wrapper: createWrapper() });

    await act(async () => {
      await result.current.mutateAsync({ grants: ["analytics"] });
    });

    expect(mockUpdateConsent).toHaveBeenCalledWith(
      "test-workspace",
      "device-abc123",
      { grants: ["analytics"] }
    );
  });

  it("returns updated consent data on success", async () => {
    const { result } = renderHook(() => useUpdateConsent(), { wrapper: createWrapper() });

    let data;
    await act(async () => {
      data = await result.current.mutateAsync({ revocations: ["analytics"] });
    });

    expect(data).toEqual({
      grants: ["analytics", "personalization"],
      defaults: ["essential"],
      denied: [],
    });
  });

  it("propagates errors from updateConsent", async () => {
    mockUpdateConsent.mockRejectedValue(new Error("Failed to update consent"));
    const { result } = renderHook(() => useUpdateConsent(), { wrapper: createWrapper() });

    await act(async () => {
      await expect(
        result.current.mutateAsync({ grants: ["unknown"] })
      ).rejects.toThrow("Failed to update consent");
    });
  });
});
