import type { ReactNode } from "react";
import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { ReactFlowProvider } from "@xyflow/react";
import { WorkloadAgentNode, WorkloadProviderNode, WorkloadSkillNode, WorkloadToolNode } from "./workload-nodes";
import type { WorkloadNode } from "./types";

function wrap(ui: ReactNode) {
  return render(<ReactFlowProvider>{ui}</ReactFlowProvider>);
}

describe("workload nodes", () => {
  it("renders a tool-registry node with its name and tool count, and fires onClick", () => {
    const node: WorkloadNode = {
      id: "toolregistry",
      kind: "tool",
      label: "demo-tools",
      badges: [{ icon: "tool", label: "2" }],
      detail: { tools: [{ name: "lookup" }, { name: "refund" }] },
    };
    const onClick = vi.fn();
    wrap(<WorkloadToolNode data={{ node, onClick }} />);
    expect(screen.getByText("demo-tools")).toBeInTheDocument();
    expect(screen.getByText(/2 tools/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button"));
    expect(onClick).toHaveBeenCalledWith("toolregistry");
  });

  it("renders an agent node with entry badge and fires onClick", () => {
    const node: WorkloadNode = {
      id: "a", kind: "agent", label: "Triage", isEntry: true,
      badges: [{ icon: "tool", label: "3" }, { icon: "skill", label: "1" }], detail: {},
    };
    const onClick = vi.fn();
    wrap(<WorkloadAgentNode data={{ node, onClick }} />);
    expect(screen.getByText("Triage")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button"));
    expect(onClick).toHaveBeenCalledWith("a");
  });

  it("renders a provider node with model", () => {
    const provider: WorkloadNode = {
      id: "p", kind: "provider", label: "default", badges: [{ label: "llm" }],
      detail: { model: "claude-opus-4-8" },
    };
    wrap(<WorkloadProviderNode data={{ node: provider }} />);
    expect(screen.getByText("claude-opus-4-8")).toBeInTheDocument();
  });

  it("renders a skill node with phase and mountAs, and fires onClick", () => {
    const skill: WorkloadNode = {
      id: "skill:anthropic", kind: "skill", label: "anthropic", resolution: "resolved",
      badges: [{ icon: "skill", label: "Ready · 12" }],
      detail: { skillSource: "anthropic", mountAs: "skills", skillPhase: "Ready" },
    };
    const onClick = vi.fn();
    wrap(<WorkloadSkillNode data={{ node: skill, onClick }} />);
    expect(screen.getByText("anthropic")).toBeInTheDocument();
    expect(screen.getByText("Ready · 12")).toBeInTheDocument();
    expect(screen.getByText(/skills/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button"));
    expect(onClick).toHaveBeenCalledWith("skill:anthropic");
  });
});
