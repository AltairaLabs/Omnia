import { Suspense } from "react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, act } from "@testing-library/react";
import PromptPackDetailPage from "./page";

const usePromptPackSpy = vi.hoisted(() => vi.fn());
const usePromptPackContentSpy = vi.hoisted(() => vi.fn());
const useWorkspacesSpy = vi.hoisted(() => vi.fn());
const useAgentsSpy = vi.hoisted(() => vi.fn());

vi.mock("next/navigation", () => ({
  useSearchParams: () => ({ get: (k: string) => (k === "namespace" ? "production" : null) }),
}));

vi.mock("@/hooks/resources", () => ({
  usePromptPack: usePromptPackSpy,
  usePromptPackContent: usePromptPackContentSpy,
  useWorkspaces: useWorkspacesSpy,
}));

vi.mock("@/hooks/agents", () => ({
  useAgents: useAgentsSpy,
}));

vi.mock("@/hooks/use-skill-sources", () => ({
  useSkillSources: () => ({ sources: [] }),
}));

vi.mock("@/components/layout", () => ({
  Header: ({ title }: { title: React.ReactNode }) => <h1>{title}</h1>,
}));

vi.mock("@/components/workload-graph", async () => {
  const actual = await vi.importActual<typeof import("@/components/workload-graph")>(
    "@/components/workload-graph",
  );
  return { ...actual, WorkloadGraph: () => <div data-testid="workload-graph" /> };
});

beforeEach(() => {
  useWorkspacesSpy.mockReturnValue({ data: [] });
  useAgentsSpy.mockReturnValue({ data: [] });
  usePromptPackContentSpy.mockReturnValue({
    data: { id: "p", prompts: { main: { id: "main", name: "Main", system_template: "hi" } } },
    isLoading: false,
  });
  usePromptPackSpy.mockReturnValue({
    data: {
      apiVersion: "omnia.altairalabs.ai/v1alpha1",
      kind: "PromptPack",
      // metadata.name is the deterministic pp-<hash> object name (#1837);
      // spec.packName is the logical name the header should display (#1860).
      metadata: { name: "pp-abc123hash", namespace: "production" },
      spec: { packName: "p", version: "1.0.0", source: { type: "configmap", configMapRef: { name: "cm" } } },
      status: { phase: "Active", activeVersion: "1.0.0" },
    },
    isLoading: false,
  });
});

describe("PromptPack detail page", () => {
  it("renders the Workload tab with the graph by default", async () => {
    await act(async () => {
      render(
        <Suspense fallback={<div>Loading...</div>}>
          <PromptPackDetailPage params={Promise.resolve({ name: "p" })} />
        </Suspense>,
      );
    });
    expect(screen.getByTestId("workload-graph")).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: /workload/i })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: /advanced/i })).toBeInTheDocument();
  });

  it("shows the logical packName in the header, not the hash object name", async () => {
    await act(async () => {
      render(
        <Suspense fallback={<div>Loading...</div>}>
          <PromptPackDetailPage params={Promise.resolve({ name: "p" })} />
        </Suspense>,
      );
    });
    expect(screen.getByRole("heading", { name: "p" })).toBeInTheDocument();
    expect(screen.queryByText("pp-abc123hash")).not.toBeInTheDocument();
  });
});
