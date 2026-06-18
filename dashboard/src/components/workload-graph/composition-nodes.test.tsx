import type { ReactNode } from "react";
import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { ReactFlowProvider } from "@xyflow/react";
import { CompositionContainerNode, CompositionStepNode, CompositionParallelNode } from "./composition-nodes";
import type { WorkloadNode } from "./types";

const wrap = (ui: ReactNode) => render(<ReactFlowProvider>{ui}</ReactFlowProvider>);

describe("composition nodes", () => {
  it("renders a container with name + composition badge and fires collapse", () => {
    const node: WorkloadNode = { id: "main", kind: "composition", label: "main", isContainer: true, badges: [], detail: { compositionName: "analyze", stepCount: 3 } };
    const onToggle = vi.fn();
    wrap(<CompositionContainerNode data={{ node, onToggle, expanded: true }} />);
    expect(screen.getByText("analyze")).toBeInTheDocument();
    expect(screen.getByText(/composition/i)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /collapse/i }));
    expect(onToggle).toHaveBeenCalledWith("main");
  });

  it("renders a branch step with its predicate and an agent step with termination", () => {
    const branch: WorkloadNode = { id: "r", kind: "stepBranch", label: "route", badges: [], detail: { stepKind: "branch", predicateText: "${x} equals y" } };
    const agent: WorkloadNode = { id: "s", kind: "stepAgent", label: "synth", badges: [], detail: { stepKind: "agent", termination: "≤10 steps" } };
    wrap(<CompositionStepNode data={{ node: branch }} />);
    wrap(<CompositionStepNode data={{ node: agent }} />);
    expect(screen.getByText("route")).toBeInTheDocument();
    expect(screen.getByText("${x} equals y")).toBeInTheDocument();
    expect(screen.getByText("≤10 steps")).toBeInTheDocument();
  });

  it("renders a parallel frame labelled with its reducer", () => {
    const node: WorkloadNode = { id: "p", kind: "stepParallel", label: "meta", isContainer: true, badges: [], detail: { stepKind: "parallel", reducer: "barrier → metadata" } };
    wrap(<CompositionParallelNode data={{ node }} />);
    expect(screen.getByText("barrier → metadata")).toBeInTheDocument();
  });
});
