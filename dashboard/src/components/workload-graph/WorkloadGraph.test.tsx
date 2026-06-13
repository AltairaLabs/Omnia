import type { ReactNode } from "react";
import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import type { WorkloadModel } from "./types";

const fitViewSpy = vi.fn();

vi.mock("@xyflow/react", () => ({
  __esModule: true,
  ReactFlow: ({
    children,
    onInit,
  }: {
    children?: ReactNode;
    onInit?: (inst: { fitView: typeof fitViewSpy }) => void;
  }) => {
    onInit?.({ fitView: fitViewSpy });
    return <div data-testid="rf">{children}</div>;
  },
  Background: () => null,
  Controls: () => null,
  Panel: ({ children }: { children?: ReactNode }) => <div>{children}</div>,
  BackgroundVariant: { Dots: "dots" },
  MarkerType: { ArrowClosed: "arrowclosed" },
  useNodesState: (init: unknown[]) => [init, vi.fn(), vi.fn()],
  useEdgesState: (init: unknown[]) => [init, vi.fn(), vi.fn()],
}));

import { WorkloadGraph, fitViewAfterPaint } from "./WorkloadGraph";

const empty: WorkloadModel = {
  tier: "single", altitude: "definition", nodes: [], edges: [],
  meta: { counts: { agents: 0, tools: 0, skills: 0, states: 0 } },
};

const deployed: WorkloadModel = {
  tier: "workflow", altitude: "deployment",
  nodes: [{ id: "a", kind: "state", label: "A", badges: [], detail: {} }],
  edges: [],
  meta: {
    counts: { agents: 1, tools: 0, skills: 0, states: 1 },
    budget: { maxTotalVisits: 12, maxToolCalls: 30, maxWallTimeSec: 60 },
    binding: { providers: [{ name: "default", model: "claude-opus-4-8" }] },
  },
};

describe("WorkloadGraph", () => {
  it("shows an empty state when there are no nodes", () => {
    render(<WorkloadGraph model={empty} />);
    expect(screen.getByText(/no workload/i)).toBeInTheDocument();
  });

  it("shows the deployment banner and budget footer", () => {
    render(<WorkloadGraph model={deployed} />);
    expect(screen.getByText(/deployment/i)).toBeInTheDocument();
    expect(screen.getByText(/claude-opus-4-8/)).toBeInTheDocument();
    expect(screen.getByText(/≤12 visits/)).toBeInTheDocument();
  });
});

describe("fitViewAfterPaint", () => {
  it("is a no-op when no instance is provided", () => {
    expect(() => fitViewAfterPaint(null)).not.toThrow();
  });

  it("fits the view on the next paint frame", () => {
    const raf = vi
      .spyOn(globalThis, "requestAnimationFrame")
      .mockImplementation((cb: FrameRequestCallback) => {
        cb(0);
        return 0;
      });
    const fitView = vi.fn();
    fitViewAfterPaint({ fitView } as unknown as Parameters<typeof fitViewAfterPaint>[0]);
    expect(fitView).toHaveBeenCalledWith({ padding: 0.08, duration: 250 });
    raf.mockRestore();
  });
});
