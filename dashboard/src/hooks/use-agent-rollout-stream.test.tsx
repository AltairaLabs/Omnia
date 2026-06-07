import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook } from "@testing-library/react";
import {
  rolloutStreamUrl,
  useAgentRolloutStream,
  type RolloutStreamFrame,
} from "./use-agent-rollout-stream";

vi.mock("./use-event-source", () => ({ useEventSource: vi.fn() }));
import { useEventSource } from "./use-event-source";

describe("rolloutStreamUrl", () => {
  it("builds the SSE url, encoding workspace + agent", () => {
    expect(rolloutStreamUrl("my ws", "agent-1")).toBe(
      "/api/workspaces/my%20ws/agents/agent-1/rollout/stream",
    );
  });

  it("returns null when disabled or ids are missing", () => {
    expect(rolloutStreamUrl(undefined, "a")).toBeNull();
    expect(rolloutStreamUrl("ws", undefined)).toBeNull();
    expect(rolloutStreamUrl("ws", "a", false)).toBeNull();
  });
});

describe("useAgentRolloutStream", () => {
  const mockUseEventSource = vi.mocked(useEventSource);
  beforeEach(() => mockUseEventSource.mockReset());

  it("subscribes to the agent rollout stream and returns its data", () => {
    const frame: RolloutStreamFrame = {
      spec: { steps: [{ setWeight: 25 }] },
      status: { active: true, currentWeight: 25 },
    };
    mockUseEventSource.mockReturnValue({ data: frame, connected: true });
    const { result } = renderHook(() => useAgentRolloutStream("ws", "a"));
    expect(mockUseEventSource).toHaveBeenCalledWith("/api/workspaces/ws/agents/a/rollout/stream");
    expect(result.current).toBe(frame);
  });

  it("passes a null url (disabling the stream) when ids are missing", () => {
    mockUseEventSource.mockReturnValue({ data: null, connected: false });
    const { result } = renderHook(() => useAgentRolloutStream(undefined, "a"));
    expect(mockUseEventSource).toHaveBeenCalledWith(null);
    expect(result.current).toBeNull();
  });
});
