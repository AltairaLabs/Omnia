import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { AgentTable } from "./agent-table";
import { useAgentCost } from "@/hooks/agents";
import type { AgentRuntime } from "@/types";

vi.mock("@/hooks/agents", () => ({ useAgentCost: vi.fn(() => ({ data: null })) }));
vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: vi.fn(() => ({ currentWorkspace: { name: "demo" } })),
}));
vi.mock("next/link", () => ({
  default: ({ children, href }: { children: React.ReactNode; href: string }) => (
    <a href={href}>{children}</a>
  ),
}));

function mkAgent(name: string, ns: string): AgentRuntime {
  return {
    apiVersion: "omnia.altairalabs.ai/v1alpha1",
    kind: "AgentRuntime",
    metadata: { name, namespace: ns, uid: `uid-${name}`, creationTimestamp: "2024-01-15T10:00:00Z" },
    spec: {
      framework: { type: "promptkit" },
      providers: [{ name: "default", providerRef: { name: "openai-provider" } }],
    },
    status: { phase: "Running", replicas: { ready: 1, desired: 1 } },
  } as AgentRuntime;
}

describe("AgentTable", () => {
  it("renders a row per agent with a name link and namespace", () => {
    render(<AgentTable agents={[mkAgent("alpha", "ns-a")]} />);
    expect(screen.getByRole("link", { name: "alpha" })).toHaveAttribute(
      "href",
      "/agents/alpha?namespace=ns-a"
    );
    expect(screen.getByText("ns-a")).toBeInTheDocument();
  });

  it("queries cost by workspace name, not the agent namespace (#1572)", () => {
    render(<AgentTable agents={[mkAgent("alpha", "ns-a")]} />);
    // currentWorkspace.name = "demo" (mocked); must be the cost key, not "ns-a".
    expect(vi.mocked(useAgentCost)).toHaveBeenCalledWith("demo", "alpha");
  });
});
