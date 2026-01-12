import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { usePromptPackContent } from "./use-promptpack-content";

// Mock prompt pack content data
const mockPromptPackContent = {
  system: "You are a helpful assistant",
  model: "gpt-4",
  temperature: 0.7,
  tools: ["search", "calculator"],
};

// Mock useDataService
const mockGetPromptPackContent = vi.fn();
vi.mock("@/lib/data", () => ({
  useDataService: () => ({
    name: "mock",
    getPromptPackContent: mockGetPromptPackContent,
  }),
}));

function TestWrapper({ children }: { children: React.ReactNode }) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        gcTime: 0,
      },
    },
  });
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
}

describe("usePromptPackContent", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetPromptPackContent.mockResolvedValue(mockPromptPackContent);
  });

  it("should fetch prompt pack content by name and namespace", async () => {
    const { result } = renderHook(
      () => usePromptPackContent("my-pack", "production"),
      { wrapper: TestWrapper }
    );

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(result.current.data).toEqual(mockPromptPackContent);
    expect(mockGetPromptPackContent).toHaveBeenCalledWith("production", "my-pack");
  });

  it("should be in loading state initially", () => {
    const { result } = renderHook(
      () => usePromptPackContent("my-pack", "production"),
      { wrapper: TestWrapper }
    );

    expect(result.current.isLoading).toBe(true);
  });

  it("should use default namespace when not provided", async () => {
    renderHook(
      () => usePromptPackContent("my-pack"),
      { wrapper: TestWrapper }
    );

    await waitFor(() => {
      expect(mockGetPromptPackContent).toHaveBeenCalled();
    });

    expect(mockGetPromptPackContent).toHaveBeenCalledWith("default", "my-pack");
  });

  it("should not fetch when name is empty", () => {
    renderHook(
      () => usePromptPackContent(""),
      { wrapper: TestWrapper }
    );

    expect(mockGetPromptPackContent).not.toHaveBeenCalled();
  });

  it("should return null when prompt pack content not found", async () => {
    mockGetPromptPackContent.mockResolvedValueOnce(null);

    const { result } = renderHook(
      () => usePromptPackContent("non-existent", "production"),
      { wrapper: TestWrapper }
    );

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(result.current.data).toBeNull();
  });

  it("should handle fetch errors", async () => {
    const mockError = new Error("Failed to fetch prompt pack content");
    mockGetPromptPackContent.mockRejectedValueOnce(mockError);

    const { result } = renderHook(
      () => usePromptPackContent("my-pack", "production"),
      { wrapper: TestWrapper }
    );

    await waitFor(() => {
      expect(result.current.isError).toBe(true);
    });

    expect(result.current.error).toBe(mockError);
  });

  it("should return null when response is undefined", async () => {
    mockGetPromptPackContent.mockResolvedValueOnce(undefined);

    const { result } = renderHook(
      () => usePromptPackContent("my-pack", "production"),
      { wrapper: TestWrapper }
    );

    await waitFor(() => {
      expect(result.current.isSuccess).toBe(true);
    });

    expect(result.current.data).toBeNull();
  });
});
