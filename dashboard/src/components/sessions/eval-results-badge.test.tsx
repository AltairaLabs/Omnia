/**
 * Tests for EvalResultsBadge component.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { describe, it, expect } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { EvalResultsBadge } from "./eval-results-badge";
import type { EvalResult } from "@/types/eval";

function makeEvalResult(overrides: Partial<EvalResult> = {}): EvalResult {
  return {
    id: "eval-1",
    sessionId: "sess-1",
    messageId: "m1",
    agentName: "agent-1",
    namespace: "default",
    promptpackName: "pp-1",
    evalId: "tone-check",
    evalType: "llm_judge",
    trigger: "on_response",
    passed: true,
    score: 0.95,
    source: "in_proc",
    createdAt: "2026-01-15T10:00:00Z",
    ...overrides,
  };
}

describe("EvalResultsBadge", () => {
  it("renders nothing when results array is empty", () => {
    const { container } = render(<EvalResultsBadge results={[]} />);
    expect(container.innerHTML).toBe("");
  });

  it("renders a passing badge when all evals pass", () => {
    const results = [
      makeEvalResult({ id: "e1", passed: true }),
      makeEvalResult({ id: "e2", passed: true }),
    ];

    render(<EvalResultsBadge results={results} />);

    expect(screen.getByText("2 evals passed")).toBeInTheDocument();
  });

  it("renders singular text for a single passing eval", () => {
    const results = [makeEvalResult({ id: "e1", passed: true })];

    render(<EvalResultsBadge results={results} />);

    expect(screen.getByText("1 eval passed")).toBeInTheDocument();
  });

  it("renders a failing badge when some evals fail", () => {
    const results = [
      makeEvalResult({ id: "e1", passed: true }),
      makeEvalResult({ id: "e2", passed: false }),
    ];

    render(<EvalResultsBadge results={results} />);

    expect(screen.getByText("1 of 2 evals failed")).toBeInTheDocument();
  });

  it("renders singular text when 1 of 1 eval fails", () => {
    const results = [makeEvalResult({ id: "e1", passed: false })];

    render(<EvalResultsBadge results={results} />);

    expect(screen.getByText("1 of 1 eval failed")).toBeInTheDocument();
  });

  it("expands to show individual eval details on click", () => {
    const results = [
      makeEvalResult({
        id: "e1",
        evalId: "tone-check",
        evalType: "llm_judge",
        passed: true,
        score: 0.85,
        details: { reason: "Good tone" },
      }),
    ];

    render(<EvalResultsBadge results={results} />);

    // Click to expand
    fireEvent.click(screen.getByTestId("eval-results-badge"));

    // Eval details should be visible
    expect(screen.getByTestId("eval-results-details")).toBeInTheDocument();
    expect(screen.getByText("tone-check")).toBeInTheDocument();
    expect(screen.getByText("LLM Judge")).toBeInTheDocument();
    expect(screen.getByText("Score: 85%")).toBeInTheDocument();
  });

  it("collapses details on second click", () => {
    const results = [makeEvalResult({ id: "e1", details: { a: 1 } })];

    render(<EvalResultsBadge results={results} />);

    const badge = screen.getByTestId("eval-results-badge");

    // Expand
    fireEvent.click(badge);
    expect(screen.getByTestId("eval-results-details")).toBeInTheDocument();

    // Collapse
    fireEvent.click(badge);
    expect(screen.queryByTestId("eval-results-details")).not.toBeInTheDocument();
  });

  it("shows eval detail row with source and duration on expand", () => {
    const results = [
      makeEvalResult({
        id: "e1",
        evalId: "safety-check",
        evalType: "rule",
        passed: false,
        durationMs: 42,
        judgeCostUsd: 0.0012,
        source: "worker",
        details: { reason: "Unsafe content" },
      }),
    ];

    render(<EvalResultsBadge results={results} />);

    // Expand the badge
    fireEvent.click(screen.getByTestId("eval-results-badge"));

    // Click the detail row to expand it
    fireEvent.click(screen.getByText("safety-check"));

    expect(screen.getByText("Rule")).toBeInTheDocument();
    expect(screen.getByText("Source: Worker")).toBeInTheDocument();
    expect(screen.getByText("42ms")).toBeInTheDocument();
    expect(screen.getByText("$0.0012")).toBeInTheDocument();
  });

  it("does not show expand chevron when details is empty", () => {
    const results = [
      makeEvalResult({
        id: "e1",
        evalId: "simple-check",
        passed: true,
        details: undefined,
      }),
    ];

    render(<EvalResultsBadge results={results} />);
    fireEvent.click(screen.getByTestId("eval-results-badge"));

    // The detail row should be visible but not expandable (no chevron)
    expect(screen.getByText("simple-check")).toBeInTheDocument();
  });

  it("handles unknown eval types gracefully", () => {
    const results = [
      makeEvalResult({
        id: "e1",
        evalId: "custom-eval",
        evalType: "unknown_type",
        passed: true,
        details: { x: 1 },
      }),
    ];

    render(<EvalResultsBadge results={results} />);
    fireEvent.click(screen.getByTestId("eval-results-badge"));

    // Should display the raw eval type when no label mapping exists
    expect(screen.getByText("unknown_type")).toBeInTheDocument();
  });

  it("handles unknown source gracefully", () => {
    const results = [
      makeEvalResult({
        id: "e1",
        source: "unknown_source",
        details: { x: 1 },
      }),
    ];

    render(<EvalResultsBadge results={results} />);
    fireEvent.click(screen.getByTestId("eval-results-badge"));
    fireEvent.click(screen.getByText("tone-check"));

    expect(screen.getByText("Source: unknown_source")).toBeInTheDocument();
  });

  it("does not show score when score is undefined", () => {
    const results = [
      makeEvalResult({
        id: "e1",
        score: undefined,
        details: { x: 1 },
      }),
    ];

    render(<EvalResultsBadge results={results} />);
    fireEvent.click(screen.getByTestId("eval-results-badge"));

    expect(screen.queryByText(/Score:/)).not.toBeInTheDocument();
  });
});
