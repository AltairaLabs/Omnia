import type { ReactNode } from "react";
import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import type { WorkloadModel } from "./types";

vi.mock("@xyflow/react", () => ({
  __esModule: true,
  ReactFlow: ({ children }: { children?: ReactNode }) => <div data-testid="rf">{children}</div>,
  Background: () => null,
  Controls: () => null,
  BackgroundVariant: { Dots: "dots" },
  useNodesState: (init: unknown[]) => [init, vi.fn(), vi.fn()],
  useEdgesState: (init: unknown[]) => [init, vi.fn(), vi.fn()],
}));

import { WorkloadGraph } from "./WorkloadGraph";

const empty: WorkloadModel = {
  tier: "solo", altitude: "definition", nodes: [], edges: [],
  meta: { counts: { agents: 0, tools: 0, skills: 0, states: 0 } },
};

const deployed: WorkloadModel = {
  tier: "flow", altitude: "deployment",
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
