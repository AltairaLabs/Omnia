import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { ReactFlowProvider } from "@xyflow/react";
import { PromptPackNodeComponent } from "./nodes";

describe("PromptPackNodeComponent tier badge", () => {
  it("shows the crew tier badge with agent count when tier data is present", () => {
    render(
      <ReactFlowProvider>
        <PromptPackNodeComponent data={{ label: "refunds", namespace: "demo", tier: "crew", agentCount: 3 }} />
      </ReactFlowProvider>
    );
    expect(screen.getByText(/crew/i)).toBeInTheDocument();
    expect(screen.getByText(/3/)).toBeInTheDocument();
  });

  it("omits the badge when no tier is present", () => {
    render(
      <ReactFlowProvider>
        <PromptPackNodeComponent data={{ label: "refunds", namespace: "demo" }} />
      </ReactFlowProvider>
    );
    expect(screen.queryByText(/crew|flow|solo/i)).not.toBeInTheDocument();
  });
});
