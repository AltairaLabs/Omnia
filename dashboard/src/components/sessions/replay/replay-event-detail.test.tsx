import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { ReplayEventDetail } from "./replay-event-detail";
import type { TimelineEvent } from "@/lib/sessions/timeline";

const t0 = "2026-04-15T12:00:00.000Z";
const events: TimelineEvent[] = [
  { id: "a", kind: "user_message", timestamp: "2026-04-15T12:00:00.500Z", label: "User", detail: "hello" },
  { id: "b", kind: "tool_call", timestamp: "2026-04-15T12:00:01.500Z", label: "Tool: search" },
];

describe("ReplayEventDetail", () => {
  it("shows a placeholder when no event has fired yet", () => {
    render(<ReplayEventDetail startedAt={t0} currentTimeMs={0} events={events} />);
    expect(screen.getByText(/no event yet/i)).toBeInTheDocument();
  });

  it("shows the most-recent event at-or-before the playhead", () => {
    render(<ReplayEventDetail startedAt={t0} currentTimeMs={1000} events={events} />);
    expect(screen.getByText("User")).toBeInTheDocument();
    expect(screen.getByText("hello")).toBeInTheDocument();
  });

  it("advances to the next event after its timestamp", () => {
    render(<ReplayEventDetail startedAt={t0} currentTimeMs={1500} events={events} />);
    expect(screen.getByText("Tool: search")).toBeInTheDocument();
  });
});
