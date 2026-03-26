import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useEventSource } from "./use-event-source";

// Mock EventSource
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

  // Test helpers
  simulateOpen() {
    this.readyState = 1;
    this.onopen?.(new Event("open"));
  }

  simulateMessage(data: string) {
    this.onmessage?.(new MessageEvent("message", { data }));
  }

  simulateError() {
    this.onerror?.(new Event("error"));
  }
}

// Install mock
const originalEventSource = globalThis.EventSource;

beforeEach(() => {
  MockEventSource.instances.length = 0;
  globalThis.EventSource = MockEventSource as never;
});

afterEach(() => {
  globalThis.EventSource = originalEventSource;
});

describe("useEventSource", () => {
  it("does not connect when url is null", () => {
    const { result } = renderHook(() => useEventSource<{ x: number }>(null));

    expect(result.current.data).toBeNull();
    expect(result.current.connected).toBe(false);
    expect(MockEventSource.instances).toHaveLength(0);
  });

  it("does not connect when disabled", () => {
    const { result } = renderHook(() =>
      useEventSource<{ x: number }>("/test", { enabled: false })
    );

    expect(result.current.data).toBeNull();
    expect(result.current.connected).toBe(false);
    expect(MockEventSource.instances).toHaveLength(0);
  });

  it("connects and receives data", () => {
    const { result } = renderHook(() =>
      useEventSource<{ value: number }>("/test")
    );

    expect(MockEventSource.instances).toHaveLength(1);
    const es = MockEventSource.instances[0];

    act(() => {
      es.simulateOpen();
    });

    expect(result.current.connected).toBe(true);

    act(() => {
      es.simulateMessage(JSON.stringify({ value: 42 }));
    });

    expect(result.current.data).toEqual({ value: 42 });
  });

  it("updates data on subsequent messages", () => {
    const { result } = renderHook(() =>
      useEventSource<{ count: number }>("/test")
    );

    const es = MockEventSource.instances[0];

    act(() => {
      es.simulateOpen();
      es.simulateMessage(JSON.stringify({ count: 1 }));
    });

    expect(result.current.data).toEqual({ count: 1 });

    act(() => {
      es.simulateMessage(JSON.stringify({ count: 2 }));
    });

    expect(result.current.data).toEqual({ count: 2 });
  });

  it("sets connected to false on error but does not close (allows native retry)", () => {
    const { result } = renderHook(() =>
      useEventSource<{ x: number }>("/test")
    );

    const es = MockEventSource.instances[0];

    act(() => {
      es.simulateOpen();
    });

    expect(result.current.connected).toBe(true);

    act(() => {
      es.simulateError();
    });

    expect(result.current.connected).toBe(false);
    // EventSource should NOT be closed — native retry will reconnect
    expect(es.closed).toBe(false);
  });

  it("closes on unmount", () => {
    const { unmount } = renderHook(() =>
      useEventSource<{ x: number }>("/test")
    );

    const es = MockEventSource.instances[0];

    unmount();

    expect(es.closed).toBe(true);
  });

  it("reconnects when url changes", () => {
    const { rerender } = renderHook(
      ({ url }: { url: string | null }) => useEventSource<{ x: number }>(url),
      { initialProps: { url: "/test-1" } }
    );

    expect(MockEventSource.instances).toHaveLength(1);
    const firstEs = MockEventSource.instances[0];
    expect(firstEs.url).toBe("/test-1");

    rerender({ url: "/test-2" });

    expect(firstEs.closed).toBe(true);
    expect(MockEventSource.instances).toHaveLength(2);
    expect(MockEventSource.instances[1].url).toBe("/test-2");
  });

  it("clears data when disabled", () => {
    const { result, rerender } = renderHook(
      ({ enabled }: { enabled: boolean }) =>
        useEventSource<{ x: number }>("/test", { enabled }),
      { initialProps: { enabled: true } }
    );

    const es = MockEventSource.instances[0];

    act(() => {
      es.simulateOpen();
      es.simulateMessage(JSON.stringify({ x: 1 }));
    });

    expect(result.current.data).toEqual({ x: 1 });

    rerender({ enabled: false });

    expect(result.current.data).toBeNull();
    expect(es.closed).toBe(true);
  });

  it("ignores malformed JSON gracefully", () => {
    const spy = vi.spyOn(console, "error").mockImplementation(() => {});
    const { result } = renderHook(() =>
      useEventSource<{ x: number }>("/test")
    );

    const es = MockEventSource.instances[0];

    act(() => {
      es.simulateOpen();
      es.simulateMessage("not-valid-json{{{");
    });

    expect(result.current.data).toBeNull();
    spy.mockRestore();
  });
});
