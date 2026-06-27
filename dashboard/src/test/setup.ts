import "@testing-library/jest-dom/vitest";
import * as matchers from "vitest-axe/matchers";
import { expect, vi } from "vitest";

// Extend vitest expect with axe matchers
expect.extend(matchers);

// Mock matchMedia
Object.defineProperty(globalThis, "matchMedia", {
  writable: true,
  value: vi.fn().mockImplementation((query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: vi.fn(),
    removeListener: vi.fn(),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn(),
  })),
});

// Mock ResizeObserver as a proper class
class MockResizeObserver {
  observe = vi.fn();
  unobserve = vi.fn();
  disconnect = vi.fn();
}
globalThis.ResizeObserver = MockResizeObserver as unknown as typeof ResizeObserver;

// Mock pointer capture methods for Radix UI components
Element.prototype.hasPointerCapture = vi.fn().mockReturnValue(false);
Element.prototype.setPointerCapture = vi.fn();
Element.prototype.releasePointerCapture = vi.fn();

// Mock scrollIntoView for Radix UI Select components
Element.prototype.scrollIntoView = vi.fn();

// Web Audio mocks (used by voice console hooks: useMicCapture / useAudioPlayback)
class FakeAudioWorkletNode {
  port = { onmessage: null as ((e: MessageEvent) => void) | null, postMessage: vi.fn() };
  connect = vi.fn();
  disconnect = vi.fn();
}
(globalThis as unknown as { AudioWorkletNode: unknown }).AudioWorkletNode = FakeAudioWorkletNode;

class FakeAudioContext {
  audioWorklet = { addModule: vi.fn().mockResolvedValue(undefined) };
  createMediaStreamSource = vi.fn().mockReturnValue({ connect: vi.fn(), disconnect: vi.fn() });
  createBuffer = vi.fn().mockImplementation((_channels: number, length: number, rate: number) => ({
    getChannelData: () => new Float32Array(length),
    duration: length / rate,
  }));
  createBufferSource = vi.fn().mockImplementation(() => ({
    buffer: null as AudioBuffer | null,
    connect: vi.fn(),
    start: vi.fn(),
    stop: vi.fn(),
    onended: null as (() => void) | null,
  }));
  close = vi.fn().mockResolvedValue(undefined);
  destination = {};
  sampleRate = 24000;
  state = "running";
  currentTime = 0;
}
const FakeAudioContextSpy = vi.fn().mockImplementation(function (this: FakeAudioContext) {
  return Object.assign(this, new FakeAudioContext());
});
(globalThis as unknown as { AudioContext: unknown }).AudioContext = FakeAudioContextSpy;

Object.defineProperty(globalThis.navigator, "mediaDevices", {
  configurable: true,
  value: { getUserMedia: vi.fn().mockResolvedValue({ getTracks: () => [{ stop: vi.fn() }] }) },
});
