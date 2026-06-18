import type { ReactNode } from "react";
import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { ReactFlowProvider } from "@xyflow/react";
import {
  WorkflowStateNode, InitialNode, FinalNode, VariableNode, ArtifactNode,
  ScenarioGroupNode, JudgeNode, PersonaNode,
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

  it("shows an expand control for a composition state and fires onToggle without selecting", () => {
    const node: WorkloadNode = { id: "main", kind: "state", label: "main", badges: [{ label: "composition" }], detail: {} };
    const onClick = vi.fn();
    const onToggle = vi.fn();
    wrap(<WorkflowStateNode data={{ node, onClick, onToggle, expandable: true }} />);
    fireEvent.click(screen.getByRole("button", { name: /expand composition/i }));
    expect(onToggle).toHaveBeenCalledWith("main");
    expect(onClick).not.toHaveBeenCalled();
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

describe("arena harness nodes", () => {
  it("renders a scenario group with count and fires onClick", () => {
    const node: WorkloadNode = {
      id: "scenarios", kind: "scenario", label: "3 scenarios", badges: [],
      detail: { scenarios: [{ id: "a" }, { id: "b" }, { id: "c" }] },
    };
    const onClick = vi.fn();
    wrap(<ScenarioGroupNode data={{ node, onClick }} />);
    expect(screen.getByText("3 scenarios")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button"));
    expect(onClick).toHaveBeenCalledWith("scenarios");
  });

  it("renders a judge with its provider", () => {
    const node: WorkloadNode = {
      id: "judge:relevance", kind: "judge", label: "relevance", badges: [],
      detail: { judgeProvider: "gpt-4" },
    };
    wrap(<JudgeNode data={{ node, onClick: vi.fn() }} />);
    expect(screen.getByText("relevance")).toBeInTheDocument();
    expect(screen.getByText(/gpt-4/)).toBeInTheDocument();
  });

  it("renders a persona node", () => {
    const node: WorkloadNode = {
      id: "persona:sre", kind: "persona", label: "sre-user", badges: [],
      detail: { persona: { id: "sre-user", role: "sre-user" } },
    };
    wrap(<PersonaNode data={{ node, onClick: vi.fn() }} />);
    expect(screen.getByText("sre-user")).toBeInTheDocument();
  });
});
