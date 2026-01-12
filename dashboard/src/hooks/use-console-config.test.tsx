import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useConsoleConfig } from "./use-console-config";

// Mock agent data with console config
const mockAgentWithConsoleConfig = {
  metadata: {
    name: "test-agent",
    namespace: "production",
    uid: "uid-1",
  },
  spec: {
    console: {
      allowedAttachmentTypes: ["image/*", "application/pdf"],
      maxFileSize: 20 * 1024 * 1024, // 20MB
      maxFiles: 10,
    },
  },
  status: {
    phase: "Running",
  },
};

const mockAgentWithoutConsoleConfig = {
  metadata: {
    name: "simple-agent",
    namespace: "production",
    uid: "uid-2",
  },
  spec: {},
  status: {
    phase: "Running",
  },
};

// Mock useAgent hook
const mockUseAgent = vi.fn();
vi.mock("./use-agents", () => ({
  useAgent: (...args: unknown[]) => mockUseAgent(...args),
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

describe("useConsoleConfig", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should return loading state initially", () => {
    mockUseAgent.mockReturnValue({
      data: undefined,
      isLoading: true,
      error: null,
    });

    const { result } = renderHook(
      () => useConsoleConfig("production", "test-agent"),
      { wrapper: TestWrapper }
    );

    expect(result.current.isLoading).toBe(true);
  });

  it("should return config from agent with console settings", async () => {
    mockUseAgent.mockReturnValue({
      data: mockAgentWithConsoleConfig,
      isLoading: false,
      error: null,
    });

    const { result } = renderHook(
      () => useConsoleConfig("production", "test-agent"),
      { wrapper: TestWrapper }
    );

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.config.allowedMimeTypes).toEqual(["image/*", "application/pdf"]);
    expect(result.current.config.maxFileSize).toBe(20 * 1024 * 1024);
    expect(result.current.config.maxFiles).toBe(10);
    expect(result.current.rawConfig).toEqual(mockAgentWithConsoleConfig.spec.console);
  });

  it("should return default config when agent has no console settings", async () => {
    mockUseAgent.mockReturnValue({
      data: mockAgentWithoutConsoleConfig,
      isLoading: false,
      error: null,
    });

    const { result } = renderHook(
      () => useConsoleConfig("production", "simple-agent"),
      { wrapper: TestWrapper }
    );

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    // Should have default config values
    expect(result.current.config).toBeDefined();
    expect(result.current.rawConfig).toBeUndefined();
  });

  it("should return error when agent fetch fails", async () => {
    const mockError = new Error("Failed to fetch agent");
    mockUseAgent.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: mockError,
    });

    const { result } = renderHook(
      () => useConsoleConfig("production", "test-agent"),
      { wrapper: TestWrapper }
    );

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.error).toBe(mockError);
  });

  it("should call useAgent with correct namespace and agent name", () => {
    mockUseAgent.mockReturnValue({
      data: undefined,
      isLoading: true,
      error: null,
    });

    renderHook(
      () => useConsoleConfig("staging", "my-agent"),
      { wrapper: TestWrapper }
    );

    expect(mockUseAgent).toHaveBeenCalledWith("my-agent", "staging");
  });

  it("should return default config when agent data is null", async () => {
    mockUseAgent.mockReturnValue({
      data: null,
      isLoading: false,
      error: null,
    });

    const { result } = renderHook(
      () => useConsoleConfig("production", "non-existent"),
      { wrapper: TestWrapper }
    );

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    // Should have default config values from buildAttachmentConfig(undefined)
    expect(result.current.config).toBeDefined();
  });
});
