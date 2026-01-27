/**
 * Tests for ImportProviderDialog component.
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { ImportProviderDialog } from "./import-provider-dialog";
import type { Provider } from "@/types";

// Mock the providers hook
vi.mock("@/hooks/use-providers", () => ({
  useProviders: vi.fn(),
}));

// Mock the import converters
vi.mock("@/lib/arena/import-converters", () => ({
  convertProviderToArena: vi.fn((p: Provider) => `yaml-content-for-${p.metadata.name}`),
  generateProviderFilename: vi.fn((p: Provider) => `${p.metadata.name}.provider.yaml`),
}));

const mockProviders: Provider[] = [
  {
    apiVersion: "omnia.altairalabs.ai/v1alpha1",
    kind: "Provider",
    metadata: { name: "openai-gpt4", namespace: "default" },
    spec: { type: "openai", model: "gpt-4" },
  },
  {
    apiVersion: "omnia.altairalabs.ai/v1alpha1",
    kind: "Provider",
    metadata: { name: "claude-sonnet", namespace: "test-ns" },
    spec: { type: "claude", model: "claude-3" },
  },
];

// Helper to create mock return value - cast to any to satisfy TypeScript
// eslint-disable-next-line @typescript-eslint/no-explicit-any
function mockQueryResult(data: unknown, isLoading = false): any {
  return { data, isLoading, error: null, refetch: vi.fn() };
}

describe("ImportProviderDialog", () => {
  const defaultProps = {
    open: true,
    onOpenChange: vi.fn(),
    parentPath: null,
    onImport: vi.fn().mockResolvedValue(undefined),
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should show loading state", async () => {
    const { useProviders } = await import("@/hooks/use-providers");
    vi.mocked(useProviders).mockReturnValue(mockQueryResult(undefined, true));

    render(<ImportProviderDialog {...defaultProps} />);

    expect(screen.getByText(/loading providers/i)).toBeInTheDocument();
  });

  it("should show empty state when no providers", async () => {
    const { useProviders } = await import("@/hooks/use-providers");
    vi.mocked(useProviders).mockReturnValue(mockQueryResult([]));

    render(<ImportProviderDialog {...defaultProps} />);

    expect(screen.getByText(/no providers found/i)).toBeInTheDocument();
  });

  it("should render provider list", async () => {
    const { useProviders } = await import("@/hooks/use-providers");
    vi.mocked(useProviders).mockReturnValue(mockQueryResult(mockProviders));

    render(<ImportProviderDialog {...defaultProps} />);

    expect(screen.getByText("openai-gpt4")).toBeInTheDocument();
    expect(screen.getByText("claude-sonnet")).toBeInTheDocument();
    expect(screen.getByText(/openai · gpt-4/i)).toBeInTheDocument();
  });

  it("should show parent path in description", async () => {
    const { useProviders } = await import("@/hooks/use-providers");
    vi.mocked(useProviders).mockReturnValue(mockQueryResult(mockProviders));

    render(<ImportProviderDialog {...defaultProps} parentPath="prompts" />);

    expect(screen.getByText(/import providers into "prompts"/i)).toBeInTheDocument();
  });

  it("should show root level description when no parent path", async () => {
    const { useProviders } = await import("@/hooks/use-providers");
    vi.mocked(useProviders).mockReturnValue(mockQueryResult(mockProviders));

    render(<ImportProviderDialog {...defaultProps} parentPath={null} />);

    expect(screen.getByText(/import providers at the root level/i)).toBeInTheDocument();
  });

  it("should toggle provider selection", async () => {
    const { useProviders } = await import("@/hooks/use-providers");
    vi.mocked(useProviders).mockReturnValue(mockQueryResult(mockProviders));

    render(<ImportProviderDialog {...defaultProps} />);

    // Initially 0 selected
    expect(screen.getByText("0 of 2 selected")).toBeInTheDocument();

    // Click first provider
    fireEvent.click(screen.getByText("openai-gpt4"));
    expect(screen.getByText("1 of 2 selected")).toBeInTheDocument();

    // Click again to deselect
    fireEvent.click(screen.getByText("openai-gpt4"));
    expect(screen.getByText("0 of 2 selected")).toBeInTheDocument();
  });

  it("should select all providers", async () => {
    const { useProviders } = await import("@/hooks/use-providers");
    vi.mocked(useProviders).mockReturnValue(mockQueryResult(mockProviders));

    render(<ImportProviderDialog {...defaultProps} />);

    fireEvent.click(screen.getByLabelText("Select all"));
    expect(screen.getByText("2 of 2 selected")).toBeInTheDocument();

    // Click again to deselect all
    fireEvent.click(screen.getByLabelText("Select all"));
    expect(screen.getByText("0 of 2 selected")).toBeInTheDocument();
  });

  it("should disable import button when nothing selected", async () => {
    const { useProviders } = await import("@/hooks/use-providers");
    vi.mocked(useProviders).mockReturnValue(mockQueryResult(mockProviders));

    render(<ImportProviderDialog {...defaultProps} />);

    expect(screen.getByRole("button", { name: /import/i })).toBeDisabled();
  });

  it("should enable import button when provider selected", async () => {
    const { useProviders } = await import("@/hooks/use-providers");
    vi.mocked(useProviders).mockReturnValue(mockQueryResult(mockProviders));

    render(<ImportProviderDialog {...defaultProps} />);

    fireEvent.click(screen.getByText("openai-gpt4"));
    expect(screen.getByRole("button", { name: /import/i })).not.toBeDisabled();
  });

  it("should call onImport with converted files", async () => {
    const { useProviders } = await import("@/hooks/use-providers");
    vi.mocked(useProviders).mockReturnValue(mockQueryResult(mockProviders));

    const onImport = vi.fn().mockResolvedValue(undefined);
    render(<ImportProviderDialog {...defaultProps} onImport={onImport} />);

    // Select first provider
    fireEvent.click(screen.getByText("openai-gpt4"));
    // Click import
    fireEvent.click(screen.getByRole("button", { name: /import/i }));

    await waitFor(() => {
      expect(onImport).toHaveBeenCalledWith([
        {
          name: "openai-gpt4.provider.yaml",
          content: "yaml-content-for-openai-gpt4",
        },
      ]);
    });
  });

  it("should close dialog after successful import", async () => {
    const { useProviders } = await import("@/hooks/use-providers");
    vi.mocked(useProviders).mockReturnValue(mockQueryResult(mockProviders));

    const onOpenChange = vi.fn();
    render(<ImportProviderDialog {...defaultProps} onOpenChange={onOpenChange} />);

    fireEvent.click(screen.getByText("openai-gpt4"));
    fireEvent.click(screen.getByRole("button", { name: /import/i }));

    await waitFor(() => {
      expect(onOpenChange).toHaveBeenCalledWith(false);
    });
  });

  it("should show error on import failure", async () => {
    const { useProviders } = await import("@/hooks/use-providers");
    vi.mocked(useProviders).mockReturnValue(mockQueryResult(mockProviders));

    const onImport = vi.fn().mockRejectedValue(new Error("Import failed"));
    render(<ImportProviderDialog {...defaultProps} onImport={onImport} />);

    fireEvent.click(screen.getByText("openai-gpt4"));
    fireEvent.click(screen.getByRole("button", { name: /import/i }));

    await waitFor(() => {
      expect(screen.getByText("Import failed")).toBeInTheDocument();
    });
  });

  it("should show generic error for non-Error exceptions", async () => {
    const { useProviders } = await import("@/hooks/use-providers");
    vi.mocked(useProviders).mockReturnValue(mockQueryResult(mockProviders));

    const onImport = vi.fn().mockRejectedValue("string error");
    render(<ImportProviderDialog {...defaultProps} onImport={onImport} />);

    fireEvent.click(screen.getByText("openai-gpt4"));
    fireEvent.click(screen.getByRole("button", { name: /import/i }));

    await waitFor(() => {
      expect(screen.getByText("Failed to import providers")).toBeInTheDocument();
    });
  });

  it("should handle undefined providers data", async () => {
    const { useProviders } = await import("@/hooks/use-providers");
    vi.mocked(useProviders).mockReturnValue(mockQueryResult(undefined));

    render(<ImportProviderDialog {...defaultProps} />);

    expect(screen.getByText(/no providers found/i)).toBeInTheDocument();
  });

  it("should call onOpenChange when cancel clicked", async () => {
    const { useProviders } = await import("@/hooks/use-providers");
    vi.mocked(useProviders).mockReturnValue(mockQueryResult(mockProviders));

    const onOpenChange = vi.fn();
    render(<ImportProviderDialog {...defaultProps} onOpenChange={onOpenChange} />);

    fireEvent.click(screen.getByRole("button", { name: /cancel/i }));

    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("should handle provider without model", async () => {
    const { useProviders } = await import("@/hooks/use-providers");
    vi.mocked(useProviders).mockReturnValue(mockQueryResult([
      {
        apiVersion: "omnia.altairalabs.ai/v1alpha1",
        kind: "Provider",
        metadata: { name: "ollama-provider" },
        spec: { type: "ollama" },
      },
    ]));

    render(<ImportProviderDialog {...defaultProps} />);

    expect(screen.getByText("ollama-provider")).toBeInTheDocument();
    expect(screen.getByText("ollama")).toBeInTheDocument();
  });
});
