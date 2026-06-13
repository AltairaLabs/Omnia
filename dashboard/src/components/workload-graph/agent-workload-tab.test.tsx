import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";

vi.mock("@/hooks/resources", () => ({
  usePromptPackContent: () => ({
    data: {
      id: "p",
      prompts: { triage: { id: "triage", name: "Triage", system_template: "t", tools: ["lookup"] } },
      tools: { lookup: { name: "lookup" } },
      workflow: { entry: "triage", states: { triage: { prompt_task: "triage", terminal: true } } },
    },
    isLoading: false,
  }),
  useProviders: () => ({
    data: [{ metadata: { name: "default" }, spec: { type: "claude", model: "claude-opus-4-8" } }],
  }),
  useToolRegistry: () => ({
    data: { status: { discoveredTools: [{ name: "lookup", handlerName: "http", endpoint: "https://x", status: "Available" }] } },
  }),
  usePromptPack: () => ({
    data: { spec: { skills: [{ source: "anthropic-skills", mountAs: "skills" }] } },
  }),
}));

vi.mock("@/hooks/use-skill-sources", () => ({
  useSkillSources: () => ({
    sources: [
      { metadata: { name: "anthropic-skills" }, status: { phase: "Ready", skillCount: 7 } },
    ],
  }),
}));

vi.mock("./WorkloadGraph", () => ({
  WorkloadGraph: ({ model }: { model: { altitude: string } }) => (
    <div data-testid="wg">{model.altitude}</div>
  ),
}));

import { AgentWorkloadTab } from "./agent-workload-tab";

const agent = {
  metadata: { name: "a", namespace: "demo" },
  spec: {
    promptPackRef: { name: "p" },
    providers: [{ name: "default", providerRef: { name: "default" } }],
    toolRegistryRef: { name: "tr" },
  },
} as never;

describe("AgentWorkloadTab", () => {
  it("renders the resolved (deployment) workload graph", () => {
    render(<AgentWorkloadTab agent={agent} workspace="demo" />);
    expect(screen.getByTestId("wg")).toHaveTextContent("deployment");
  });
});
