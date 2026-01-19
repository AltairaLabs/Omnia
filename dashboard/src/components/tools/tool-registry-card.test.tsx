/**
 * Tests for ToolRegistryCard component.
 */

import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { ToolRegistryCard } from "./tool-registry-card";
import type { ToolRegistry } from "@/types";

// Mock next/link to avoid router issues in tests
vi.mock("next/link", () => ({
  default: ({ children, href }: { children: React.ReactNode; href: string }) => (
    <a href={href}>{children}</a>
  ),
}));

const mockRegistry: ToolRegistry = {
  apiVersion: "omnia.altairalabs.ai/v1alpha1",
  kind: "ToolRegistry",
  metadata: {
    name: "test-registry",
    namespace: "test-namespace",
    uid: "test-uid",
  },
  spec: {
    handlers: [
      { type: "http", name: "handler-1" },
      { type: "grpc", name: "handler-2" },
    ],
  },
  status: {
    phase: "Ready",
    discoveredToolsCount: 5,
    discoveredTools: [
      { name: "tool-1", handlerName: "handler-1", description: "Tool 1", endpoint: "http://localhost:8080/tool-1", status: "Available" },
      { name: "tool-2", handlerName: "handler-1", description: "Tool 2", endpoint: "http://localhost:8080/tool-2", status: "Available" },
      { name: "tool-3", handlerName: "handler-2", description: "Tool 3", endpoint: "http://localhost:8080/tool-3", status: "Unavailable" },
    ],
    lastDiscoveryTime: new Date().toISOString(),
  },
};

describe("ToolRegistryCard", () => {
  it("renders registry name and namespace", () => {
    render(<ToolRegistryCard registry={mockRegistry} />);

    expect(screen.getByText("test-registry")).toBeInTheDocument();
    expect(screen.getByText("test-namespace")).toBeInTheDocument();
  });

  it("renders tool count summary", () => {
    render(<ToolRegistryCard registry={mockRegistry} />);

    expect(screen.getByText("2/5 available")).toBeInTheDocument();
  });

  it("renders handler types", () => {
    render(<ToolRegistryCard registry={mockRegistry} />);

    expect(screen.getByText("http")).toBeInTheDocument();
    expect(screen.getByText("grpc")).toBeInTheDocument();
  });

  it("renders status badge", () => {
    render(<ToolRegistryCard registry={mockRegistry} />);

    expect(screen.getByTestId("status-badge")).toHaveTextContent("Ready");
  });

  it("renders shared badge when isShared is true", () => {
    render(<ToolRegistryCard registry={mockRegistry} isShared={true} />);

    expect(screen.getByTestId("shared-badge")).toBeInTheDocument();
    expect(screen.getByText("Shared")).toBeInTheDocument();
  });

  it("does not render shared badge when isShared is false", () => {
    render(<ToolRegistryCard registry={mockRegistry} isShared={false} />);

    expect(screen.queryByTestId("shared-badge")).not.toBeInTheDocument();
  });

  it("does not render shared badge when isShared is not provided", () => {
    render(<ToolRegistryCard registry={mockRegistry} />);

    expect(screen.queryByTestId("shared-badge")).not.toBeInTheDocument();
  });

  it("renders tool list preview", () => {
    render(<ToolRegistryCard registry={mockRegistry} />);

    expect(screen.getByText("tool-1")).toBeInTheDocument();
    expect(screen.getByText("tool-2")).toBeInTheDocument();
    expect(screen.getByText("tool-3")).toBeInTheDocument();
  });

  it("links to correct detail page", () => {
    render(<ToolRegistryCard registry={mockRegistry} />);

    const link = screen.getByRole("link");
    expect(link).toHaveAttribute("href", "/tools/test-registry?namespace=test-namespace");
  });

  it("renders mcp handler type", () => {
    const registryWithMcp: ToolRegistry = {
      ...mockRegistry,
      spec: {
        handlers: [{ type: "mcp", name: "mcp-handler" }],
      },
    };
    render(<ToolRegistryCard registry={registryWithMcp} />);

    expect(screen.getByText("mcp")).toBeInTheDocument();
  });

  it("renders openapi handler type", () => {
    const registryWithOpenApi: ToolRegistry = {
      ...mockRegistry,
      spec: {
        handlers: [{ type: "openapi", name: "openapi-handler" }],
      },
    };
    render(<ToolRegistryCard registry={registryWithOpenApi} />);

    expect(screen.getByText("openapi")).toBeInTheDocument();
  });

  it("renders default handler type for unknown types", () => {
    const registryWithUnknown: ToolRegistry = {
      ...mockRegistry,
      spec: {
        handlers: [{ type: "custom" as "http", name: "custom-handler" }],
      },
    };
    render(<ToolRegistryCard registry={registryWithUnknown} />);

    expect(screen.getByText("custom")).toBeInTheDocument();
  });

  it("renders Unknown status icon for tools with unknown status", () => {
    const registryWithUnknownStatus: ToolRegistry = {
      ...mockRegistry,
      status: {
        ...mockRegistry.status,
        discoveredTools: [
          { name: "unknown-tool", handlerName: "handler-1", description: "Unknown", endpoint: "http://localhost:8080/unknown", status: "Unknown" },
        ],
      },
    };
    render(<ToolRegistryCard registry={registryWithUnknownStatus} />);

    expect(screen.getByText("unknown-tool")).toBeInTheDocument();
  });

  it("formats time in hours when between 1-24 hours ago", () => {
    const twoHoursAgo = new Date(Date.now() - 2 * 60 * 60 * 1000).toISOString();
    const registryWithHoursAgo: ToolRegistry = {
      ...mockRegistry,
      status: {
        ...mockRegistry.status,
        lastDiscoveryTime: twoHoursAgo,
      },
    };
    render(<ToolRegistryCard registry={registryWithHoursAgo} />);

    expect(screen.getByText(/Discovered 2h ago/)).toBeInTheDocument();
  });

  it("formats time in days when over 24 hours ago", () => {
    const twoDaysAgo = new Date(Date.now() - 2 * 24 * 60 * 60 * 1000).toISOString();
    const registryWithDaysAgo: ToolRegistry = {
      ...mockRegistry,
      status: {
        ...mockRegistry.status,
        lastDiscoveryTime: twoDaysAgo,
      },
    };
    render(<ToolRegistryCard registry={registryWithDaysAgo} />);

    expect(screen.getByText(/Discovered 2d ago/)).toBeInTheDocument();
  });

  it("shows dash when lastDiscoveryTime is missing", () => {
    const registryNoDiscoveryTime: ToolRegistry = {
      ...mockRegistry,
      status: {
        ...mockRegistry.status,
        lastDiscoveryTime: undefined,
      },
    };
    render(<ToolRegistryCard registry={registryNoDiscoveryTime} />);

    expect(screen.getByText(/Discovered -/)).toBeInTheDocument();
  });

  it("shows '+N more' when there are more than 3 tools", () => {
    const registryWithManyTools: ToolRegistry = {
      ...mockRegistry,
      status: {
        ...mockRegistry.status,
        discoveredToolsCount: 6,
        discoveredTools: [
          { name: "tool-1", handlerName: "handler-1", description: "Tool 1", endpoint: "http://localhost:8080/tool-1", status: "Available" },
          { name: "tool-2", handlerName: "handler-1", description: "Tool 2", endpoint: "http://localhost:8080/tool-2", status: "Available" },
          { name: "tool-3", handlerName: "handler-1", description: "Tool 3", endpoint: "http://localhost:8080/tool-3", status: "Available" },
          { name: "tool-4", handlerName: "handler-1", description: "Tool 4", endpoint: "http://localhost:8080/tool-4", status: "Available" },
          { name: "tool-5", handlerName: "handler-1", description: "Tool 5", endpoint: "http://localhost:8080/tool-5", status: "Available" },
          { name: "tool-6", handlerName: "handler-1", description: "Tool 6", endpoint: "http://localhost:8080/tool-6", status: "Available" },
        ],
      },
    };
    render(<ToolRegistryCard registry={registryWithManyTools} />);

    expect(screen.getByText("+3 more")).toBeInTheDocument();
  });

  it("handles missing status gracefully", () => {
    const registryNoStatus: ToolRegistry = {
      ...mockRegistry,
      status: undefined,
    };
    render(<ToolRegistryCard registry={registryNoStatus} />);

    expect(screen.getByText("test-registry")).toBeInTheDocument();
    expect(screen.getByText("0/0 available")).toBeInTheDocument();
  });

  it("handles empty handlers array gracefully", () => {
    const registryNoHandlers: ToolRegistry = {
      ...mockRegistry,
      spec: {
        handlers: [],
      },
    };
    render(<ToolRegistryCard registry={registryNoHandlers} />);

    expect(screen.getByText("0 handlers")).toBeInTheDocument();
  });

  it("handles empty tools list", () => {
    const registryNoTools: ToolRegistry = {
      ...mockRegistry,
      status: {
        ...mockRegistry.status,
        discoveredTools: [],
        discoveredToolsCount: 0,
      },
    };
    render(<ToolRegistryCard registry={registryNoTools} />);

    expect(screen.getByText("0/0 available")).toBeInTheDocument();
  });
});
