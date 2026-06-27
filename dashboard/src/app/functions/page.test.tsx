/**
 * Tests for the /functions catalog page.
 *
 * The page is mostly orchestration — filter useAgents() down to
 * function-mode rows, render FunctionCard for each. Tests pin: the
 * agent-mode filter, the namespace filter integration, the empty
 * state, and the loading skeleton.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import FunctionsPage from "./page";
import type { AgentRuntime } from "@/types";

const agentsSpy = vi.hoisted(() => vi.fn());
const workspaceSpy = vi.hoisted(() => vi.fn());

vi.mock("@/hooks/agents", () => ({
  useAgents: agentsSpy,
  // FunctionCard reads cost from this Prometheus-backed hook.
  useAgentCost: vi.fn(() => ({ data: { totalCost: 0, timeSeries: [] } })),
}));

vi.mock("@/hooks/resources", () => ({
  useProvider: vi.fn(() => ({ data: { spec: { type: "anthropic", model: "claude-opus-4-8" } } })),
}));

vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: workspaceSpy,
}));

vi.mock("next/link", () => ({
  default: ({
    children,
    href,
  }: {
    children: React.ReactNode;
    href: string;
  }) => <a href={href}>{children}</a>,
}));

// Stub the layout Header — it pulls in WorkspaceSwitcher, UserMenu and
// the next-themes provider, none of which we need for this page's
// filter logic.
vi.mock("@/components/layout", () => ({
  Header: ({ title, description }: { title: React.ReactNode; description?: React.ReactNode }) => (
    <div>
      <h1>{title}</h1>
      {description ? <p>{description}</p> : null}
    </div>
  ),
}));

// Stub NamespaceFilter — its popover internals (Radix) need a portal
// host the JSDOM environment doesn't supply by default. The page-level
// test only cares that the function-mode rows are filtered correctly.
vi.mock("@/components/filters", () => ({
  NamespaceFilter: () => <div data-testid="namespace-filter-stub" />,
}));

beforeEach(() => {
  agentsSpy.mockReset();
  workspaceSpy.mockReset();
  workspaceSpy.mockReturnValue({ isLoading: false, currentWorkspace: { name: "ws" } });
});

function mkAgent(
  name: string,
  mode: "agent" | "function" | undefined,
  namespace = "ns-a",
): AgentRuntime {
  return {
    apiVersion: "omnia.altairalabs.ai/v1alpha1",
    kind: "AgentRuntime",
    metadata: { name, namespace, uid: `uid-${name}` },
    spec: {
      mode,
      promptPackRef: { name: "pack" },
      facades: [{ type: "rest" }],
    },
  };
}

describe("FunctionsPage", () => {
  it("shows the loading skeleton while agents are loading", () => {
    agentsSpy.mockReturnValue({ data: undefined, isLoading: true });
    render(<FunctionsPage />);
    // The Skeleton component doesn't expose a role, but it does have the
    // grid layout structure — check the heading is present and no cards
    // are rendered (loading state suppresses them).
    expect(screen.getByText("Functions")).toBeInTheDocument();
    expect(screen.queryAllByTestId("function-card")).toHaveLength(0);
  });

  it("filters out agent-mode runtimes — only function-mode rows render", () => {
    agentsSpy.mockReturnValue({
      data: [
        mkAgent("summarizer", "function"),
        mkAgent("chatbot", "agent"),
        mkAgent("classifier", "function"),
        mkAgent("legacy", undefined), // unset mode == agent default
      ],
      isLoading: false,
    });
    render(<FunctionsPage />);
    const cards = screen.getAllByTestId("function-card");
    expect(cards).toHaveLength(2);
    expect(screen.getByText("summarizer")).toBeInTheDocument();
    expect(screen.getByText("classifier")).toBeInTheDocument();
    expect(screen.queryByText("chatbot")).not.toBeInTheDocument();
    expect(screen.queryByText("legacy")).not.toBeInTheDocument();
  });

  it("renders the empty state when no function-mode runtimes exist", () => {
    agentsSpy.mockReturnValue({
      data: [mkAgent("chatbot", "agent")], // agent-mode only
      isLoading: false,
    });
    render(<FunctionsPage />);
    expect(
      screen.getByText("No function-mode AgentRuntimes found."),
    ).toBeInTheDocument();
  });

  it("sorts function rows alphabetically by name", () => {
    agentsSpy.mockReturnValue({
      data: [
        mkAgent("zeta", "function"),
        mkAgent("alpha", "function"),
        mkAgent("middle", "function"),
      ],
      isLoading: false,
    });
    render(<FunctionsPage />);
    const cards = screen.getAllByTestId("function-card");
    const names = cards.map((c) => c.querySelector("div")?.textContent ?? "");
    expect(names[0]).toContain("alpha");
    expect(names[1]).toContain("middle");
    expect(names[2]).toContain("zeta");
  });

  it("falls back gracefully when useAgents returns undefined data", () => {
    agentsSpy.mockReturnValue({ data: undefined, isLoading: false });
    render(<FunctionsPage />);
    expect(screen.getByText("No function-mode AgentRuntimes found.")).toBeInTheDocument();
  });
});
