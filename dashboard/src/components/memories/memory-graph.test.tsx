/**
 * Tests for MemoryGraph and related utilities.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { memoryToNode, MemoryGraph } from "./memory-graph";
import type { MemoryEntity } from "@/lib/data/types";

// @xyflow/react requires DOM measurement APIs not available in jsdom.
// We mock the entire module to test component rendering at the container level.
vi.mock("@xyflow/react", async () => {
  const actual = await vi.importActual<typeof import("@xyflow/react")>("@xyflow/react");
  return {
    ...actual,
    ReactFlow: ({ children }: { children?: React.ReactNode }) => (
      <div data-testid="react-flow">{children}</div>
    ),
    Background: () => <div data-testid="react-flow-background" />,
    Controls: () => <div data-testid="react-flow-controls" />,
    useNodesState: (initial: unknown[]) => [initial, vi.fn(), vi.fn()],
    useEdgesState: (initial: unknown[]) => [initial, vi.fn(), vi.fn()],
  };
});

// Mock elkjs — we don't need real force layout in unit tests.
vi.mock("elkjs/lib/elk.bundled.js", () => {
  const mockLayout = vi.fn().mockResolvedValue({
    children: [
      { id: "mem-1", x: 100, y: 200 },
      { id: "mem-2", x: 300, y: 400 },
    ],
  });
  class MockELK {
    layout = mockLayout;
  }
  return { default: MockELK };
});

function makeMemory(overrides: Partial<MemoryEntity> = {}): MemoryEntity {
  return {
    id: "mem-1",
    type: "fact",
    content: "User likes coffee",
    confidence: 0.8,
    scope: { workspace: "default" },
    createdAt: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

// ============================================================
// memoryToNode
// ============================================================

describe("memoryToNode", () => {
  it("uses memory.id as node id", () => {
    const node = memoryToNode(makeMemory({ id: "abc-123" }), 0);
    expect(node.id).toBe("abc-123");
  });

  it("falls back to index-based id when id is empty string", () => {
    const node = memoryToNode(makeMemory({ id: "" }), 3);
    expect(node.id).toBe("memory-3");
  });

  it("sets node type to 'memory'", () => {
    const node = memoryToNode(makeMemory(), 0);
    expect(node.type).toBe("memory");
  });

  it("initialises position at origin", () => {
    const node = memoryToNode(makeMemory(), 0);
    expect(node.position).toEqual({ x: 0, y: 0 });
  });

  it("scales size with confidence — low confidence gives smaller node", () => {
    const small = memoryToNode(makeMemory({ confidence: 0 }), 0);
    const large = memoryToNode(makeMemory({ confidence: 1 }), 1);
    expect(small.width).toBe(30);
    expect(large.width).toBe(80);
  });

  it("scales size with confidence — mid confidence is interpolated", () => {
    const node = memoryToNode(makeMemory({ confidence: 0.5 }), 0);
    expect(node.width).toBe(55);
  });

  it("falls back to 0.5 confidence when undefined", () => {
    const memory = makeMemory();
    // Simulate missing confidence via type cast
    const node = memoryToNode({ ...memory, confidence: undefined as unknown as number }, 0);
    expect(node.width).toBe(55);
  });

  it("truncates long content to 17 chars + ellipsis", () => {
    const node = memoryToNode(makeMemory({ content: "This is a very long memory content string" }), 0);
    expect((node.data as { label: string }).label).toBe("This is a very lo...");
  });

  it("leaves short content unchanged", () => {
    const node = memoryToNode(makeMemory({ content: "Short text" }), 0);
    expect((node.data as { label: string }).label).toBe("Short text");
  });

  it("reads category from metadata.consent_category", () => {
    const node = memoryToNode(
      makeMemory({ metadata: { consent_category: "memory:identity" } }),
      0
    );
    expect((node.data as { category: string }).category).toBe("memory:identity");
  });

  it("sets category to undefined when metadata is absent", () => {
    const node = memoryToNode(makeMemory({ metadata: undefined }), 0);
    expect((node.data as { category: unknown }).category).toBeUndefined();
  });

  it("stores memoryId in data", () => {
    const node = memoryToNode(makeMemory({ id: "xyz-999" }), 0);
    expect((node.data as { memoryId: string }).memoryId).toBe("xyz-999");
  });
});

// ============================================================
// MemoryGraph component
// ============================================================

describe("MemoryGraph", () => {
  const onNodeClick = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders the container with data-testid", () => {
    render(<MemoryGraph memories={[]} onNodeClick={onNodeClick} />);
    expect(screen.getByTestId("memory-graph")).toBeInTheDocument();
  });

  it("shows loading state initially", () => {
    render(<MemoryGraph memories={[makeMemory()]} onNodeClick={onNodeClick} />);
    expect(screen.getByText("Arranging memories...")).toBeInTheDocument();
  });

  it("renders ReactFlow after layout resolves", async () => {
    render(<MemoryGraph memories={[makeMemory()]} onNodeClick={onNodeClick} />);
    await waitFor(() => expect(screen.getByTestId("react-flow")).toBeInTheDocument());
  });

  it("renders without error when memories array is empty", async () => {
    render(<MemoryGraph memories={[]} onNodeClick={onNodeClick} />);
    // Empty memories: layoutNodes returns immediately with [], so loading clears
    await waitFor(() => expect(screen.queryByText("Arranging memories...")).not.toBeInTheDocument());
    expect(screen.getByTestId("memory-graph")).toBeInTheDocument();
  });

  it("renders ReactFlow for multiple memories", async () => {
    const memories = [
      makeMemory({ id: "mem-1", content: "First memory" }),
      makeMemory({ id: "mem-2", content: "Second memory" }),
    ];

    render(<MemoryGraph memories={memories} onNodeClick={onNodeClick} />);
    await waitFor(() => expect(screen.getByTestId("react-flow")).toBeInTheDocument());
  });
});
