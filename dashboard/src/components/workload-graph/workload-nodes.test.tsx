import type { ReactNode } from "react";
import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { ReactFlowProvider } from "@xyflow/react";
import { WorkloadAgentNode, WorkloadProviderNode } from "./workload-nodes";
import type { WorkloadNode } from "./types";

function wrap(ui: ReactNode) {
  return render(<ReactFlowProvider>{ui}</ReactFlowProvider>);
}

describe("workload nodes", () => {
  it("renders an agent node with entry badge and fires onClick", () => {
    const node: WorkloadNode = {
      id: "a", kind: "agent", label: "Triage", isEntry: true,
      badges: [{ icon: "tool", label: "3" }, { icon: "skill", label: "1" }], detail: {},
    };
    const onClick = vi.fn();
    wrap(<WorkloadAgentNode data={{ node, onClick }} />);
    expect(screen.getByText("Triage")).toBeInTheDocument();
    expect(screen.getByText("entry")).toBeInTheDocument();
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
});
