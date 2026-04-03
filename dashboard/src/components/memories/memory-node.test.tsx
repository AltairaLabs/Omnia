/**
 * Tests for MemoryNode component.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryNode, type MemoryNodeData } from "./memory-node";
import type { NodeProps } from "@xyflow/react";

// Mock @xyflow/react — Handle requires a ReactFlow context.
vi.mock("@xyflow/react", () => ({
  Handle: () => <div data-testid="xyflow-handle" />,
  Position: { Right: "right", Left: "left" },
}));

function makeNodeProps(data: MemoryNodeData): NodeProps {
  return {
    id: "test-node",
    type: "memory",
    data,
    selected: false,
    selectable: true,
    draggable: true,
    dragging: false,
    deletable: true,
    isConnectable: true,
    zIndex: 0,
    positionAbsoluteX: 0,
    positionAbsoluteY: 0,
  } as NodeProps;
}

function makeData(overrides: Partial<MemoryNodeData> = {}): MemoryNodeData {
  return {
    label: "Test memory",
    category: "memory:identity",
    confidence: 0.8,
    memoryId: "mem-1",
    ...overrides,
  };
}

describe("MemoryNode", () => {
  it("renders with data-testid", () => {
    render(<MemoryNode {...makeNodeProps(makeData())} />);
    expect(screen.getByTestId("memory-node")).toBeInTheDocument();
  });

  it("displays short label unchanged", () => {
    render(<MemoryNode {...makeNodeProps(makeData({ label: "Short label" }))} />);
    expect(screen.getByText("Short label")).toBeInTheDocument();
  });

  it("truncates label longer than 15 characters", () => {
    render(<MemoryNode {...makeNodeProps(makeData({ label: "This is a long label" }))} />);
    expect(screen.getByText("This is a lo...")).toBeInTheDocument();
  });

  it("sets title to full label (not truncated)", () => {
    const label = "This is a long label";
    render(<MemoryNode {...makeNodeProps(makeData({ label }))} />);
    expect(screen.getByTestId("memory-node")).toHaveAttribute("title", label);
  });

  it("applies identity category color (blue)", () => {
    render(<MemoryNode {...makeNodeProps(makeData({ category: "memory:identity" }))} />);
    const node = screen.getByTestId("memory-node");
    expect(node).toHaveStyle({ backgroundColor: "#3b82f6" });
  });

  it("applies health category color (red)", () => {
    render(<MemoryNode {...makeNodeProps(makeData({ category: "memory:health" }))} />);
    const node = screen.getByTestId("memory-node");
    expect(node).toHaveStyle({ backgroundColor: "#ef4444" });
  });

  it("applies gray fallback for unknown category", () => {
    render(<MemoryNode {...makeNodeProps(makeData({ category: "memory:unknown" }))} />);
    const node = screen.getByTestId("memory-node");
    expect(node).toHaveStyle({ backgroundColor: "#6b7280" });
  });

  it("applies gray fallback when category is undefined", () => {
    render(<MemoryNode {...makeNodeProps(makeData({ category: undefined }))} />);
    const node = screen.getByTestId("memory-node");
    expect(node).toHaveStyle({ backgroundColor: "#6b7280" });
  });

  it("scales size up with higher confidence", () => {
    render(<MemoryNode {...makeNodeProps(makeData({ confidence: 1.0 }))} />);
    const node = screen.getByTestId("memory-node");
    expect(node).toHaveStyle({ width: "80px", height: "80px" });
  });

  it("scales size down with lower confidence", () => {
    render(<MemoryNode {...makeNodeProps(makeData({ confidence: 0 }))} />);
    const node = screen.getByTestId("memory-node");
    expect(node).toHaveStyle({ width: "30px", height: "30px" });
  });

  it("renders source and target handles", () => {
    render(<MemoryNode {...makeNodeProps(makeData())} />);
    const handles = screen.getAllByTestId("xyflow-handle");
    expect(handles).toHaveLength(2);
  });
});
