/**
 * Tests for the /functions/[name] detail page.
 *
 * The page is orchestration — resolve the AgentRuntime by name, guard
 * against agent-mode runtimes, render the schemas. The session-backed
 * invocation history wiring lands in the next PR.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import FunctionDetailPage from "./page";
import type { AgentRuntime } from "@/types";

const useAgentSpy = vi.hoisted(() => vi.fn());
const useParamsSpy = vi.hoisted(() => vi.fn());

vi.mock("@/hooks/agents", () => ({
  useAgent: useAgentSpy,
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

beforeEach(() => {
  useAgentSpy.mockReset();
  useParamsSpy.mockReset();

  useParamsSpy.mockReturnValue({ name: "summarizer" });
});

describe("FunctionDetailPage", () => {
  it("shows a loading skeleton while useAgent is loading", () => {
    useAgentSpy.mockReturnValue({ data: undefined, isLoading: true });
    render(<FunctionDetailPage />);
    expect(screen.getByText("summarizer")).toBeInTheDocument();
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
  });

  it("renders both schemas in their respective cards", () => {
    useAgentSpy.mockReturnValue({ data: mkFn(), isLoading: false });
    render(<FunctionDetailPage />);
    expect(screen.getByTestId("schema-card-input-schema")).toHaveTextContent('"q"');
    expect(screen.getByTestId("schema-card-output-schema")).toHaveTextContent('"a"');
  });

  it("renders the next-PR placeholder for invocation history", () => {
    useAgentSpy.mockReturnValue({ data: mkFn(), isLoading: false });
    render(<FunctionDetailPage />);
    expect(
      screen.getByText(/Function invocations are recorded as sessions/),
    ).toBeInTheDocument();
  });
});
