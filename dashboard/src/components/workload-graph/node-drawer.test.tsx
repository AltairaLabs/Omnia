import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import type { WorkloadNode } from "./types";

vi.mock("@/hooks/use-skill-source-content", () => ({
  useSkillSourceContent: () => ({
    // nested like the real synced tree: skills live under a category directory
    tree: [
      {
        name: "document",
        path: "document",
        isDirectory: true,
        children: [
          { name: "pdf", path: "document/pdf", isDirectory: true, children: [{ name: "SKILL.md", path: "document/pdf/SKILL.md", isDirectory: false }] },
          { name: "docx", path: "document/docx", isDirectory: true, children: [{ name: "SKILL.md", path: "document/docx/SKILL.md", isDirectory: false }] },
        ],
      },
      { name: "README.md", path: "README.md", isDirectory: false },
    ],
    fileCount: 2,
    directoryCount: 2,
    loading: false,
    error: null,
    refetch: vi.fn(),
  }),
}));

import { NodeDrawer } from "./node-drawer";

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

  it("renders composition step detail: kind, prompt task, predicate, termination, tool args, evals", () => {
    const stepNode: WorkloadNode = {
      id: "s", kind: "stepAgent", label: "synth", badges: [],
      detail: {
        stepKind: "agent", promptTask: "analyzer", termination: "≤10 steps",
        toolRef: "doc.parse", args: { content: "${input.text}" },
        predicateText: "${x} equals y", evals: ["quality"],
      },
    };
    render(<NodeDrawer node={stepNode} onClose={vi.fn()} />);
    expect(screen.getByText("agent")).toBeInTheDocument();
    expect(screen.getByText("analyzer")).toBeInTheDocument();
    expect(screen.getByText("≤10 steps")).toBeInTheDocument();
    expect(screen.getByText("doc.parse")).toBeInTheDocument();
    expect(screen.getByText(/quality/)).toBeInTheDocument();
  });

  it("shows SkillSource detail with a deep link to the Skills explorer", () => {
    const skillNode: WorkloadNode = {
      id: "skill:anthropic", kind: "skill", label: "anthropic", resolution: "resolved", badges: [],
      detail: { skillSource: "anthropic", mountAs: "skills", include: ["report/*"], skillCount: 12, skillPhase: "Ready" },
    };
    render(<NodeDrawer node={skillNode} onClose={vi.fn()} namespace="dev-agents" />);
    expect(screen.getByText(/Ready/)).toBeInTheDocument();
    // the actual skills are listed (from the synced content tree), not just a count
    expect(screen.getByText("Skills (2)")).toBeInTheDocument();
    expect(screen.getByText("pdf")).toBeInTheDocument();
    expect(screen.getByText("docx")).toBeInTheDocument();
    const link = screen.getByRole("link", { name: /Skills explorer/i });
    expect(link).toHaveAttribute("href", "/skills/anthropic?namespace=dev-agents");
  });

  it("shows variable detail for a variable node", () => {
    const node: WorkloadNode = {
      id: "var:topic", kind: "variable", label: "topic", badges: [],
      detail: { varType: "string", required: true, example: "AI" },
    };
    render(<NodeDrawer node={node} onClose={vi.fn()} />);
    expect(screen.getByText(/string/)).toBeInTheDocument();
    expect(screen.getByText(/required/i)).toBeInTheDocument();
    expect(screen.getByText(/AI/)).toBeInTheDocument();
  });

  it("shows producers and consumers for an artifact node", () => {
    const node: WorkloadNode = {
      id: "artifact:notes", kind: "artifact", label: "notes", badges: [],
      detail: { artifactMode: "append", producers: ["gather"], consumers: ["answer"] },
    };
    render(<NodeDrawer node={node} onClose={vi.fn()} />);
    expect(screen.getByText(/append/)).toBeInTheDocument();
    expect(screen.getByText(/gather/)).toBeInTheDocument();
    expect(screen.getByText(/answer/)).toBeInTheDocument();
  });

  it("shows pricing for a provider node", () => {
    const provider: WorkloadNode = {
      id: "provider:gpt", kind: "provider", label: "gpt", badges: [],
      detail: { model: "gpt-4o", providerType: "openai", pricing: { inputPer1kTokens: 0.01, outputPer1kTokens: 0.03 } },
    };
    render(<NodeDrawer node={provider} onClose={vi.fn()} />);
    expect(screen.getByText(/0.01/)).toBeInTheDocument();
    expect(screen.getByText(/0.03/)).toBeInTheDocument();
  });

  it("lists scenarios for a scenario group node", () => {
    const scenarios: WorkloadNode = {
      id: "scenarios", kind: "scenario", label: "2 scenarios", badges: [],
      detail: { scenarios: [{ id: "qa", turnCount: 2, tags: ["smoke"] }, { id: "edge" }] },
    };
    render(<NodeDrawer node={scenarios} onClose={vi.fn()} />);
    expect(screen.getByText("qa")).toBeInTheDocument();
    expect(screen.getByText("edge")).toBeInTheDocument();
    expect(screen.getByText(/smoke/)).toBeInTheDocument();
  });

  it("shows the judge provider and persona role/provider", () => {
    const judge: WorkloadNode = { id: "judge:r", kind: "judge", label: "r", badges: [], detail: { judgeProvider: "judge-gpt" } };
    const { rerender } = render(<NodeDrawer node={judge} onClose={vi.fn()} />);
    expect(screen.getByText(/judge-gpt/)).toBeInTheDocument();
    const persona: WorkloadNode = { id: "persona:u", kind: "persona", label: "u", badges: [], detail: { persona: { id: "u", role: "user", provider: "selfplay" } } };
    rerender(<NodeDrawer node={persona} onClose={vi.fn()} />);
    expect(screen.getByText(/selfplay/)).toBeInTheDocument();
    expect(screen.getByText(/user/)).toBeInTheDocument();
  });
});
