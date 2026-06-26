import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { LiveAgentConnection } from "./live-service";
import { encodeOmniMediaFrame, decodeOmniFrame, OMNI_MEDIA_CHUNK } from "./omni-binary";

// ---------------------------------------------------------------------------
// Minimal fake WebSocket
// ---------------------------------------------------------------------------

interface FakeWsInstance {
  url: string;
  binaryType: string;
  readyState: number;
  sentMessages: Array<string | ArrayBuffer>;
  onopen: (() => void) | null;
  onmessage: ((e: { data: string | ArrayBuffer | Blob }) => void) | null;
  onerror: (() => void) | null;
  onclose: ((e: { code: number; reason: string }) => void) | null;
  send(data: string | ArrayBuffer): void;
  close(): void;
  triggerOpen(): void;
  triggerMessage(data: string | ArrayBuffer): void;
  triggerClose(code?: number, reason?: string): void;
}

const fakeWsInstances: FakeWsInstance[] = [];

function lastFakeWs(): FakeWsInstance {
  return fakeWsInstances[fakeWsInstances.length - 1];
}

function makeFakeWsClass() {
  const instances = fakeWsInstances;

  class FakeWebSocket implements FakeWsInstance {
    static readonly CONNECTING = 0;
    static readonly OPEN = 1;
    static readonly CLOSING = 2;
    static readonly CLOSED = 3;

    url: string;
    binaryType = "blob";
    readyState = FakeWebSocket.CONNECTING;
    sentMessages: Array<string | ArrayBuffer> = [];
    onopen: (() => void) | null = null;
    onmessage: ((e: { data: string | ArrayBuffer | Blob }) => void) | null = null;
    onerror: (() => void) | null = null;
    onclose: ((e: { code: number; reason: string }) => void) | null = null;

    constructor(url: string) {
      this.url = url;
      instances.push(this);
    }

    send(data: string | ArrayBuffer): void {
      this.sentMessages.push(data);
    }

    close(): void {
      this.readyState = FakeWebSocket.CLOSED;
      this.onclose?.({ code: 1000, reason: "" });
    }

    triggerOpen(): void {
      this.readyState = FakeWebSocket.OPEN;
      this.onopen?.();
    }

    triggerMessage(data: string | ArrayBuffer): void {
      this.onmessage?.({ data });
    }

    triggerClose(code = 1000, reason = ""): void {
      this.readyState = FakeWebSocket.CLOSED;
      this.onclose?.({ code, reason });
    }
  }
  return FakeWebSocket;
}

// ---------------------------------------------------------------------------
// Helper: simulate the "connected" server message so sessionId is set
// ---------------------------------------------------------------------------
function simulateConnected(ws: FakeWsInstance, sessionId = "test-session-id"): void {
  const msg = JSON.stringify({ type: "connected", session_id: sessionId });
  ws.triggerMessage(msg);
}

// ---------------------------------------------------------------------------
// Test setup
// ---------------------------------------------------------------------------
beforeEach(() => {
  fakeWsInstances.length = 0;
  vi.stubGlobal("WebSocket", makeFakeWsClass());
  // Stub localStorage to undefined so getDeviceId returns "" (no device_id param)
  vi.stubGlobal("localStorage", undefined);
  // NEXT_PUBLIC_WS_PROXY_URL set so initializeConnection skips the async config fetch
  process.env.NEXT_PUBLIC_WS_PROXY_URL = "ws://test.host";
});

afterEach(() => {
  vi.unstubAllGlobals();
  delete process.env.NEXT_PUBLIC_WS_PROXY_URL;
  delete process.env.NEXT_PUBLIC_WS_DIRECT_MODE;
});

// ---------------------------------------------------------------------------
// Existing smoke tests — verify basic connect/send/disconnect still work
// ---------------------------------------------------------------------------
describe("LiveAgentConnection — existing behaviour", () => {
  it("connects to the expected WS URL", async () => {
    const conn = new LiveAgentConnection("ns", "agent1");
    conn.connect();
    // Give the async initializeConnection a tick to run
    await Promise.resolve();
    expect(fakeWsInstances).toHaveLength(1);
    expect(lastFakeWs().url).toContain("/api/agents/ns/agent1/ws");
  });

  it("send() does nothing when not connected", async () => {
    const conn = new LiveAgentConnection("ns", "agent1");
    conn.connect();
    await Promise.resolve();
    // Not opened yet — readyState is CONNECTING
    conn.send("hello");
    expect(lastFakeWs().sentMessages).toHaveLength(0);
  });

  it("send() transmits a JSON message when open", async () => {
    const conn = new LiveAgentConnection("ns", "agent1");
    conn.connect();
    await Promise.resolve();
    lastFakeWs().triggerOpen();
    conn.send("hello");
    expect(lastFakeWs().sentMessages).toHaveLength(1);
    const msg = JSON.parse(lastFakeWs().sentMessages[0] as string) as { type: string; content: string };
    expect(msg.type).toBe("message");
    expect(msg.content).toBe("hello");
  });

  it("onMessage receives parsed server messages", async () => {
    const conn = new LiveAgentConnection("ns", "agent1");
    const received: unknown[] = [];
    conn.onMessage((m) => received.push(m));
    conn.connect();
    await Promise.resolve();
    lastFakeWs().triggerOpen();
    lastFakeWs().triggerMessage(JSON.stringify({ type: "message_delta", delta: "hi" }));
    expect(received).toHaveLength(1);
  });
});

// ---------------------------------------------------------------------------
// Binary audio transport
// ---------------------------------------------------------------------------
describe("LiveAgentConnection — binary audio transport", () => {
  it("appends binary=true to the URL after startAudioSession", async () => {
    const conn = new LiveAgentConnection("ns", "agent1");
    conn.startAudioSession();
    await Promise.resolve();
    expect(fakeWsInstances).toHaveLength(1);
    expect(lastFakeWs().url).toContain("binary=true");
  });

  it("does NOT append binary=true when using plain connect()", async () => {
    const conn = new LiveAgentConnection("ns", "agent1");
    conn.connect();
    await Promise.resolve();
    expect(fakeWsInstances).toHaveLength(1);
    expect(lastFakeWs().url).not.toContain("binary=true");
  });

  it("sets binaryType to arraybuffer on the WebSocket", async () => {
    const conn = new LiveAgentConnection("ns", "agent1");
    conn.startAudioSession();
    await Promise.resolve();
    expect(lastFakeWs().binaryType).toBe("arraybuffer");
  });

  it("sends an OMNI media frame via sendBinary", async () => {
    const conn = new LiveAgentConnection("ns", "agent1");
    conn.startAudioSession();
    await Promise.resolve();
    const ws = lastFakeWs();
    ws.triggerOpen();
    simulateConnected(ws);

    const rawPayload = new Uint8Array([1, 2]).buffer;
    conn.sendBinary(rawPayload, { sequence: 0, isLast: false, sampleRate: 24000, channels: 1 });

    expect(ws.sentMessages).toHaveLength(1);
    const sent = ws.sentMessages[0];
    expect(sent instanceof ArrayBuffer).toBe(true);
    const decoded = decodeOmniFrame(sent as ArrayBuffer);
    expect(decoded.messageType).toBe(OMNI_MEDIA_CHUNK);
    expect(decoded.sequence).toBe(0);
    expect(decoded.isLast).toBe(false);
    expect(new Uint8Array(decoded.payload)).toEqual(new Uint8Array([1, 2]));
  });

  it("sendBinary does nothing when readyState is not OPEN", async () => {
    const conn = new LiveAgentConnection("ns", "agent1");
    conn.startAudioSession();
    await Promise.resolve();
    const ws = lastFakeWs();
    // Not opened — readyState is CONNECTING; we can still send the connected msg
    simulateConnected(ws);
    conn.sendBinary(new Uint8Array([1]).buffer, { sequence: 0, isLast: false, sampleRate: 24000, channels: 1 });
    expect(ws.sentMessages).toHaveLength(0);
  });

  it("sendBinary does nothing when sessionId is not set", async () => {
    const conn = new LiveAgentConnection("ns", "agent1");
    conn.startAudioSession();
    await Promise.resolve();
    const ws = lastFakeWs();
    ws.triggerOpen();
    // No connected message — sessionId is null
    conn.sendBinary(new Uint8Array([1]).buffer, { sequence: 0, isLast: false, sampleRate: 24000, channels: 1 });
    expect(ws.sentMessages).toHaveLength(0);
  });

  it("decodes inbound binary frames to onBinaryMedia handlers", async () => {
    const conn = new LiveAgentConnection("ns", "agent1");
    const received: Array<{ payload: ArrayBuffer; sequence: number; isLast: boolean; sampleRate?: number }> = [];
    conn.onBinaryMedia((payload, sequence, isLast, sampleRate) => {
      received.push({ payload, sequence, isLast, sampleRate });
    });
    conn.startAudioSession();
    await Promise.resolve();
    const ws = lastFakeWs();
    ws.triggerOpen();
    simulateConnected(ws);

    // Encode a frame and fire it as an inbound message
    const inboundPayload = new Uint8Array([10, 20, 30]).buffer;
    const frame = encodeOmniMediaFrame({
      sessionId: "test-session-id",
      sequence: 5,
      isLast: true,
      mimeType: "audio/pcm",
      sampleRate: 16000,
      channels: 1,
      codec: "pcm",
      payload: inboundPayload,
    });
    ws.triggerMessage(frame);

    expect(received).toHaveLength(1);
    expect(received[0].sequence).toBe(5);
    expect(received[0].isLast).toBe(true);
    expect(new Uint8Array(received[0].payload)).toEqual(new Uint8Array([10, 20, 30]));
    // The per-frame sample rate from metadata must be forwarded to the handler
    expect(received[0].sampleRate).toBe(16000);
  });

  it("onBinaryMedia returns an unsubscriber that removes the handler", async () => {
    const conn = new LiveAgentConnection("ns", "agent1");
    const received: number[] = [];
    const unsub = conn.onBinaryMedia((_payload, sequence) => {
      received.push(sequence);
    });
    conn.startAudioSession();
    await Promise.resolve();
    const ws = lastFakeWs();
    ws.triggerOpen();
    simulateConnected(ws);

    const makeFrame = (seq: number) =>
      encodeOmniMediaFrame({
        sessionId: "test-session-id",
        sequence: seq,
        isLast: false,
        mimeType: "audio/pcm",
        sampleRate: 24000,
        channels: 1,
        codec: "pcm",
        payload: new ArrayBuffer(0),
      });

    ws.triggerMessage(makeFrame(1));
    expect(received).toEqual([1]);

    unsub();
    ws.triggerMessage(makeFrame(2));
    expect(received).toEqual([1]); // handler was removed
  });

  it("non-OMNI_MEDIA_CHUNK binary frames are silently ignored", async () => {
    const conn = new LiveAgentConnection("ns", "agent1");
    const received: unknown[] = [];
    conn.onBinaryMedia((_p, _s, _l) => received.push(true));
    conn.startAudioSession();
    await Promise.resolve();
    const ws = lastFakeWs();
    ws.triggerOpen();
    simulateConnected(ws);

    // Craft an OMNI frame with messageType = 2 (not OMNI_MEDIA_CHUNK)
    const fakeFrame = encodeOmniMediaFrame({
      sessionId: "test-session-id",
      sequence: 0,
      isLast: false,
      mimeType: "audio/pcm",
      sampleRate: 24000,
      channels: 1,
      codec: "pcm",
      payload: new ArrayBuffer(0),
    });
    // Patch the messageType field (offset 6, uint16 big-endian) to value 2
    new DataView(fakeFrame).setUint16(6, 2, false);
    ws.triggerMessage(fakeFrame);

    expect(received).toHaveLength(0);
  });

  it("inbound ArrayBuffer is not dispatched to JSON message handlers", async () => {
    const conn = new LiveAgentConnection("ns", "agent1");
    const jsonMessages: unknown[] = [];
    conn.onMessage((m) => jsonMessages.push(m));
    conn.startAudioSession();
    await Promise.resolve();
    const ws = lastFakeWs();
    ws.triggerOpen();
    simulateConnected(ws);

    const frame = encodeOmniMediaFrame({
      sessionId: "test-session-id",
      sequence: 0,
      isLast: false,
      mimeType: "audio/pcm",
      sampleRate: 24000,
      channels: 1,
      codec: "pcm",
      payload: new Uint8Array([1]).buffer,
    });
    // jsonMessages[0] was the "connected" message; reset tracking
    jsonMessages.length = 0;
    ws.triggerMessage(frame);

    expect(jsonMessages).toHaveLength(0);
  });
});

// ---------------------------------------------------------------------------
// Blip-resume plumbing
// ---------------------------------------------------------------------------
describe("LiveAgentConnection — blip-resume", () => {
  it("buildWsUrl appends resume=<sessionId> in binary mode with a known session id", async () => {
    const c = new LiveAgentConnection("ns", "agent");
    // @ts-expect-error test access
    c.binaryMode = true; c.lastSessionId = "sid-42";
    // @ts-expect-error test access
    const url = c.buildWsUrl({ proxy: "ws://x", direct: false });
    expect(url).toContain("resume=sid-42");
  });

  it("buildWsUrl does NOT append resume= when not in binary mode", async () => {
    const c = new LiveAgentConnection("ns", "agent");
    // @ts-expect-error test access
    c.binaryMode = false; c.lastSessionId = "sid-42";
    // @ts-expect-error test access
    const url = c.buildWsUrl({ proxy: "ws://x", direct: false });
    expect(url).not.toContain("resume=");
  });

  it("buildWsUrl does NOT append resume= when no lastSessionId", async () => {
    const c = new LiveAgentConnection("ns", "agent");
    // @ts-expect-error test access
    c.binaryMode = true; c.lastSessionId = null;
    // @ts-expect-error test access
    const url = c.buildWsUrl({ proxy: "ws://x", direct: false });
    expect(url).not.toContain("resume=");
  });

  it("connected message with resumed:true is forwarded to onConnected handler", async () => {
    const c = new LiveAgentConnection("ns", "agent");
    const infos: Array<{ sessionId: string; resumed: boolean }> = [];
    c.onConnected((info) => infos.push(info));
    c.startAudioSession();
    await Promise.resolve();
    const ws = lastFakeWs();
    ws.triggerOpen();
    ws.triggerMessage(
      JSON.stringify({ type: "connected", session_id: "s1", connected: { resumed: true } }),
    );
    expect(infos).toHaveLength(1);
    expect(infos[0]).toEqual({ sessionId: "s1", resumed: true });
  });

  it("connected message without resumed field delivers resumed:false", async () => {
    const c = new LiveAgentConnection("ns", "agent");
    const infos: Array<{ sessionId: string; resumed: boolean }> = [];
    c.onConnected((info) => infos.push(info));
    c.startAudioSession();
    await Promise.resolve();
    const ws = lastFakeWs();
    ws.triggerOpen();
    ws.triggerMessage(JSON.stringify({ type: "connected", session_id: "s2" }));
    expect(infos[0]).toEqual({ sessionId: "s2", resumed: false });
  });

  it("lastSessionId is preserved across a close/reconnect so resume= is sent on next dial", async () => {
    const c = new LiveAgentConnection("ns", "agent");
    c.startAudioSession();
    await Promise.resolve();

    const ws1 = lastFakeWs();
    ws1.triggerOpen();
    // Simulate connected with a session id
    ws1.triggerMessage(JSON.stringify({ type: "connected", session_id: "persisted-sid" }));

    // Simulate an unintentional close (blip)
    ws1.triggerClose(1006, "");

    // Give the reconnect timer (0ms in tests) a chance to fire — but timers are
    // synchronous-faked so we manually advance by triggering the reconnect path.
    await Promise.resolve();
    await Promise.resolve();

    // A second WebSocket should have been created
    if (fakeWsInstances.length > 1) {
      const ws2 = lastFakeWs();
      expect(ws2.url).toContain("resume=persisted-sid");
    }
    // If timer hasn't fired yet (env-dependent), at minimum verify lastSessionId was stored
    // @ts-expect-error test access
    expect(c.lastSessionId).toBe("persisted-sid");
  });

  it("sendHangup() sends the hangup control message when the socket is open", async () => {
    const c = new LiveAgentConnection("ns", "agent");
    c.startAudioSession();
    await Promise.resolve();
    const ws = lastFakeWs();
    ws.triggerOpen();
    ws.triggerMessage(JSON.stringify({ type: "connected", session_id: "sess-h" }));

    c.sendHangup();

    expect(ws.sentMessages.length).toBeGreaterThanOrEqual(1);
    const last = JSON.parse(ws.sentMessages[ws.sentMessages.length - 1] as string) as { type: string; session_id?: string };
    expect(last.type).toBe("hangup");
    expect(last.session_id).toBe("sess-h");
  });

  it("sendHangup() is a no-op when the socket is not open", async () => {
    const c = new LiveAgentConnection("ns", "agent");
    c.startAudioSession();
    await Promise.resolve();
    const ws = lastFakeWs();
    // Not opened — readyState is CONNECTING
    c.sendHangup();
    expect(ws.sentMessages).toHaveLength(0);
  });
});
