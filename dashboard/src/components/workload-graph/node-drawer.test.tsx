import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { NodeDrawer } from "./node-drawer";
import type { WorkloadNode } from "./types";

const node: WorkloadNode = {
  id: "triage", kind: "state", label: "Triage", badges: [],
  detail: {
    description: "Triages requests",
    systemTemplatePreview: "You are a triage agent.",
    tools: [{ name: "lookup", endpoint: "https://x", status: "resolved" }, { name: "ghost", status: "unavailable" }],
    skills: ["billing"],
  },
};

describe("NodeDrawer", () => {
  it("renders nothing when no node is selected", () => {
    const { container } = render(<NodeDrawer node={undefined} onClose={vi.fn()} />);
    expect(container).toBeEmptyDOMElement();
  });

  it("shows the node's prompt preview, tools with resolution, and skills", () => {
    render(<NodeDrawer node={node} onClose={vi.fn()} />);
    expect(screen.getByText("Triage")).toBeInTheDocument();
    expect(screen.getByText("You are a triage agent.")).toBeInTheDocument();
    expect(screen.getByText("lookup")).toBeInTheDocument();
    expect(screen.getByText("ghost")).toBeInTheDocument();
    expect(screen.getByText("unavailable")).toBeInTheDocument();
    expect(screen.getByText("billing")).toBeInTheDocument();
  });
});
