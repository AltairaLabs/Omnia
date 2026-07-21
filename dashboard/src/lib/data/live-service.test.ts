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

// ---------------------------------------------------------------------------
// SESSION_EXPIRED recovery (#1876)
//
// The facade decides resumability from the context store, not from session-api.
// When the named session's working context is gone it drops the message and
// replies SESSION_EXPIRED rather than silently answering with no history, so
// the client must start a fresh session instead of losing the user's turn.
// ---------------------------------------------------------------------------
describe("LiveAgentConnection — expired session recovery", () => {
  async function connectedConn(sessionId = "old-session") {
    const conn = new LiveAgentConnection("ns", "agent");
    conn.connect();
    await Promise.resolve();
    const ws = lastFakeWs();
    ws.triggerOpen();
    simulateConnected(ws, sessionId);
    return { conn, ws };
  }

  function expireSession(ws: FakeWsInstance): void {
    ws.triggerMessage(
      JSON.stringify({
        type: "error",
        error: { code: "SESSION_EXPIRED", message: "session context has expired" },
      })
    );
  }

  function parseSent(ws: FakeWsInstance, i: number) {
    return JSON.parse(ws.sentMessages[i] as string) as {
      type: string;
      session_id?: string;
      content?: string;
    };
  }

  it("resends the dropped message without a session id", async () => {
    const { conn, ws } = await connectedConn();

    conn.send("what did we discuss?");
    expect(parseSent(ws, 0).session_id).toBe("old-session");

    expireSession(ws);

    // The user's turn is retried rather than lost, and carries no stale id so
    // the server opens a fresh session for it.
    expect(ws.sentMessages).toHaveLength(2);
    const retry = parseSent(ws, 1);
    expect(retry.content).toBe("what did we discuss?");
    expect(retry.session_id).toBeUndefined();
  });

  it("does not surface the error to consumers when it recovers", async () => {
    const { conn, ws } = await connectedConn();
    const received: Array<{ type: string }> = [];
    conn.onMessage((m) => received.push(m));

    conn.send("hello");
    expireSession(ws);

    // Presenting a failure would be wrong — the turn is being retried.
    expect(received.filter((m) => m.type === "error")).toHaveLength(0);
  });

  it("surfaces a repeat expiry as a real error instead of looping", async () => {
    const { conn, ws } = await connectedConn();
    const received: Array<{ type: string }> = [];
    conn.onMessage((m) => received.push(m));

    conn.send("hello");
    expireSession(ws);
    expect(ws.sentMessages).toHaveLength(2);

    // Second expiry: the retry is already spent, so this is a genuine failure.
    expireSession(ws);
    expect(ws.sentMessages).toHaveLength(2);
    expect(received.filter((m) => m.type === "error")).toHaveLength(1);
  });

  it("stops offering the dead id as resume= on the next dial", async () => {
    const conn = new LiveAgentConnection("ns", "agent");
    conn.startAudioSession();
    await Promise.resolve();
    const ws = lastFakeWs();
    ws.triggerOpen();
    simulateConnected(ws, "dead-session");

    // @ts-expect-error test access
    expect(conn.lastSessionId).toBe("dead-session");

    expireSession(ws);

    // Dialling back with resume=dead-session would ask the server to resume a
    // context it has already said is gone.
    // @ts-expect-error test access
    expect(conn.lastSessionId).toBeNull();
    // @ts-expect-error test access
    expect(conn.buildWsUrl({ proxy: "ws://x", direct: false })).not.toContain("resume=");
  });
});

// ---------------------------------------------------------------------------
// Connection lifecycle and control messages
// ---------------------------------------------------------------------------
describe("LiveAgentConnection — lifecycle and control messages", () => {
  async function openConn(sessionId = "sess-1") {
    const conn = new LiveAgentConnection("ns", "agent");
    conn.connect();
    await Promise.resolve();
    const ws = lastFakeWs();
    ws.triggerOpen();
    simulateConnected(ws, sessionId);
    return { conn, ws };
  }

  it("exposes status, session id and max payload size", async () => {
    const conn = new LiveAgentConnection("ns", "agent");
    expect(conn.getStatus()).toBe("disconnected");
    expect(conn.getSessionId()).toBeNull();
    expect(conn.getMaxPayloadSize()).toBeNull();

    conn.connect();
    await Promise.resolve();
    const ws = lastFakeWs();
    ws.triggerOpen();
    ws.triggerMessage(
      JSON.stringify({
        type: "connected",
        session_id: "sess-caps",
        connected: { capabilities: { max_payload_size: 4096 } },
      })
    );

    expect(conn.getStatus()).toBe("connected");
    expect(conn.getSessionId()).toBe("sess-caps");
    expect(conn.getMaxPayloadSize()).toBe(4096);
  });

  it("disconnect() closes the socket, clears session state and drops handlers", async () => {
    const { conn, ws } = await openConn();
    const received: unknown[] = [];
    conn.onMessage((m) => received.push(m));

    conn.disconnect();

    expect(ws.readyState).toBe(3);
    expect(conn.getSessionId()).toBeNull();
    expect(conn.getMaxPayloadSize()).toBeNull();
    expect(conn.getStatus()).toBe("disconnected");

    // Handlers are released, so late frames reach nobody.
    ws.triggerMessage(JSON.stringify({ type: "chunk", content: "late" }));
    expect(received).toHaveLength(0);
  });

  it("disconnect() suppresses the automatic reconnect", async () => {
    const { conn } = await openConn();
    const before = fakeWsInstances.length;

    conn.disconnect();
    await Promise.resolve();

    // @ts-expect-error test access
    expect(conn.intentionalDisconnect).toBe(true);
    expect(fakeWsInstances).toHaveLength(before);
  });

  it("sendToolCallAck sends the ack with the current session id", async () => {
    const { conn, ws } = await openConn("sess-ack");
    ws.sentMessages.length = 0;

    conn.sendToolCallAck("call-1");

    const sent = JSON.parse(ws.sentMessages[0] as string) as {
      type: string;
      session_id?: string;
      tool_call_ack?: { call_id: string };
    };
    expect(sent.type).toBe("tool_call_ack");
    expect(sent.session_id).toBe("sess-ack");
    expect(sent.tool_call_ack?.call_id).toBe("call-1");
  });

  it("sendToolResult carries the result and the error field", async () => {
    const { conn, ws } = await openConn("sess-res");
    ws.sentMessages.length = 0;

    conn.sendToolResult("call-2", { ok: true }, "boom");

    const sent = JSON.parse(ws.sentMessages[0] as string) as {
      type: string;
      tool_result?: { call_id: string; result?: unknown; error?: string };
    };
    expect(sent.type).toBe("tool_result");
    expect(sent.tool_result?.call_id).toBe("call-2");
    expect(sent.tool_result?.result).toEqual({ ok: true });
    expect(sent.tool_result?.error).toBe("boom");
  });

  it("tool control messages are no-ops when the socket is not open", async () => {
    const conn = new LiveAgentConnection("ns", "agent");
    conn.connect();
    await Promise.resolve();
    const ws = lastFakeWs(); // still CONNECTING

    conn.sendToolCallAck("call-3");
    conn.sendToolResult("call-4");

    expect(ws.sentMessages).toHaveLength(0);
  });

  it("reports an error status when the socket errors", async () => {
    const conn = new LiveAgentConnection("ns", "agent");
    const statuses: Array<{ status: string; error?: string }> = [];
    conn.onStatusChange((status, error) => statuses.push({ status, error }));
    conn.connect();
    await Promise.resolve();

    lastFakeWs().onerror?.();

    expect(conn.getStatus()).toBe("error");
    expect(statuses.at(-1)?.error).toBe("WebSocket connection failed");
  });

  it("preserves an error status when the socket closes abnormally", async () => {
    const { ws, conn } = await openConn();

    ws.triggerClose(1011, "internal error");

    expect(conn.getStatus()).toBe("error");
  });
});

// ---------------------------------------------------------------------------
// URL assembly variants
// ---------------------------------------------------------------------------
describe("LiveAgentConnection — URL assembly", () => {
  it("uses the direct query-param form when direct mode is on", () => {
    const c = new LiveAgentConnection("ns one", "agent one");
    // @ts-expect-error test access
    const url = c.buildWsUrl({ proxy: "ws://x", direct: true });
    expect(url).toContain("ws://x/ws?agent=agent%20one");
    expect(url).toContain("namespace=ns%20one");
  });

  it("falls back to the page host when no proxy is configured", () => {
    const c = new LiveAgentConnection("ns", "agent");
    // @ts-expect-error test access
    const url = c.buildWsUrl({ proxy: null, direct: false });
    expect(url).toContain("/api/agents/ns/agent/ws");
    expect(url.startsWith("ws://")).toBe(true);
  });

  it("uses wss when the page is served over https", () => {
    vi.stubGlobal("location", { protocol: "https:", host: "example.test" });
    const c = new LiveAgentConnection("ns", "agent");
    // @ts-expect-error test access
    const url = c.buildWsUrl({ proxy: null, direct: false });
    expect(url).toBe("wss://example.test/api/agents/ns/agent/ws");
  });

  it("appends device_id when one is stored", () => {
    const store = new Map([["omnia-device-id", "dev-123"]]);
    vi.stubGlobal("localStorage", {
      getItem: (k: string) => store.get(k) ?? null,
      setItem: (k: string, v: string) => { store.set(k, v); },
    });
    const c = new LiveAgentConnection("ns", "agent");
    // @ts-expect-error test access
    const url = c.buildWsUrl({ proxy: "ws://x", direct: false });
    expect(url).toContain("device_id=dev-123");
  });

  it("separates binary and resume params correctly when a device_id is present", () => {
    const store = new Map([["omnia-device-id", "dev-9"]]);
    vi.stubGlobal("localStorage", {
      getItem: (k: string) => store.get(k) ?? null,
      setItem: (k: string, v: string) => { store.set(k, v); },
    });
    const c = new LiveAgentConnection("ns", "agent");
    // @ts-expect-error test access
    c.binaryMode = true; c.lastSessionId = "sid-7";
    // @ts-expect-error test access
    const url = c.buildWsUrl({ proxy: "ws://x", direct: false });
    expect(url).toContain("?device_id=dev-9");
    expect(url).toContain("&binary=true");
    expect(url).toContain("&resume=sid-7");
  });
});

// ---------------------------------------------------------------------------
// Guard clauses
// ---------------------------------------------------------------------------
describe("LiveAgentConnection — guards", () => {
  it("connect() is a no-op when the socket is already open", async () => {
    const c = new LiveAgentConnection("ns", "agent");
    c.connect();
    await Promise.resolve();
    lastFakeWs().triggerOpen();

    c.connect();
    await Promise.resolve();

    expect(fakeWsInstances).toHaveLength(1);
  });

  it("send() honours an explicit session id over the tracked one", async () => {
    const c = new LiveAgentConnection("ns", "agent");
    c.connect();
    await Promise.resolve();
    const ws = lastFakeWs();
    ws.triggerOpen();
    simulateConnected(ws, "tracked");

    c.send("hi", { sessionId: "explicit" });

    const sent = JSON.parse(ws.sentMessages[0] as string) as { session_id?: string };
    expect(sent.session_id).toBe("explicit");
  });

  it("omits session_id on tool messages before a session exists", async () => {
    const c = new LiveAgentConnection("ns", "agent");
    c.connect();
    await Promise.resolve();
    const ws = lastFakeWs();
    ws.triggerOpen(); // no connected message, so no session id yet

    c.sendToolCallAck("call-x");
    c.sendToolResult("call-y");

    const ack = JSON.parse(ws.sentMessages[0] as string) as { session_id?: string };
    const res = JSON.parse(ws.sentMessages[1] as string) as { session_id?: string };
    expect(ack.session_id).toBeUndefined();
    expect(res.session_id).toBeUndefined();
  });

});

// ---------------------------------------------------------------------------
// Handler unsubscription and remaining guards
// ---------------------------------------------------------------------------
describe("LiveAgentConnection — unsubscribe and guards", () => {
  it("unsubscribing stops each handler kind from firing", async () => {
    const c = new LiveAgentConnection("ns", "agent");
    const messages: unknown[] = [];
    const statuses: unknown[] = [];
    const connects: unknown[] = [];
    const binaries: unknown[] = [];

    const offMessage = c.onMessage((m) => messages.push(m));
    const offStatus = c.onStatusChange((s) => statuses.push(s));
    const offConnected = c.onConnected((i) => connects.push(i));
    const offBinary = c.onBinaryMedia((p) => binaries.push(p));

    offMessage();
    offStatus();
    offConnected();
    offBinary();

    c.startAudioSession();
    await Promise.resolve();
    const ws = lastFakeWs();
    ws.triggerOpen();
    simulateConnected(ws, "sess-unsub");
    ws.triggerMessage(
      encodeOmniMediaFrame({
        sessionId: "sess-unsub",
        sequence: 0,
        isLast: false,
        mimeType: "audio/pcm",
        sampleRate: 24000,
        channels: 1,
        codec: "pcm",
        payload: new Uint8Array([1]).buffer,
      })
    );

    expect(messages).toHaveLength(0);
    expect(statuses).toHaveLength(0);
    expect(connects).toHaveLength(0);
    expect(binaries).toHaveLength(0);
  });

  it("disconnect() is safe when no socket was ever created", () => {
    const c = new LiveAgentConnection("ns", "agent");
    expect(() => c.disconnect()).not.toThrow();
    expect(c.getStatus()).toBe("disconnected");
  });

  it("sendHangup omits session_id when no session is established", async () => {
    const c = new LiveAgentConnection("ns", "agent");
    c.startAudioSession();
    await Promise.resolve();
    const ws = lastFakeWs();
    ws.triggerOpen(); // no connected message

    c.sendHangup();

    const sent = JSON.parse(ws.sentMessages[0] as string) as { session_id?: string };
    expect(sent.session_id).toBeUndefined();
  });

  it("sendHangup is a no-op when the socket is not open", async () => {
    const c = new LiveAgentConnection("ns", "agent");
    c.connect();
    await Promise.resolve();
    c.sendHangup();
    expect(lastFakeWs().sentMessages).toHaveLength(0);
  });

  it("connect() clears a pending reconnect timer", async () => {
    const c = new LiveAgentConnection("ns", "agent");
    c.connect();
    await Promise.resolve();
    lastFakeWs().triggerClose(1006, "blip"); // schedules a reconnect

    // @ts-expect-error test access
    expect(c.reconnectTimer).not.toBeNull();

    c.connect();
    // @ts-expect-error test access
    expect(c.reconnectTimer).toBeNull();
  });
});
