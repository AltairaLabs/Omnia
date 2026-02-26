/**
 * Tests for FailingSessionsTable component.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React from "react";

// Mock hooks
const mockUseRecentEvalFailures = vi.fn();

vi.mock("@/hooks", () => ({
  useRecentEvalFailures: (...args: unknown[]) => mockUseRecentEvalFailures(...args),
}));

vi.mock("next/link", () => ({
  default: ({ children, href }: { children: React.ReactNode; href: string }) => (
    <a href={href}>{children}</a>
  ),
}));

vi.mock("date-fns", () => ({
  formatDistanceToNow: () => "3 minutes ago",
}));

import { FailingSessionsTable } from "./failing-sessions-table";

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  function TestQueryProvider({ children }: { children: React.ReactNode }) {
    return React.createElement(QueryClientProvider, { client: queryClient }, children);
  }
  return TestQueryProvider;
}

const mockFailureData = {
  evalResults: [
    {
      id: "er-1",
      sessionId: "sess-abc",
      agentName: "my-agent",
      evalId: "tone-check",
      evalType: "llm_judge",
      promptpackName: "default-pack",
      passed: false,
      score: 0.3,
      createdAt: new Date().toISOString(),
    },
    {
      id: "er-2",
      sessionId: "sess-def",
      agentName: "other-agent",
      evalId: "safety-check",
      evalType: "rule",
      promptpackName: "safety-pack",
      passed: false,
      score: 0.1,
      createdAt: new Date().toISOString(),
    },
  ],
  total: 2,
};

describe("FailingSessionsTable", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("shows skeleton loading state", () => {
    mockUseRecentEvalFailures.mockReturnValue({
      data: undefined,
      isLoading: true,
      error: null,
    });

    const Wrapper = createWrapper();
    const { container } = render(
      <Wrapper>
        <FailingSessionsTable />
      </Wrapper>
    );

    const skeletons = container.querySelectorAll('[data-slot="skeleton"]');
    expect(skeletons.length).toBeGreaterThan(0);
  });

  it("shows 'No recent failures' when empty", () => {
    mockUseRecentEvalFailures.mockReturnValue({
      data: { evalResults: [], total: 0 },
      isLoading: false,
      error: null,
    });

    const Wrapper = createWrapper();
    render(
      <Wrapper>
        <FailingSessionsTable />
      </Wrapper>
    );

    expect(screen.getByText("No recent failures")).toBeInTheDocument();
  });

  it("shows 'No recent failures' when data is undefined", () => {
    mockUseRecentEvalFailures.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: null,
    });

    const Wrapper = createWrapper();
    render(
      <Wrapper>
        <FailingSessionsTable />
      </Wrapper>
    );

    expect(screen.getByText("No recent failures")).toBeInTheDocument();
  });

  it("renders failure rows with eval ID, agent, pack name", () => {
    mockUseRecentEvalFailures.mockReturnValue({
      data: mockFailureData,
      isLoading: false,
      error: null,
    });

    const Wrapper = createWrapper();
    render(
      <Wrapper>
        <FailingSessionsTable />
      </Wrapper>
    );

    // Eval IDs
    expect(screen.getByText("tone-check")).toBeInTheDocument();
    expect(screen.getByText("safety-check")).toBeInTheDocument();
    // Agent names
    expect(screen.getByText("my-agent")).toBeInTheDocument();
    expect(screen.getByText("other-agent")).toBeInTheDocument();
    // Pack names
    expect(screen.getByText("default-pack")).toBeInTheDocument();
    expect(screen.getByText("safety-pack")).toBeInTheDocument();
    // Time
    const timeElements = screen.getAllByText("3 minutes ago");
    expect(timeElements).toHaveLength(2);
  });

  it("renders session links", () => {
    mockUseRecentEvalFailures.mockReturnValue({
      data: mockFailureData,
      isLoading: false,
      error: null,
    });

    const Wrapper = createWrapper();
    const { container } = render(
      <Wrapper>
        <FailingSessionsTable />
      </Wrapper>
    );

    const links = container.querySelectorAll("a[href]");
    const hrefs = Array.from(links).map((l) => l.getAttribute("href"));
    expect(hrefs).toContain("/sessions/sess-abc");
    expect(hrefs).toContain("/sessions/sess-def");
  });

  it("shows error alert on error", () => {
    mockUseRecentEvalFailures.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error("Service unavailable"),
    });

    const Wrapper = createWrapper();
    render(
      <Wrapper>
        <FailingSessionsTable />
      </Wrapper>
    );

    expect(screen.getByText("Failed to load failing sessions")).toBeInTheDocument();
  });

  it("displays card header with title and description", () => {
    mockUseRecentEvalFailures.mockReturnValue({
      data: { evalResults: [], total: 0 },
      isLoading: false,
      error: null,
    });

    const Wrapper = createWrapper();
    render(
      <Wrapper>
        <FailingSessionsTable />
      </Wrapper>
    );

    expect(screen.getByText("Failing Sessions")).toBeInTheDocument();
    expect(screen.getByText("Recent failures for: All Types")).toBeInTheDocument();
  });

  it("displays formatted eval type in description", () => {
    mockUseRecentEvalFailures.mockReturnValue({
      data: { evalResults: [], total: 0 },
      isLoading: false,
      error: null,
    });

    const Wrapper = createWrapper();
    render(
      <Wrapper>
        <FailingSessionsTable evalType="omnia_eval_tone_quality" />
      </Wrapper>
    );

    expect(screen.getByText("Recent failures for: tone quality")).toBeInTheDocument();
  });

  it("passes correct parameters to useRecentEvalFailures", () => {
    mockUseRecentEvalFailures.mockReturnValue({
      data: undefined,
      isLoading: true,
      error: null,
    });

    const Wrapper = createWrapper();
    render(
      <Wrapper>
        <FailingSessionsTable evalType="tone" agentName="my-agent" limit={5} />
      </Wrapper>
    );

    expect(mockUseRecentEvalFailures).toHaveBeenCalledWith({
      evalType: "tone",
      agentName: "my-agent",
      passed: false,
      limit: 5,
    });
  });

  it("uses default limit of 10", () => {
    mockUseRecentEvalFailures.mockReturnValue({
      data: undefined,
      isLoading: true,
      error: null,
    });

    const Wrapper = createWrapper();
    render(
      <Wrapper>
        <FailingSessionsTable />
      </Wrapper>
    );

    expect(mockUseRecentEvalFailures).toHaveBeenCalledWith({
      evalType: undefined,
      agentName: undefined,
      passed: false,
      limit: 10,
    });
  });
});
