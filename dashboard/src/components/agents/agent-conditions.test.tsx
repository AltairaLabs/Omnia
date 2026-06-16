import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { AgentConditions } from "./agent-conditions";
import type { Condition } from "@/types/common";

const conditions: Condition[] = [
  { type: "Ready", status: "True", reason: "Reconciled", message: "All good" },
  { type: "Available", status: "False", reason: "Scaling", message: "0/2 ready" },
];

describe("AgentConditions", () => {
  it("renders a row for every condition with type, reason and message", () => {
    render(<AgentConditions conditions={conditions} />);
    expect(screen.getByText("Ready")).toBeInTheDocument();
    expect(screen.getByText("Available")).toBeInTheDocument();
    expect(screen.getByText("Reconciled")).toBeInTheDocument();
    expect(screen.getByText("Scaling")).toBeInTheDocument();
    expect(screen.getByText("All good")).toBeInTheDocument();
    expect(screen.getByText("0/2 ready")).toBeInTheDocument();
  });

  it("keeps all conditions, not only the failing ones", () => {
    render(<AgentConditions conditions={conditions} />);
    // both a healthy (True) and a failing (False) condition are present
    expect(screen.getByText("Ready")).toBeInTheDocument();
    expect(screen.getByText("Available")).toBeInTheDocument();
  });

  it("falls back to an em dash when a condition has no message", () => {
    render(
      <AgentConditions
        conditions={[{ type: "Synced", status: "True", reason: "OK" }]}
      />,
    );
    expect(screen.getByText("—")).toBeInTheDocument();
  });

  it("renders nothing when there are no conditions", () => {
    const { container } = render(<AgentConditions conditions={[]} />);
    expect(container).toBeEmptyDOMElement();
  });

  it("renders nothing when conditions is undefined", () => {
    const { container } = render(<AgentConditions conditions={undefined} />);
    expect(container).toBeEmptyDOMElement();
  });
});
