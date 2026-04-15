import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { ReplayScrubber } from "./replay-scrubber";
import type { TimelineEvent } from "@/lib/sessions/timeline";

const events: TimelineEvent[] = [
  { id: "1", kind: "user_message", timestamp: "2026-04-15T12:00:00.000Z", label: "User" },
  { id: "2", kind: "tool_call", timestamp: "2026-04-15T12:00:00.500Z", label: "Tool" },
  { id: "3", kind: "assistant_message", timestamp: "2026-04-15T12:00:01.000Z", label: "Bot" },
];

describe("ReplayScrubber", () => {
  it("renders one marker per event", () => {
    const { container } = render(
      <ReplayScrubber
        startedAt="2026-04-15T12:00:00.000Z"
        durationMs={1000}
        currentTimeMs={0}
        events={events}
        onSeek={vi.fn()}
      />
    );
    expect(container.querySelectorAll("[data-event-marker]")).toHaveLength(3);
  });

  it("positions markers proportionally to duration", () => {
    const { container } = render(
      <ReplayScrubber
        startedAt="2026-04-15T12:00:00.000Z"
        durationMs={1000}
        currentTimeMs={0}
        events={events}
        onSeek={vi.fn()}
      />
    );
    const markers = container.querySelectorAll("[data-event-marker]");
    expect((markers[0] as HTMLElement).style.left).toBe("0%");
    expect((markers[1] as HTMLElement).style.left).toBe("50%");
    expect((markers[2] as HTMLElement).style.left).toBe("100%");
  });

  it("calls onSeek when the slider moves", () => {
    const onSeek = vi.fn();
    render(
      <ReplayScrubber
        startedAt="2026-04-15T12:00:00.000Z"
        durationMs={1000}
        currentTimeMs={0}
        events={events}
        onSeek={onSeek}
      />
    );
    const slider = screen.getByRole("slider");
    fireEvent.keyDown(slider, { key: "ArrowRight" });
    expect(onSeek).toHaveBeenCalled();
  });

  it("uses safeDuration of 1 when durationMs is 0 (avoids division by zero)", () => {
    const { container } = render(
      <ReplayScrubber
        startedAt="2026-04-15T12:00:00.000Z"
        durationMs={0}
        currentTimeMs={0}
        events={[{ id: "1", kind: "user_message", timestamp: "2026-04-15T12:00:00.000Z", label: "User" }]}
        onSeek={vi.fn()}
      />
    );
    // With durationMs=0 → safeDuration=1 → elapsed=0 → pct=0%
    const marker = container.querySelector("[data-event-marker]") as HTMLElement;
    expect(marker.style.left).toBe("0%");
  });
});
