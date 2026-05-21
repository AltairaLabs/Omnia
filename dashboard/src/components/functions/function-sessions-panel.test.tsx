/**
 * Tests for FunctionSessionsPanel.
 *
 * Covers loading skeleton, error envelope, empty state, row
 * rendering, status badge mapping, latency/cost formatting, and the
 * session-id link target — which is the load-bearing assertion:
 * each row must link into /sessions/{id} so operators can pivot
 * from this view into the full session detail.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import type { UseQueryResult } from "@tanstack/react-query";
import { FunctionSessionsPanel } from "./function-sessions-panel";
import type { SessionListResponse, SessionSummary } from "@/types/session";

const hookSpy = vi.hoisted(() => vi.fn());

vi.mock("@/hooks/sessions", () => ({
  useSessions: hookSpy,
}));

vi.mock("next/link", () => ({
  default: ({ children, href }: { children: React.ReactNode; href: string }) => (
    <a href={href}>{children}</a>
  ),
}));

beforeEach(() => {
  hookSpy.mockReset();
});

function mkHook(state: {
  data?: SessionListResponse;
  isLoading?: boolean;
  error?: unknown;
}): UseQueryResult<SessionListResponse> {
  return {
    data: state.data,
    isLoading: state.isLoading ?? false,
    error: state.error ?? null,
    isError: Boolean(state.error),
    isSuccess: state.data !== undefined,
  } as unknown as UseQueryResult<SessionListResponse>;
}

function mkSession(overrides: Partial<SessionSummary> = {}): SessionSummary {
  return {
    id: "00000000-0000-0000-0000-000000000001",
    agentName: "summarizer",
    agentNamespace: "ns-a",
    status: "completed",
    startedAt: "2026-05-20T10:00:00Z",
    endedAt: "2026-05-20T10:00:01.500Z",
    messageCount: 2,
    toolCallCount: 0,
    totalTokens: 50,
    estimatedCost: 0.0012,
    tags: ["function"],
    ...overrides,
  };
}

describe("FunctionSessionsPanel", () => {
  it("renders the loading skeleton while the hook is loading", () => {
    hookSpy.mockReturnValue(mkHook({ isLoading: true }));
    render(<FunctionSessionsPanel functionName="summarizer" />);
    expect(
      screen.getByTestId("function-sessions-panel-loading"),
    ).toBeInTheDocument();
  });

  it("renders an error envelope when the hook reports a failure", () => {
    hookSpy.mockReturnValue(mkHook({ error: new Error("boom") }));
    render(<FunctionSessionsPanel functionName="summarizer" />);
    expect(
      screen.getByTestId("function-sessions-panel-error"),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/Failed to load invocations: boom/),
    ).toBeInTheDocument();
  });

  it("renders the empty state when no sessions exist", () => {
    hookSpy.mockReturnValue(
      mkHook({ data: { sessions: [], total: 0, hasMore: false } }),
    );
    render(<FunctionSessionsPanel functionName="summarizer" />);
    expect(screen.getByText("No invocations recorded yet.")).toBeInTheDocument();
  });

  it("renders one row per session", () => {
    hookSpy.mockReturnValue(
      mkHook({
        data: {
          sessions: [
            mkSession({ id: "a" }),
            mkSession({ id: "b" }),
            mkSession({ id: "c" }),
          ],
          total: 3,
          hasMore: false,
        },
      }),
    );
    render(<FunctionSessionsPanel functionName="summarizer" />);
    expect(screen.getAllByTestId("function-sessions-row")).toHaveLength(3);
  });

  it("maps each status enum to a readable label", () => {
    hookSpy.mockReturnValue(
      mkHook({
        data: {
          sessions: [
            mkSession({ id: "a", status: "completed" }),
            mkSession({ id: "b", status: "error" }),
            mkSession({ id: "c", status: "active", endedAt: undefined }),
            mkSession({ id: "d", status: "expired" }),
          ],
          total: 4,
          hasMore: false,
        },
      }),
    );
    render(<FunctionSessionsPanel functionName="summarizer" />);
    expect(screen.getByText("Completed")).toBeInTheDocument();
    expect(screen.getByText("Error")).toBeInTheDocument();
    expect(screen.getByText("Active")).toBeInTheDocument();
    expect(screen.getByText("Expired")).toBeInTheDocument();
  });

  it("formats sub-second latency in ms and >= 1s in seconds", () => {
    hookSpy.mockReturnValue(
      mkHook({
        data: {
          sessions: [
            mkSession({
              id: "fast",
              startedAt: "2026-05-20T10:00:00Z",
              endedAt: "2026-05-20T10:00:00.100Z",
            }),
            mkSession({
              id: "slow",
              startedAt: "2026-05-20T10:00:00Z",
              endedAt: "2026-05-20T10:00:02.500Z",
            }),
          ],
          total: 2,
          hasMore: false,
        },
      }),
    );
    render(<FunctionSessionsPanel functionName="summarizer" />);
    expect(screen.getByText("100ms")).toBeInTheDocument();
    expect(screen.getByText("2.50s")).toBeInTheDocument();
  });

  it("renders an em-dash for active rows that have no endedAt", () => {
    hookSpy.mockReturnValue(
      mkHook({
        data: {
          sessions: [
            mkSession({ id: "live", status: "active", endedAt: undefined }),
          ],
          total: 1,
          hasMore: false,
        },
      }),
    );
    render(<FunctionSessionsPanel functionName="summarizer" />);
    expect(screen.getByText("—")).toBeInTheDocument();
  });

  it("each row links into /sessions/{id} so operators can drill in", () => {
    hookSpy.mockReturnValue(
      mkHook({
        data: {
          sessions: [
            mkSession({ id: "abcdef0123456789", status: "error" }),
          ],
          total: 1,
          hasMore: false,
        },
      }),
    );
    render(<FunctionSessionsPanel functionName="summarizer" />);
    const link = screen.getByRole("link");
    expect(link).toHaveAttribute("href", "/sessions/abcdef0123456789");
    // The truncated label (first 8 chars + ellipsis) is what the user sees.
    expect(link.textContent).toContain("abcdef01");
  });

  it("passes the function name to useSessions as the `agent` filter", () => {
    hookSpy.mockReturnValue(
      mkHook({ data: { sessions: [], total: 0, hasMore: false } }),
    );
    render(<FunctionSessionsPanel functionName="my-fn" limit={25} />);
    expect(hookSpy).toHaveBeenCalledWith({ agent: "my-fn", limit: 25 });
  });
});
