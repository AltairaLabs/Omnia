import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { renderHook } from "@testing-library/react";
import { useArenaLiveStats } from "./use-arena-live-stats";

// Minimal mock EventSource
class MockEventSource {
  url: string;
  onopen: ((ev: Event) => void) | null = null;
  onmessage: ((ev: MessageEvent) => void) | null = null;
  onerror: ((ev: Event) => void) | null = null;
  readyState = 0;
  closed = false;

  static readonly instances: MockEventSource[] = [];

  constructor(url: string) {
    this.url = url;
    MockEventSource.instances.push(this);
  }

  close() {
    this.closed = true;
    this.readyState = 2;
  }
}

const originalEventSource = globalThis.EventSource;

beforeEach(() => {
  MockEventSource.instances.length = 0;
  globalThis.EventSource = MockEventSource as never;
});

afterEach(() => {
  globalThis.EventSource = originalEventSource;
});

describe("useArenaLiveStats", () => {
  it("connects to the correct SSE URL when enabled", () => {
    renderHook(() => useArenaLiveStats("my-workspace", "job-1", true));

    expect(MockEventSource.instances).toHaveLength(1);
    expect(MockEventSource.instances[0].url).toBe(
      "/api/workspaces/my-workspace/arena/jobs/job-1/live-stats"
    );
  });

  it("does not connect when disabled", () => {
    const { result } = renderHook(() =>
      useArenaLiveStats("my-workspace", "job-1", false)
    );

    expect(MockEventSource.instances).toHaveLength(0);
    expect(result.current.data).toBeNull();
    expect(result.current.connected).toBe(false);
  });

  it("encodes special characters in workspace and job name", () => {
    renderHook(() =>
      useArenaLiveStats("ws/special", "job name&test", true)
    );

    expect(MockEventSource.instances).toHaveLength(1);
    expect(MockEventSource.instances[0].url).toBe(
      "/api/workspaces/ws%2Fspecial/arena/jobs/job%20name%26test/live-stats"
    );
  });

  it("returns data and connected status from useEventSource", () => {
    const { result } = renderHook(() =>
      useArenaLiveStats("ws", "job", true)
    );

    // Initial state before any events
    expect(result.current.data).toBeNull();
    expect(result.current.connected).toBe(false);
  });
});
