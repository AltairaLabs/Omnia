import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { ArenaWorkloadView } from "./arena-workload-view";
import type { WorkloadModel } from "@/components/workload-graph";

const mockState = { model: null as WorkloadModel | null, loading: false, parseError: null as string | null };
vi.mock("@/hooks/arena", () => ({ useArenaWorkloadModel: () => mockState }));
vi.mock("@/components/workload-graph", async (orig) => ({
  ...(await orig<typeof import("@/components/workload-graph")>()),
  WorkloadGraph: ({ model }: { model: WorkloadModel }) => <div data-testid="graph">{model.altitude}</div>,
}));

describe("ArenaWorkloadView", () => {
  it("shows a loading state", () => {
    mockState.model = null;
    mockState.loading = true;
    mockState.parseError = null;
    render(<ArenaWorkloadView projectId="p" />);
    expect(screen.getByText(/Loading workload/i)).toBeInTheDocument();
  });

  it("shows an empty state when there is no workload", () => {
    mockState.model = { tier: "single", altitude: "test", nodes: [], edges: [], meta: { counts: { agents: 0, tools: 0, skills: 0, states: 0 } } };
    mockState.loading = false;
    mockState.parseError = null;
    render(<ArenaWorkloadView projectId="p" />);
    expect(screen.getByText(/No workload to show/i)).toBeInTheDocument();
  });

  it("renders the graph and a parse-error hint", () => {
    mockState.model = { tier: "single", altitude: "test", nodes: [{ id: "a", kind: "agent", label: "A", badges: [], detail: {} }], edges: [], meta: { counts: { agents: 1, tools: 0, skills: 0, states: 0 } } };
    mockState.loading = false;
    mockState.parseError = "Could not parse arena config";
    render(<ArenaWorkloadView projectId="p" />);
    expect(screen.getByTestId("graph")).toBeInTheDocument();
    expect(screen.getByText(/Could not parse/)).toBeInTheDocument();
  });
});
