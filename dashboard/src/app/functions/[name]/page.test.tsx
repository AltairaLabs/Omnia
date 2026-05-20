/**
 * Tests for the /functions/[name] detail page.
 *
 * The page is also orchestration — resolve the AgentRuntime by name,
 * guard against agent-mode runtimes, render the invocations panel
 * when recording is enabled, render an opt-in nag when it's not.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import FunctionDetailPage from "./page";
import type { AgentRuntime } from "@/types";

const useAgentSpy = vi.hoisted(() => vi.fn());
const useWorkspaceSpy = vi.hoisted(() => vi.fn());
const useParamsSpy = vi.hoisted(() => vi.fn());
const panelSpy = vi.hoisted(() => vi.fn());

vi.mock("@/hooks/agents", () => ({
  useAgent: useAgentSpy,
}));

vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: useWorkspaceSpy,
}));

vi.mock("next/navigation", () => ({
  useParams: useParamsSpy,
}));

vi.mock("@/components/layout", () => ({
  Header: ({ title, description }: { title: React.ReactNode; description?: React.ReactNode }) => (
    <div>
      <h1>{title}</h1>
      {description ? <p>{description}</p> : null}
    </div>
  ),
}));

vi.mock("@/components/functions/function-invocations-panel", () => ({
  FunctionInvocationsPanel: (props: { workspace: string; functionName: string; windowMs: number }) => {
    panelSpy(props);
    return <div data-testid="invocations-panel" data-window={props.windowMs} />;
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
      invocationRecording: { state: "enabled" },
      ...overrides,
    },
  };
}

beforeEach(() => {
  useAgentSpy.mockReset();
  useWorkspaceSpy.mockReset();
  useParamsSpy.mockReset();
  panelSpy.mockReset();

  useParamsSpy.mockReturnValue({ name: "summarizer" });
  useWorkspaceSpy.mockReturnValue({ currentWorkspace: { name: "ws" } });
});

describe("FunctionDetailPage", () => {
  it("shows a loading skeleton while useAgent is loading", () => {
    useAgentSpy.mockReturnValue({ data: undefined, isLoading: true });
    render(<FunctionDetailPage />);
    expect(screen.getByText("summarizer")).toBeInTheDocument();
    expect(screen.queryByTestId("invocations-panel")).not.toBeInTheDocument();
  });

  it("shows a 'not found' state when the runtime does not exist", () => {
    useAgentSpy.mockReturnValue({ data: null, isLoading: false });
    render(<FunctionDetailPage />);
    expect(screen.getByText("Function not found")).toBeInTheDocument();
  });

  it("rejects agent-mode AgentRuntimes with a friendly message", () => {
    useAgentSpy.mockReturnValue({
      data: mkFn({ mode: "agent" }),
      isLoading: false,
    });
    render(<FunctionDetailPage />);
    expect(screen.getByText("This AgentRuntime is not a Function")).toBeInTheDocument();
    expect(screen.queryByTestId("invocations-panel")).not.toBeInTheDocument();
  });

  it("renders the invocations panel when recording is enabled", () => {
    useAgentSpy.mockReturnValue({ data: mkFn(), isLoading: false });
    render(<FunctionDetailPage />);
    expect(screen.getByTestId("invocations-panel")).toBeInTheDocument();
    const last = panelSpy.mock.calls.at(-1)?.[0];
    expect(last.workspace).toBe("ws");
    expect(last.functionName).toBe("summarizer");
    // Default window is 24h
    expect(last.windowMs).toBe(24 * 60 * 60 * 1000);
  });

  it("renders the opt-in nag when recording is disabled", () => {
    useAgentSpy.mockReturnValue({
      data: mkFn({ invocationRecording: { state: "disabled" } }),
      isLoading: false,
    });
    render(<FunctionDetailPage />);
    expect(
      screen.getByText(/Invocation recording is disabled/),
    ).toBeInTheDocument();
    expect(screen.queryByTestId("invocations-panel")).not.toBeInTheDocument();
  });

  it("changes the panel windowMs when a different time-window preset is clicked", () => {
    useAgentSpy.mockReturnValue({ data: mkFn(), isLoading: false });
    render(<FunctionDetailPage />);
    fireEvent.click(screen.getByTestId("window-7d"));
    const last = panelSpy.mock.calls.at(-1)?.[0];
    expect(last.windowMs).toBe(7 * 24 * 60 * 60 * 1000);
  });

  it("renders both schemas in their respective cards", () => {
    useAgentSpy.mockReturnValue({ data: mkFn(), isLoading: false });
    render(<FunctionDetailPage />);
    // Both cards should display the JSON-stringified schema; assert via
    // a substring match on a property name the test fixture uses.
    expect(screen.getByTestId("schema-card-input-schema")).toHaveTextContent('"q"');
    expect(screen.getByTestId("schema-card-output-schema")).toHaveTextContent('"a"');
  });
});
