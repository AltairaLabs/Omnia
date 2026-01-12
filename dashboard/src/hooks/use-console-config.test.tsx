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

// Mock useProvider hook
const mockUseProvider = vi.fn();
vi.mock("./use-provider", () => ({
  useProvider: (...args: unknown[]) => mockUseProvider(...args),
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
    // Default mock for useProvider - no provider ref
    mockUseProvider.mockReturnValue({
      data: null,
      isLoading: false,
      error: null,
    });
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

  it("should return conservative media requirements when no provider specified", async () => {
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

    // Should have conservative media requirements (no provider = auto = conservative)
    expect(result.current.mediaRequirements).toBeDefined();
    expect(result.current.mediaRequirements.image).toBeDefined();
    expect(result.current.providerType).toBeUndefined();
  });

  it("should return provider-specific media requirements for claude provider", async () => {
    const mockAgentWithClaudeProvider = {
      ...mockAgentWithoutConsoleConfig,
      spec: {
        provider: { type: "claude" },
      },
    };

    mockUseAgent.mockReturnValue({
      data: mockAgentWithClaudeProvider,
      isLoading: false,
      error: null,
    });

    const { result } = renderHook(
      () => useConsoleConfig("production", "claude-agent"),
      { wrapper: TestWrapper }
    );

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    // Should have claude-specific media requirements
    expect(result.current.providerType).toBe("claude");
    expect(result.current.mediaRequirements.image?.maxSizeBytes).toBe(5 * 1024 * 1024);
    expect(result.current.mediaRequirements.image?.compressionGuidance).toBe("lossless");
  });

  it("should use provider from ProviderRef when available", async () => {
    const mockAgentWithProviderRef = {
      ...mockAgentWithoutConsoleConfig,
      spec: {
        providerRef: { name: "shared-openai", namespace: "production" },
        provider: { type: "claude" }, // inline provider should be overridden
      },
    };

    mockUseAgent.mockReturnValue({
      data: mockAgentWithProviderRef,
      isLoading: false,
      error: null,
    });

    // Mock the provider CRD
    mockUseProvider.mockReturnValue({
      data: { spec: { type: "openai" } },
      isLoading: false,
      error: null,
    });

    const { result } = renderHook(
      () => useConsoleConfig("production", "openai-agent"),
      { wrapper: TestWrapper }
    );

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    // Should use openai from ProviderRef, not claude from inline provider
    expect(result.current.providerType).toBe("openai");
    expect(result.current.mediaRequirements.image?.maxSizeBytes).toBe(20 * 1024 * 1024);
  });

  it("should use CRD media requirements overrides when specified", async () => {
    const mockAgentWithOverrides = {
      ...mockAgentWithoutConsoleConfig,
      spec: {
        provider: { type: "claude" },
        console: {
          mediaRequirements: {
            image: {
              maxSizeBytes: 50 * 1024 * 1024, // Override claude default
            },
          },
        },
      },
    };

    mockUseAgent.mockReturnValue({
      data: mockAgentWithOverrides,
      isLoading: false,
      error: null,
    });

    const { result } = renderHook(
      () => useConsoleConfig("production", "custom-agent"),
      { wrapper: TestWrapper }
    );

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    // Should use overridden value
    expect(result.current.mediaRequirements.image?.maxSizeBytes).toBe(50 * 1024 * 1024);
  });

  it("should merge partial overrides with provider defaults", async () => {
    // Only override video, let other fields fall back to provider defaults
    const mockAgentWithPartialOverrides = {
      ...mockAgentWithoutConsoleConfig,
      spec: {
        provider: { type: "openai" },
        console: {
          mediaRequirements: {
            video: {
              maxDurationSeconds: 120,
            },
          },
        },
      },
    };

    mockUseAgent.mockReturnValue({
      data: mockAgentWithPartialOverrides,
      isLoading: false,
      error: null,
    });

    const { result } = renderHook(
      () => useConsoleConfig("production", "partial-override-agent"),
      { wrapper: TestWrapper }
    );

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    // Video should use override
    expect(result.current.mediaRequirements.video?.maxDurationSeconds).toBe(120);
    // Image should fall back to openai defaults (not overridden)
    expect(result.current.mediaRequirements.image?.maxSizeBytes).toBe(20 * 1024 * 1024);
  });
});
