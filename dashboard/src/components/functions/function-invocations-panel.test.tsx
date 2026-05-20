/**
 * Tests for FunctionInvocationsPanel.
 *
 * Covers: loading skeleton, error envelope, summary stats (averages,
 * totals), table row rendering, status badge mapping, and the empty
 * state for recording-enabled functions that simply have no rows yet.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import type { UseQueryResult } from "@tanstack/react-query";
import { FunctionInvocationsPanel } from "./function-invocations-panel";
import type { FunctionInvocation } from "@/lib/data/function-invocations-service";

const hookSpy = vi.hoisted(() => vi.fn());

vi.mock("@/hooks/use-function-invocations", () => ({
  useFunctionInvocations: hookSpy,
}));

beforeEach(() => {
  hookSpy.mockReset();
});

/** mkHook returns a stub UseQueryResult shape that covers the union
 * branches the panel switches on. */
function mkHook(
  state: { data?: FunctionInvocation[]; isLoading?: boolean; error?: unknown },
): UseQueryResult<FunctionInvocation[]> {
  return {
    data: state.data,
    isLoading: state.isLoading ?? false,
    error: state.error ?? null,
    // The panel doesn't read these but the type demands them.
    isError: Boolean(state.error),
    isSuccess: state.data !== undefined,
  } as unknown as UseQueryResult<FunctionInvocation[]>;
}

function mkRow(overrides: Partial<FunctionInvocation> = {}): FunctionInvocation {
  return {
    id: "inv-1",
    namespace: "ns-a",
    functionName: "summarizer",
    inputHash: "abc",
    status: "success",
    durationMs: 100,
    costUsd: 0.001,
    createdAt: "2026-05-20T10:00:00Z",
    ...overrides,
  };
}

const props = {
  workspace: "ws",
  functionName: "summarizer",
  windowMs: 24 * 60 * 60 * 1000,
};

describe("FunctionInvocationsPanel", () => {
  it("renders the loading skeleton while the hook is loading", () => {
    hookSpy.mockReturnValue(mkHook({ isLoading: true }));
    render(<FunctionInvocationsPanel {...props} />);
    expect(screen.getByTestId("function-invocations-panel-loading")).toBeInTheDocument();
  });

  it("renders an error message when the hook reports a failure", () => {
    hookSpy.mockReturnValue(mkHook({ error: new Error("boom") }));
    render(<FunctionInvocationsPanel {...props} />);
    expect(screen.getByTestId("function-invocations-panel-error")).toBeInTheDocument();
    expect(screen.getByText(/Failed to load invocations: boom/)).toBeInTheDocument();
  });

  it("renders the empty state when the window has no rows", () => {
    hookSpy.mockReturnValue(mkHook({ data: [] }));
    render(<FunctionInvocationsPanel {...props} />);
    expect(
      screen.getByText("No invocations recorded in this window."),
    ).toBeInTheDocument();
  });

  it("renders one table row per invocation", () => {
    hookSpy.mockReturnValue(
      mkHook({
        data: [
          mkRow({ id: "inv-1", durationMs: 100, costUsd: 0.001 }),
          mkRow({ id: "inv-2", durationMs: 200, costUsd: 0.002 }),
        ],
      }),
    );
    render(<FunctionInvocationsPanel {...props} />);
    const rows = screen.getAllByTestId("function-invocations-row");
    expect(rows).toHaveLength(2);
  });

  it("summarises stats across all loaded rows", () => {
    hookSpy.mockReturnValue(
      mkHook({
        data: [
          mkRow({ id: "a", durationMs: 100, costUsd: 0.001 }),
          mkRow({ id: "b", durationMs: 300, costUsd: 0.003 }),
        ],
      }),
    );
    render(<FunctionInvocationsPanel {...props} />);
    // 2 invocations total
    expect(screen.getByText("2")).toBeInTheDocument();
    // Average latency = 200ms
    expect(screen.getByText("200ms")).toBeInTheDocument();
    // Total cost = $0.004
    expect(screen.getByText("$0.0040")).toBeInTheDocument();
  });

  it("maps each status enum to a readable label", () => {
    hookSpy.mockReturnValue(
      mkHook({
        data: [
          mkRow({ id: "a", status: "success" }),
          mkRow({ id: "b", status: "input_invalid" }),
          mkRow({ id: "c", status: "output_invalid" }),
          mkRow({ id: "d", status: "runtime_error" }),
        ],
      }),
    );
    render(<FunctionInvocationsPanel {...props} />);
    expect(screen.getByText("Success")).toBeInTheDocument();
    expect(screen.getByText("Input Invalid")).toBeInTheDocument();
    expect(screen.getByText("Output Invalid")).toBeInTheDocument();
    expect(screen.getByText("Runtime Error")).toBeInTheDocument();
  });

  it("formats sub-second latency in ms and >= 1s latency in seconds", () => {
    hookSpy.mockReturnValue(
      mkHook({ data: [mkRow({ durationMs: 1500 })] }),
    );
    render(<FunctionInvocationsPanel {...props} />);
    // Cell + avg both show 1.50s
    expect(screen.getAllByText("1.50s").length).toBeGreaterThanOrEqual(1);
  });

  it("renders an em-dash when traceId is missing", () => {
    hookSpy.mockReturnValue(
      mkHook({ data: [mkRow({ traceId: undefined })] }),
    );
    render(<FunctionInvocationsPanel {...props} />);
    expect(screen.getByText("—")).toBeInTheDocument();
  });

  it("truncates long trace ids to the leading 12 chars", () => {
    hookSpy.mockReturnValue(
      mkHook({
        data: [mkRow({ traceId: "0102030405060708090a0b0c0d0e0f10" })],
      }),
    );
    render(<FunctionInvocationsPanel {...props} />);
    expect(screen.getByText("010203040506")).toBeInTheDocument();
  });
});
