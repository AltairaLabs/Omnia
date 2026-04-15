import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { ReplayControls } from "./replay-controls";
import type { TimelineEvent } from "@/lib/sessions/timeline";

const base = {
  playing: false,
  speed: 1,
  currentTimeMs: 0,
  durationMs: 5000,
  startedAt: "2026-04-15T12:00:00.000Z",
  events: [] as TimelineEvent[],
  onPlay: vi.fn(),
  onPause: vi.fn(),
  onSeek: vi.fn(),
  onSpeedChange: vi.fn(),
};

describe("ReplayControls", () => {
  it("shows Play when paused and Pause when playing", () => {
    const { rerender } = render(<ReplayControls {...base} />);
    expect(screen.getByRole("button", { name: /play/i })).toBeInTheDocument();
    rerender(<ReplayControls {...base} playing />);
    expect(screen.getByRole("button", { name: /pause/i })).toBeInTheDocument();
  });

  it("formats current time and duration as MM:SS.mmm", () => {
    render(<ReplayControls {...base} currentTimeMs={65_123} durationMs={125_456} />);
    expect(screen.getByText("01:05.123 / 02:05.456")).toBeInTheDocument();
  });

  it("calls onPlay / onPause on button click", () => {
    const onPlay = vi.fn();
    const onPause = vi.fn();
    const { rerender } = render(<ReplayControls {...base} onPlay={onPlay} onPause={onPause} />);
    fireEvent.click(screen.getByRole("button", { name: /play/i }));
    expect(onPlay).toHaveBeenCalled();
    rerender(<ReplayControls {...base} playing onPlay={onPlay} onPause={onPause} />);
    fireEvent.click(screen.getByRole("button", { name: /pause/i }));
    expect(onPause).toHaveBeenCalled();
  });

  it("Next jumps to the next event strictly after currentTimeMs", () => {
    const onSeek = vi.fn();
    const events: TimelineEvent[] = [
      { id: "a", kind: "user_message", timestamp: "2026-04-15T12:00:00.500Z", label: "" },
      { id: "b", kind: "tool_call", timestamp: "2026-04-15T12:00:01.200Z", label: "" },
    ];
    render(<ReplayControls {...base} events={events} currentTimeMs={800} onSeek={onSeek} />);
    fireEvent.click(screen.getByRole("button", { name: /next event/i }));
    expect(onSeek).toHaveBeenCalledWith(1200);
  });

  it("Prev jumps to the previous event strictly before currentTimeMs", () => {
    const onSeek = vi.fn();
    const events: TimelineEvent[] = [
      { id: "a", kind: "user_message", timestamp: "2026-04-15T12:00:00.500Z", label: "" },
      { id: "b", kind: "tool_call", timestamp: "2026-04-15T12:00:01.200Z", label: "" },
    ];
    render(<ReplayControls {...base} events={events} currentTimeMs={800} onSeek={onSeek} />);
    fireEvent.click(screen.getByRole("button", { name: /previous event/i }));
    expect(onSeek).toHaveBeenCalledWith(500);
  });

  it("Prev does not call onSeek when no earlier event exists", () => {
    const onSeek = vi.fn();
    const events: TimelineEvent[] = [
      { id: "a", kind: "user_message", timestamp: "2026-04-15T12:00:01.000Z", label: "" },
    ];
    render(<ReplayControls {...base} events={events} currentTimeMs={500} onSeek={onSeek} />);
    fireEvent.click(screen.getByRole("button", { name: /previous event/i }));
    expect(onSeek).not.toHaveBeenCalled();
  });

  it("Next does not call onSeek when no later event exists", () => {
    const onSeek = vi.fn();
    const events: TimelineEvent[] = [
      { id: "a", kind: "user_message", timestamp: "2026-04-15T12:00:00.500Z", label: "" },
    ];
    render(<ReplayControls {...base} events={events} currentTimeMs={800} onSeek={onSeek} />);
    fireEvent.click(screen.getByRole("button", { name: /next event/i }));
    expect(onSeek).not.toHaveBeenCalled();
  });
});
