/**
 * Tests for ImportToolDialog component.
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { ImportToolDialog } from "./import-tool-dialog";
import type { ToolRegistry } from "@/types";

// Mock the tool registries hook
vi.mock("@/hooks/use-tool-registries", () => ({
  useToolRegistries: vi.fn(),
}));

// Mock the import converters
vi.mock("@/lib/arena/import-converters", () => ({
  convertToolToArena: vi.fn(
    (tool, opts) => `yaml-content-for-${opts.registryName}-${tool.name}`
  ),
  generateToolFilename: vi.fn(
    (tool, registryName) => `${registryName}-${tool.name}.tool.yaml`
  ),
}));

const mockRegistries: ToolRegistry[] = [
  {
    apiVersion: "omnia.altairalabs.ai/v1alpha1",
    kind: "ToolRegistry",
    metadata: { name: "http-tools", namespace: "default" },
    spec: { handlers: [] },
    status: {
      phase: "Ready",
      discoveredToolsCount: 2,
      discoveredTools: [
        { name: "get-weather", handlerName: "weather", description: "Get weather data", endpoint: "http://localhost", status: "Available" },
        { name: "send-email", handlerName: "email", description: "Send an email", endpoint: "http://localhost", status: "Unavailable" },
      ],
    },
  },
  {
    apiVersion: "omnia.altairalabs.ai/v1alpha1",
    kind: "ToolRegistry",
    metadata: { name: "mcp-tools", namespace: "test-ns" },
    spec: { handlers: [] },
    status: {
      phase: "Ready",
      discoveredToolsCount: 1,
      discoveredTools: [
        { name: "search-docs", handlerName: "search", description: "", endpoint: "http://localhost", status: "Available" },
      ],
    },
  },
];

// Helper to create mock return value - cast to any to satisfy TypeScript
// eslint-disable-next-line @typescript-eslint/no-explicit-any
function mockQueryResult(data: unknown, isLoading = false): any {
  return { data, isLoading, error: null, refetch: vi.fn() };
}

describe("ImportToolDialog", () => {
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
    const { useToolRegistries } = await import("@/hooks/use-tool-registries");
    vi.mocked(useToolRegistries).mockReturnValue(mockQueryResult(undefined, true));

    render(<ImportToolDialog {...defaultProps} />);

    expect(screen.getByText(/loading tool registries/i)).toBeInTheDocument();
  });

  it("should show empty state when no registries", async () => {
    const { useToolRegistries } = await import("@/hooks/use-tool-registries");
    vi.mocked(useToolRegistries).mockReturnValue(mockQueryResult([]));

    render(<ImportToolDialog {...defaultProps} />);

    expect(screen.getByText(/no tool registries found/i)).toBeInTheDocument();
  });

  it("should show step 1/2 on initial render", async () => {
    const { useToolRegistries } = await import("@/hooks/use-tool-registries");
    vi.mocked(useToolRegistries).mockReturnValue(mockQueryResult(mockRegistries));

    render(<ImportToolDialog {...defaultProps} />);

    expect(screen.getByText(/import tools \(1\/2\)/i)).toBeInTheDocument();
  });

  it("should render registry list", async () => {
    const { useToolRegistries } = await import("@/hooks/use-tool-registries");
    vi.mocked(useToolRegistries).mockReturnValue(mockQueryResult(mockRegistries));

    render(<ImportToolDialog {...defaultProps} />);

    expect(screen.getByText("http-tools")).toBeInTheDocument();
    expect(screen.getByText("mcp-tools")).toBeInTheDocument();
    expect(screen.getByText("2 tools")).toBeInTheDocument();
    expect(screen.getByText("1 tool")).toBeInTheDocument();
  });

  it("should show parent path in description", async () => {
    const { useToolRegistries } = await import("@/hooks/use-tool-registries");
    vi.mocked(useToolRegistries).mockReturnValue(mockQueryResult(mockRegistries));

    render(<ImportToolDialog {...defaultProps} parentPath="tools" />);

    expect(screen.getByText(/import tools into "tools"/i)).toBeInTheDocument();
  });

  it("should disable Next button when no registry selected", async () => {
    const { useToolRegistries } = await import("@/hooks/use-tool-registries");
    vi.mocked(useToolRegistries).mockReturnValue(mockQueryResult(mockRegistries));

    render(<ImportToolDialog {...defaultProps} />);

    expect(screen.getByRole("button", { name: /next/i })).toBeDisabled();
  });

  it("should enable Next button when registry selected", async () => {
    const { useToolRegistries } = await import("@/hooks/use-tool-registries");
    vi.mocked(useToolRegistries).mockReturnValue(mockQueryResult(mockRegistries));

    render(<ImportToolDialog {...defaultProps} />);

    fireEvent.click(screen.getByText("http-tools"));
    expect(screen.getByRole("button", { name: /next/i })).not.toBeDisabled();
  });

  it("should navigate to step 2 when Next clicked", async () => {
    const { useToolRegistries } = await import("@/hooks/use-tool-registries");
    vi.mocked(useToolRegistries).mockReturnValue(mockQueryResult(mockRegistries));

    render(<ImportToolDialog {...defaultProps} />);

    // Select registry
    fireEvent.click(screen.getByText("http-tools"));
    // Click Next
    fireEvent.click(screen.getByRole("button", { name: /next/i }));

    // Should now be on step 2
    expect(screen.getByText(/import tools \(2\/2\)/i)).toBeInTheDocument();
    expect(screen.getByText(/select tools from "http-tools"/i)).toBeInTheDocument();
  });

  it("should render tool list in step 2", async () => {
    const { useToolRegistries } = await import("@/hooks/use-tool-registries");
    vi.mocked(useToolRegistries).mockReturnValue(mockQueryResult(mockRegistries));

    render(<ImportToolDialog {...defaultProps} />);

    // Navigate to step 2
    fireEvent.click(screen.getByText("http-tools"));
    fireEvent.click(screen.getByRole("button", { name: /next/i }));

    // Should show tools
    expect(screen.getByText("get-weather")).toBeInTheDocument();
    expect(screen.getByText("send-email")).toBeInTheDocument();
    expect(screen.getByText("Get weather data")).toBeInTheDocument();
    expect(screen.getByText("Available")).toBeInTheDocument();
    expect(screen.getByText("Unavailable")).toBeInTheDocument();
  });

  it("should toggle tool selection", async () => {
    const { useToolRegistries } = await import("@/hooks/use-tool-registries");
    vi.mocked(useToolRegistries).mockReturnValue(mockQueryResult(mockRegistries));

    render(<ImportToolDialog {...defaultProps} />);

    // Navigate to step 2
    fireEvent.click(screen.getByText("http-tools"));
    fireEvent.click(screen.getByRole("button", { name: /next/i }));

    // Initially 0 selected
    expect(screen.getByText("0 of 2 selected")).toBeInTheDocument();

    // Select first tool
    fireEvent.click(screen.getByText("get-weather"));
    expect(screen.getByText("1 of 2 selected")).toBeInTheDocument();

    // Deselect
    fireEvent.click(screen.getByText("get-weather"));
    expect(screen.getByText("0 of 2 selected")).toBeInTheDocument();
  });

  it("should select all tools", async () => {
    const { useToolRegistries } = await import("@/hooks/use-tool-registries");
    vi.mocked(useToolRegistries).mockReturnValue(mockQueryResult(mockRegistries));

    render(<ImportToolDialog {...defaultProps} />);

    // Navigate to step 2
    fireEvent.click(screen.getByText("http-tools"));
    fireEvent.click(screen.getByRole("button", { name: /next/i }));

    // Select all
    fireEvent.click(screen.getByLabelText("Select all"));
    expect(screen.getByText("2 of 2 selected")).toBeInTheDocument();

    // Deselect all
    fireEvent.click(screen.getByLabelText("Select all"));
    expect(screen.getByText("0 of 2 selected")).toBeInTheDocument();
  });

  it("should navigate back to step 1", async () => {
    const { useToolRegistries } = await import("@/hooks/use-tool-registries");
    vi.mocked(useToolRegistries).mockReturnValue(mockQueryResult(mockRegistries));

    render(<ImportToolDialog {...defaultProps} />);

    // Navigate to step 2
    fireEvent.click(screen.getByText("http-tools"));
    fireEvent.click(screen.getByRole("button", { name: /next/i }));

    // Click Back
    fireEvent.click(screen.getByRole("button", { name: /back/i }));

    // Should be back on step 1
    expect(screen.getByText(/import tools \(1\/2\)/i)).toBeInTheDocument();
  });

  it("should disable Import button when no tools selected", async () => {
    const { useToolRegistries } = await import("@/hooks/use-tool-registries");
    vi.mocked(useToolRegistries).mockReturnValue(mockQueryResult(mockRegistries));

    render(<ImportToolDialog {...defaultProps} />);

    // Navigate to step 2
    fireEvent.click(screen.getByText("http-tools"));
    fireEvent.click(screen.getByRole("button", { name: /next/i }));

    expect(screen.getByRole("button", { name: /import/i })).toBeDisabled();
  });

  it("should call onImport with converted files", async () => {
    const { useToolRegistries } = await import("@/hooks/use-tool-registries");
    vi.mocked(useToolRegistries).mockReturnValue(mockQueryResult(mockRegistries));

    const onImport = vi.fn().mockResolvedValue(undefined);
    render(<ImportToolDialog {...defaultProps} onImport={onImport} />);

    // Navigate to step 2
    fireEvent.click(screen.getByText("http-tools"));
    fireEvent.click(screen.getByRole("button", { name: /next/i }));

    // Select a tool
    fireEvent.click(screen.getByText("get-weather"));

    // Click Import
    fireEvent.click(screen.getByRole("button", { name: /import/i }));

    await waitFor(() => {
      expect(onImport).toHaveBeenCalledWith([
        {
          name: "http-tools-get-weather.tool.yaml",
          content: "yaml-content-for-http-tools-get-weather",
        },
      ]);
    });
  });

  it("should close dialog after successful import", async () => {
    const { useToolRegistries } = await import("@/hooks/use-tool-registries");
    vi.mocked(useToolRegistries).mockReturnValue(mockQueryResult(mockRegistries));

    const onOpenChange = vi.fn();
    render(<ImportToolDialog {...defaultProps} onOpenChange={onOpenChange} />);

    // Navigate to step 2
    fireEvent.click(screen.getByText("http-tools"));
    fireEvent.click(screen.getByRole("button", { name: /next/i }));

    // Select and import
    fireEvent.click(screen.getByText("get-weather"));
    fireEvent.click(screen.getByRole("button", { name: /import/i }));

    await waitFor(() => {
      expect(onOpenChange).toHaveBeenCalledWith(false);
    });
  });

  it("should show error on import failure", async () => {
    const { useToolRegistries } = await import("@/hooks/use-tool-registries");
    vi.mocked(useToolRegistries).mockReturnValue(mockQueryResult(mockRegistries));

    const onImport = vi.fn().mockRejectedValue(new Error("Import failed"));
    render(<ImportToolDialog {...defaultProps} onImport={onImport} />);

    // Navigate to step 2
    fireEvent.click(screen.getByText("http-tools"));
    fireEvent.click(screen.getByRole("button", { name: /next/i }));

    // Select and import
    fireEvent.click(screen.getByText("get-weather"));
    fireEvent.click(screen.getByRole("button", { name: /import/i }));

    await waitFor(() => {
      expect(screen.getByText("Import failed")).toBeInTheDocument();
    });
  });

  it("should show generic error for non-Error exceptions", async () => {
    const { useToolRegistries } = await import("@/hooks/use-tool-registries");
    vi.mocked(useToolRegistries).mockReturnValue(mockQueryResult(mockRegistries));

    const onImport = vi.fn().mockRejectedValue("string error");
    render(<ImportToolDialog {...defaultProps} onImport={onImport} />);

    // Navigate to step 2
    fireEvent.click(screen.getByText("http-tools"));
    fireEvent.click(screen.getByRole("button", { name: /next/i }));

    // Select and import
    fireEvent.click(screen.getByText("get-weather"));
    fireEvent.click(screen.getByRole("button", { name: /import/i }));

    await waitFor(() => {
      expect(screen.getByText("Failed to import tools")).toBeInTheDocument();
    });
  });

  it("should call onOpenChange when cancel clicked on step 1", async () => {
    const { useToolRegistries } = await import("@/hooks/use-tool-registries");
    vi.mocked(useToolRegistries).mockReturnValue(mockQueryResult(mockRegistries));

    const onOpenChange = vi.fn();
    render(<ImportToolDialog {...defaultProps} onOpenChange={onOpenChange} />);

    fireEvent.click(screen.getByRole("button", { name: /cancel/i }));

    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("should call onOpenChange when cancel clicked on step 2", async () => {
    const { useToolRegistries } = await import("@/hooks/use-tool-registries");
    vi.mocked(useToolRegistries).mockReturnValue(mockQueryResult(mockRegistries));

    const onOpenChange = vi.fn();
    render(<ImportToolDialog {...defaultProps} onOpenChange={onOpenChange} />);

    // Navigate to step 2
    fireEvent.click(screen.getByText("http-tools"));
    fireEvent.click(screen.getByRole("button", { name: /next/i }));

    fireEvent.click(screen.getByRole("button", { name: /cancel/i }));

    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("should show empty tools message when registry has no tools", async () => {
    const { useToolRegistries } = await import("@/hooks/use-tool-registries");
    vi.mocked(useToolRegistries).mockReturnValue(mockQueryResult([
      {
        apiVersion: "omnia.altairalabs.ai/v1alpha1",
        kind: "ToolRegistry",
        metadata: { name: "empty-registry" },
        spec: { handlers: [] },
        status: {
          phase: "Ready",
          discoveredToolsCount: 0,
          discoveredTools: [],
        },
      },
    ]));

    render(<ImportToolDialog {...defaultProps} />);

    // Navigate to step 2
    fireEvent.click(screen.getByText("empty-registry"));
    fireEvent.click(screen.getByRole("button", { name: /next/i }));

    expect(screen.getByText(/no tools discovered/i)).toBeInTheDocument();
  });

  it("should handle registry without namespace", async () => {
    const { useToolRegistries } = await import("@/hooks/use-tool-registries");
    vi.mocked(useToolRegistries).mockReturnValue(mockQueryResult([
      {
        apiVersion: "omnia.altairalabs.ai/v1alpha1",
        kind: "ToolRegistry",
        metadata: { name: "no-ns-registry" },
        spec: { handlers: [] },
        status: {
          phase: "Ready",
          discoveredToolsCount: 1,
          discoveredTools: [
            { name: "test-tool", handlerName: "test", description: "", endpoint: "http://localhost", status: "Available" },
          ],
        },
      },
    ]));

    const onImport = vi.fn().mockResolvedValue(undefined);
    render(<ImportToolDialog {...defaultProps} onImport={onImport} />);

    // Navigate to step 2, select and import
    fireEvent.click(screen.getByText("no-ns-registry"));
    fireEvent.click(screen.getByRole("button", { name: /next/i }));
    fireEvent.click(screen.getByText("test-tool"));
    fireEvent.click(screen.getByRole("button", { name: /import/i }));

    await waitFor(() => {
      expect(onImport).toHaveBeenCalled();
    });
  });

  it("should handle undefined registries data", async () => {
    const { useToolRegistries } = await import("@/hooks/use-tool-registries");
    vi.mocked(useToolRegistries).mockReturnValue(mockQueryResult(undefined));

    render(<ImportToolDialog {...defaultProps} />);

    expect(screen.getByText(/no tool registries found/i)).toBeInTheDocument();
  });
});
