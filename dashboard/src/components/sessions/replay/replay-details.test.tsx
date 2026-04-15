import { describe, it, expect } from "vitest";
import { render, screen, fireEvent, within } from "@testing-library/react";
import { ReplayDetails } from "./replay-details";
import type { TimelineEvent } from "@/lib/sessions/timeline";

const t0 = "2026-04-15T12:00:00.000Z";
const events: TimelineEvent[] = [
  {
    id: "a",
    kind: "user_message",
    timestamp: "2026-04-15T12:00:00.500Z",
    label: "User said hi",
    detail: "hi",
  },
  {
    id: "b",
    kind: "pipeline_event",
    timestamp: "2026-04-15T12:00:01.000Z",
    label: "pipeline.started",
    metadata: { stage: "intro" },
  },
  {
    id: "c",
    kind: "tool_call",
    timestamp: "2026-04-15T12:00:01.500Z",
    label: "Tool: search",
    duration: 42,
    status: "success",
  },
];

describe("ReplayDetails", () => {
  it("renders an empty-state placeholder when no events are visible", () => {
    render(<ReplayDetails startedAt={t0} currentTimeMs={0} events={events} />);
    expect(screen.getByText(/no events yet/i)).toBeInTheDocument();
  });

  it("renders one row per visible event up to currentTimeMs", () => {
    render(<ReplayDetails startedAt={t0} currentTimeMs={1200} events={events} />);
    const rows = screen.getAllByTestId("replay-details-row");
    expect(rows).toHaveLength(2);
    expect(screen.getByText("User said hi")).toBeInTheDocument();
    expect(screen.getByText("pipeline.started")).toBeInTheDocument();
    expect(screen.queryByText("Tool: search")).not.toBeInTheDocument();
  });

  it("marks the most-recent row as current", () => {
    render(<ReplayDetails startedAt={t0} currentTimeMs={1200} events={events} />);
    const rows = screen.getAllByTestId("replay-details-row");
    // Rows sorted ascending by time; the most recent is the last one.
    expect(rows[rows.length - 1]).toHaveAttribute("data-current", "true");
    expect(rows[0]).not.toHaveAttribute("data-current");
  });

  it("expands metadata on row click", () => {
    render(<ReplayDetails startedAt={t0} currentTimeMs={1200} events={events} />);
    const pipelineRow = screen
      .getByText("pipeline.started")
      .closest("[data-testid='replay-details-row']") as HTMLElement;
    // Not expanded initially.
    expect(within(pipelineRow).queryByText(/"stage"/)).not.toBeInTheDocument();
    // Click to expand.
    fireEvent.click(within(pipelineRow).getByRole("button"));
    expect(within(pipelineRow).getByText(/"stage": "intro"/)).toBeInTheDocument();
  });

  it("expands events that have no detail/metadata/duration, showing kind + id + timestamp", () => {
    const sparse: TimelineEvent[] = [
      { id: "bare", kind: "system_message", timestamp: t0, label: "Nothing to see" },
    ];
    render(<ReplayDetails startedAt={t0} currentTimeMs={100} events={sparse} />);
    const row = screen.getByTestId("replay-details-row");
    const toggle = within(row).getByRole("button");
    expect(toggle).not.toBeDisabled();
    expect(toggle).toHaveAttribute("aria-expanded", "false");
    fireEvent.click(toggle);
    expect(toggle).toHaveAttribute("aria-expanded", "true");
    // The expanded drawer shows the raw event identifiers.
    expect(within(row).getByText("bare")).toBeInTheDocument();
    expect(within(row).getByText("system_message")).toBeInTheDocument();
  });

  it("collapses again on a second click", () => {
    const sparse: TimelineEvent[] = [
      { id: "bare", kind: "system_message", timestamp: t0, label: "Toggle me" },
    ];
    render(<ReplayDetails startedAt={t0} currentTimeMs={100} events={sparse} />);
    const toggle = screen.getByRole("button");
    fireEvent.click(toggle);
    expect(toggle).toHaveAttribute("aria-expanded", "true");
    fireEvent.click(toggle);
    expect(toggle).toHaveAttribute("aria-expanded", "false");
  });

  it("renders an error badge when a row's status is error", () => {
    const withError: TimelineEvent[] = [
      { id: "e1", kind: "error", timestamp: t0, label: "Boom", status: "error" },
    ];
    render(<ReplayDetails startedAt={t0} currentTimeMs={100} events={withError} />);
    expect(screen.getByText("error")).toBeInTheDocument();
  });

  it("renders duration in ms when provided", () => {
    render(<ReplayDetails startedAt={t0} currentTimeMs={10_000} events={events} />);
    expect(screen.getByText("42ms")).toBeInTheDocument();
  });
});
