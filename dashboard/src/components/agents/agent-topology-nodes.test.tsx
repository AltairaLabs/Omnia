import type { ReactNode } from "react";
import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { ReactFlowProvider } from "@xyflow/react";
import {
  AgentFacadeNode,
  AgentRuntimeNode,
  AgentPromptPackNode,
  AgentSessionNode,
  AgentMemoryNode,
} from "./agent-topology-nodes";

function wrap(ui: ReactNode) {
  return render(<ReactFlowProvider>{ui}</ReactFlowProvider>);
}

describe("agent topology nodes", () => {
  it("facade node shows the label, type and port", () => {
    wrap(<AgentFacadeNode data={{ facadeType: "websocket", port: 8080 }} />);
    expect(screen.getByText("Facade")).toBeInTheDocument();
    expect(screen.getByText("websocket")).toBeInTheDocument();
    expect(screen.getByText(/8080/)).toBeInTheDocument();
  });

  it("runtime node shows the label and the framework pill (logo + label) with version", () => {
    wrap(<AgentRuntimeNode data={{ frameworkType: "promptkit", frameworkVersion: "1.4.14" }} />);
    expect(screen.getByText("Runtime")).toBeInTheDocument();
    // Reuses FrameworkBadge from the agents list — renders the styled "PromptKit" pill.
    expect(screen.getByText("PromptKit")).toBeInTheDocument();
    expect(screen.getByText(/1\.4\.14/)).toBeInTheDocument();
  });

  it("promptpack node shows name + version and is not a button", () => {
    wrap(<AgentPromptPackNode data={{ name: "echo", version: "v3" }} />);
    expect(screen.getByText("echo")).toBeInTheDocument();
    expect(screen.getByText(/v3/)).toBeInTheDocument();
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });

  it("session node shows type and ttl", () => {
    wrap(<AgentSessionNode data={{ sessionType: "memory", ttl: "1h" }} />);
    expect(screen.getByText(/memory/)).toBeInTheDocument();
    expect(screen.getByText(/1h/)).toBeInTheDocument();
  });

  it("memory node shows On with a deep-link when enabled", () => {
    wrap(<AgentMemoryNode data={{ enabled: true, agentName: "echo" }} />);
    const link = screen.getByRole("link", { name: /on/i });
    expect(link).toHaveAttribute("href", "/memory-analytics?agent=echo");
  });

  it("memory node shows Off with no link when disabled", () => {
    wrap(<AgentMemoryNode data={{ enabled: false, agentName: "echo" }} />);
    expect(screen.getByText(/off/i)).toBeInTheDocument();
    expect(screen.queryByRole("link")).not.toBeInTheDocument();
  });
});
