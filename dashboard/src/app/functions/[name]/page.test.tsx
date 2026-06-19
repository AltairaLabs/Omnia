/**
 * Tests for the /functions/[name] detail page.
 *
 * The page is orchestration — resolve the AgentRuntime by name, guard
 * against agent-mode runtimes, and assemble the shared agent controls into
 * function-flavoured tabs (Overview diagram + scale + conditions, a
 * pretty-printed Schema tab, and the sessions-backed Invocations tab). The
 * heavy shared components are stubbed here; their behaviour is covered by
 * their own tests.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import FunctionDetailPage from "./page";
import type { AgentRuntime } from "@/types";

const useAgentSpy = vi.hoisted(() => vi.fn());
const useParamsSpy = vi.hoisted(() => vi.fn());
const useSearchParamsSpy = vi.hoisted(() => vi.fn(() => new URLSearchParams()));

vi.mock("@/hooks/agents", () => ({ useAgent: useAgentSpy }));
vi.mock("next/navigation", () => ({
  useParams: useParamsSpy,
  useSearchParams: useSearchParamsSpy,
  useRouter: () => ({ replace: vi.fn() }),
  usePathname: () => "/functions/summarizer",
}));

vi.mock("@/components/layout", () => ({
  Header: ({ title, description }: { title: ReactNode; description?: ReactNode }) => (
    <div>
      <h1>{title}</h1>
      {description ? <p>{description}</p> : null}
    </div>
  ),
}));

vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: () => ({ currentWorkspace: { name: "ws-a" } }),
}));
vi.mock("@/lib/data", () => ({ useDataService: () => ({ scaleAgent: vi.fn() }) }));

// Stub the heavy shared controls — their own tests cover behaviour.
vi.mock("@/components/agents", () => ({
  StatusBadge: () => <div data-testid="status-badge" />,
  ScaleControl: () => <div data-testid="scale-control" />,
  AgentTopology: () => <div data-testid="agent-topology" />,
  AgentConditions: () => <div data-testid="agent-conditions" />,
  EvalConfigPanel: () => <div data-testid="eval-config" />,
  AgentMetricsPanel: () => <div data-testid="metrics-panel" />,
  AgentQualityTab: () => <div data-testid="quality-tab" />,
  EventsPanel: () => <div data-testid="events-panel" />,
}));
vi.mock("@/components/agents/system-pack-badge", () => ({
  SystemPackBadge: () => <div data-testid="system-pack" />,
}));
vi.mock("@/components/workload-graph/agent-workload-tab", () => ({
  AgentWorkloadTab: () => <div data-testid="workload-tab" />,
}));
vi.mock("@/components/logs", () => ({ LogViewer: () => <div data-testid="log-viewer" /> }));

const panelSpy = vi.hoisted(() => vi.fn());
vi.mock("@/components/functions/function-sessions-panel", () => ({
  FunctionSessionsPanel: (props: { functionName: string }) => {
    panelSpy(props);
    return <div data-testid="sessions-panel" data-fn={props.functionName} />;
  },
}));

function mkFn(overrides: Partial<AgentRuntime["spec"]> = {}): AgentRuntime {
  return {
    apiVersion: "omnia.altairalabs.ai/v1alpha1",
    kind: "AgentRuntime",
    metadata: { name: "summarizer", namespace: "ns-a", uid: "uid-1" },
    spec: {
      mode: "function",
      promptPackRef: { name: "pack" },
      facade: { type: "grpc" as never },
      inputSchema: { type: "object", properties: { q: { type: "string" } } },
      outputSchema: { type: "object", properties: { a: { type: "string" } } },
      ...overrides,
    },
  };
}

function renderPage() {
  const qc = new QueryClient();
  return render(
    <QueryClientProvider client={qc}>
      <FunctionDetailPage />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  useAgentSpy.mockReset();
  useParamsSpy.mockReset();
  panelSpy.mockReset();
  useSearchParamsSpy.mockReset();
  useParamsSpy.mockReturnValue({ name: "summarizer" });
  useSearchParamsSpy.mockReturnValue(new URLSearchParams());
});

describe("FunctionDetailPage", () => {
  it("shows a loading skeleton while useAgent is loading", () => {
    useAgentSpy.mockReturnValue({ data: undefined, isLoading: true });
    renderPage();
    expect(screen.getByText("summarizer")).toBeInTheDocument();
  });

  it("shows a 'not found' state when the runtime does not exist", () => {
    useAgentSpy.mockReturnValue({ data: null, isLoading: false });
    renderPage();
    expect(screen.getByText("Function not found")).toBeInTheDocument();
  });

  it("rejects agent-mode AgentRuntimes with a friendly message", () => {
    useAgentSpy.mockReturnValue({ data: mkFn({ mode: "agent" }), isLoading: false });
    renderPage();
    expect(screen.getByText("This AgentRuntime is not a Function")).toBeInTheDocument();
  });

  it("renders the shared function tabs", () => {
    useAgentSpy.mockReturnValue({ data: mkFn(), isLoading: false });
    renderPage();
    for (const tab of [
      "Overview", "Schema", "Invocations", "Workload",
      "Logs", "Metrics", "Quality", "Configuration", "Events",
    ]) {
      expect(screen.getByRole("tab", { name: tab })).toBeInTheDocument();
    }
  });

  it("puts the architecture diagram on the Overview tab", () => {
    useAgentSpy.mockReturnValue({ data: mkFn(), isLoading: false });
    renderPage();
    expect(screen.getByTestId("agent-topology")).toBeInTheDocument();
    expect(screen.getByTestId("scale-control")).toBeInTheDocument();
  });

  it("pretty-prints both schemas on the Schema tab", () => {
    useSearchParamsSpy.mockReturnValue(new URLSearchParams("tab=schema"));
    useAgentSpy.mockReturnValue({ data: mkFn(), isLoading: false });
    renderPage();
    expect(screen.getByText('"q"')).toBeInTheDocument();
    expect(screen.getByText('"a"')).toBeInTheDocument();
  });

  it("mounts the sessions panel on the Invocations tab", () => {
    useSearchParamsSpy.mockReturnValue(new URLSearchParams("tab=invocations"));
    useAgentSpy.mockReturnValue({ data: mkFn(), isLoading: false });
    renderPage();
    expect(screen.getByTestId("sessions-panel")).toBeInTheDocument();
    expect(panelSpy).toHaveBeenCalledWith({ functionName: "summarizer" });
  });

  it("keeps the forceMount Test panel hidden on other tabs", () => {
    useAgentSpy.mockReturnValue({ data: mkFn(), isLoading: false });
    renderPage(); // default tab = overview
    // The Test panel is forceMount, so it stays in the DOM across tabs; without
    // an explicit inactive-hide it would render on EVERY tab. The only inactive
    // tabpanel present (the others unmount) is the Test panel — it must carry
    // the hide class so it isn't shown when another tab is active.
    const inactivePanel = screen
      .getAllByRole("tabpanel")
      .find((p) => p.getAttribute("data-state") === "inactive");
    expect(inactivePanel).toBeTruthy();
    expect(inactivePanel?.className).toContain("data-[state=inactive]:hidden");
  });
});
