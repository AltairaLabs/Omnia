import type { ReactNode } from "react";
import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { ReactFlowProvider } from "@xyflow/react";
import {
  WorkflowStateNode, InitialNode, FinalNode, VariableNode, ArtifactNode,
} from "./workflow-nodes";
import type { WorkloadNode } from "./types";

const wrap = (ui: ReactNode) => render(<ReactFlowProvider>{ui}</ReactFlowProvider>);

describe("workflow shape nodes", () => {
  it("renders a stadium state with tool/skill/loop badges and fires onClick", () => {
    const node: WorkloadNode = {
      id: "s", kind: "state", label: "Plan",
      badges: [{ icon: "tool", label: "2" }, { icon: "skill", label: "1" }, { icon: "loop", label: "≤3" }],
      detail: {},
    };
    const onClick = vi.fn();
    wrap(<WorkflowStateNode data={{ node, onClick }} />);
    expect(screen.getByText("Plan")).toBeInTheDocument();
    expect(screen.getByText("≤3")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button"));
    expect(onClick).toHaveBeenCalledWith("s");
  });

  it("renders initial and final markers", () => {
    wrap(<InitialNode data={{ node: { id: "i", kind: "initial", label: "", badges: [], detail: {} } }} />);
    wrap(<FinalNode data={{ node: { id: "f", kind: "final", label: "", badges: [], detail: {} } }} />);
    expect(screen.getByTestId("marker-initial")).toBeInTheDocument();
    expect(screen.getByTestId("marker-final")).toBeInTheDocument();
  });

  it("renders a variable lozenge and an artifact parallelogram, each clickable", () => {
    const onVar = vi.fn();
    const onArt = vi.fn();
    wrap(<VariableNode data={{ node: { id: "v", kind: "variable", label: "topic", badges: [], detail: { varType: "string" } }, onClick: onVar }} />);
    wrap(<ArtifactNode data={{ node: { id: "a", kind: "artifact", label: "notes", badges: [], detail: { artifactMode: "append" } }, onClick: onArt }} />);
    expect(screen.getByText("topic")).toBeInTheDocument();
    expect(screen.getByText("notes")).toBeInTheDocument();
    fireEvent.click(screen.getByText("topic"));
    fireEvent.click(screen.getByText("notes"));
    expect(onVar).toHaveBeenCalledWith("v");
    expect(onArt).toHaveBeenCalledWith("a");
  });

  it("mutes an unresolved artifact", () => {
    wrap(<ArtifactNode data={{ node: { id: "a", kind: "artifact", label: "ghost", resolution: "unresolved", badges: [], detail: {} } }} />);
    expect(screen.getByText("ghost")).toBeInTheDocument();
  });
});
