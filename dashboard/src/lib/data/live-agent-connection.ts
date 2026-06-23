/**
 * LiveAgentConnection — WebSocket connection class for the Omnia agent facade.
 *
 * Handles connect/disconnect, binary audio transport, reconnect with
 * exponential back-off, and blip-resume session tracking.
 */

import type { AgentConnection } from "./types";
import type {
  ServerMessage,
  ConnectionStatus,
  ClientMessage,
  ContentPart,
} from "@/types/websocket";
import { getWsProxyUrl } from "@/lib/config";
import { getDeviceId } from "@/lib/device-id";
import { encodeOmniMediaFrame, decodeOmniFrame, OMNI_MEDIA_CHUNK } from "./omni-binary";

/** Initial reconnection delay in milliseconds */
export const RECONNECT_BASE_DELAY_MS = 1000;
/** Maximum reconnection delay in milliseconds */
export const RECONNECT_MAX_DELAY_MS = 30000;

/** Info delivered to onConnected callbacks */
export interface ConnectedEventInfo {
  sessionId: string;
  resumed: boolean;
}

/**
 * Live agent connection using real WebSocket.
 * Connects through the dashboard's WebSocket proxy to the agent's facade.
 */
export class LiveAgentConnection implements AgentConnection {
  private status: ConnectionStatus = "disconnected";
  private sessionId: string | null = null;
  /** Last session id seen from a connected message; survives the close so resume= can be sent. */
  private lastSessionId: string | null = null;
  private maxPayloadSize: number | null = null;
  private ws: WebSocket | null = null;
  private readonly messageHandlers: Array<(message: ServerMessage) => void> = [];
  private readonly statusHandlers: Array<(status: ConnectionStatus, error?: string) => void> = [];
  private readonly connectedHandlers: Array<(info: ConnectedEventInfo) => void> = [];
  private reconnectDelay: number = RECONNECT_BASE_DELAY_MS;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private intentionalDisconnect = false;
  private binaryMode = false;
  private readonly binaryHandlers: Array<(payload: ArrayBuffer, sequence: number, isLast: boolean, sampleRate?: number) => void> = [];

  constructor(
    private readonly namespace: string,
    private readonly agentName: string
  ) {}

  connect(): void {
    if (this.ws?.readyState === WebSocket.OPEN) {
      return; // Already connected
    }

    this.intentionalDisconnect = false;
    this.clearReconnectTimer();
    this.setStatus("connecting");

    // Fetch runtime config and then connect
    this.initializeConnection().catch((err) => {
      console.error("Failed to initialize WebSocket connection:", err);
      this.setStatus("error", err instanceof Error ? err.message : "Failed to connect");
    });
  }

  /**
   * Assembles the WebSocket URL from a resolved proxy URL and connection mode flags.
   * Extracted so URL assembly (including resume= injection) is unit-testable.
   */
  private buildWsUrl(opts: { proxy: string | null; direct: boolean }): string {
    const protocol = typeof globalThis !== "undefined" && globalThis.location?.protocol === "https:" ? "wss:" : "ws:";
    const { proxy, direct } = opts;

    let wsUrl: string;
    if (proxy && direct) {
      wsUrl = `${proxy}/ws?agent=${encodeURIComponent(this.agentName)}&namespace=${encodeURIComponent(this.namespace)}`;
    } else if (proxy) {
      wsUrl = `${proxy}/api/agents/${this.namespace}/${this.agentName}/ws`;
    } else {
      const wsHost = typeof globalThis !== "undefined" && globalThis.location ? globalThis.location.host : "localhost:3002";
      wsUrl = `${protocol}//${wsHost}/api/agents/${this.namespace}/${this.agentName}/ws`;
    }

    const deviceId = getDeviceId();
    if (deviceId) {
      const sep = wsUrl.includes("?") ? "&" : "?";
      wsUrl += `${sep}device_id=${encodeURIComponent(deviceId)}`;
    }

    if (this.binaryMode) {
      const sep = wsUrl.includes("?") ? "&" : "?";
      wsUrl += `${sep}binary=true`;
    }

    if (this.binaryMode && this.lastSessionId) {
      const sep = wsUrl.includes("?") ? "&" : "?";
      wsUrl += `${sep}resume=${encodeURIComponent(this.lastSessionId)}`;
    }

    return wsUrl;
  }

  private async initializeConnection(): Promise<void> {
    try {
      // Check build-time env var first, then fall back to runtime config
      let wsProxyUrl = process.env.NEXT_PUBLIC_WS_PROXY_URL;
      if (!wsProxyUrl) {
        // Fetch from runtime config (needed for K8s deployments where config comes from ConfigMap)
        wsProxyUrl = await getWsProxyUrl();
      }
      const wsDirectMode = process.env.NEXT_PUBLIC_WS_DIRECT_MODE === "true";

      const wsUrl = this.buildWsUrl({ proxy: wsProxyUrl ?? null, direct: wsDirectMode });

      this.ws = new WebSocket(wsUrl);
      this.ws.binaryType = "arraybuffer";

      this.ws.onopen = () => {
        this.reconnectDelay = RECONNECT_BASE_DELAY_MS;
        this.setStatus("connected");
      };

      this.ws.onmessage = async (event) => {
        if (event.data instanceof ArrayBuffer) {
          this.handleBinary(event.data);
          return;
        }
        try {
          // Handle both string and Blob data
          let data: string;
          if (event.data instanceof Blob) {
            data = await event.data.text();
          } else {
            data = event.data as string;
          }

          const message: ServerMessage = JSON.parse(data);

          // Track session ID and capabilities from connected message
          if (message.type === "connected") {
            if (message.session_id) {
              this.sessionId = message.session_id;
              this.lastSessionId = message.session_id;
            }
            // Extract max payload size from server capabilities
            if (message.connected?.capabilities?.max_payload_size) {
              this.maxPayloadSize = message.connected.capabilities.max_payload_size;
            }
            // Emit connected event with resumed flag
            const resumed = message.connected?.resumed === true;
            const sessionId = message.session_id ?? "";
            this.emitConnected({ sessionId, resumed });
          }

          this.emitMessage(message);
        } catch (err) {
          console.error("Failed to parse WebSocket message:", err);
        }
      };

      this.ws.onerror = () => {
        // WebSocket errors don't expose details for security reasons
        console.warn("[LiveAgentConnection] WebSocket connection failed");
        this.setStatus("error", "WebSocket connection failed");
      };

      this.ws.onclose = (event) => {
        console.warn("[LiveAgentConnection] WebSocket closed:", event.code, event.reason);
        this.ws = null;
        this.sessionId = null;
        this.maxPayloadSize = null;
        // NOTE: lastSessionId is intentionally preserved here so that the next
        // reconnect dial can append resume=<lastSessionId> in binary mode.
        // If we got a close code indicating an error, preserve error status
        if (event.code === 1011 || event.code >= 4000) {
          this.setStatus("error", event.reason || "Connection closed unexpectedly");
        } else {
          this.setStatus("disconnected");
        }
        this.scheduleReconnect();
      };
    } catch (err) {
      this.setStatus("error", err instanceof Error ? err.message : "Failed to connect");
    }
  }

  disconnect(): void {
    this.intentionalDisconnect = true;
    this.clearReconnectTimer();
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }
    this.sessionId = null;
    this.lastSessionId = null;
    this.maxPayloadSize = null;
    this.setStatus("disconnected");
    this.messageHandlers.length = 0;
    this.statusHandlers.length = 0;
    this.connectedHandlers.length = 0;
  }

  send(content: string, options?: { sessionId?: string; parts?: ContentPart[] }): void {
    if (this.ws?.readyState !== WebSocket.OPEN) {
      console.warn("Cannot send message: not connected");
      return;
    }

    const message: ClientMessage = {
      type: "message",
      session_id: options?.sessionId || this.sessionId || undefined,
      content,
      parts: options?.parts,
    };

    this.ws.send(JSON.stringify(message));
  }

  sendToolCallAck(callId: string): void {
    if (this.ws?.readyState !== WebSocket.OPEN) {
      return;
    }

    const message = {
      type: "tool_call_ack" as const,
      session_id: this.sessionId || undefined,
      tool_call_ack: {
        call_id: callId,
      },
    };

    this.ws.send(JSON.stringify(message));
  }

  sendToolResult(callId: string, result?: unknown, error?: string): void {
    if (this.ws?.readyState !== WebSocket.OPEN) {
      console.warn("Cannot send tool result: not connected");
      return;
    }

    const message = {
      type: "tool_result" as const,
      session_id: this.sessionId || undefined,
      tool_result: {
        call_id: callId,
        result,
        error,
      },
    };

    this.ws.send(JSON.stringify(message));
  }

  sendBinary(
    payload: ArrayBuffer,
    opts: { sequence: number; isLast: boolean; sampleRate: number; channels: number },
  ): void {
    if (this.ws?.readyState !== WebSocket.OPEN || !this.sessionId) return;
    const frame = encodeOmniMediaFrame({
      sessionId: this.sessionId,
      sequence: opts.sequence,
      isLast: opts.isLast,
      mimeType: "audio/pcm",
      sampleRate: opts.sampleRate,
      channels: opts.channels,
      codec: "pcm",
      payload,
    });
    this.ws.send(frame);
  }

  onBinaryMedia(
    handler: (payload: ArrayBuffer, sequence: number, isLast: boolean, sampleRate?: number) => void,
  ): () => void {
    this.binaryHandlers.push(handler);
    return () => {
      const i = this.binaryHandlers.indexOf(handler);
      if (i !== -1) this.binaryHandlers.splice(i, 1);
    };
  }

  startAudioSession(): void {
    this.binaryMode = true;
    this.connect();
  }

  onMessage(handler: (message: ServerMessage) => void): () => void {
    this.messageHandlers.push(handler);
    return () => {
      const index = this.messageHandlers.indexOf(handler);
      if (index !== -1) this.messageHandlers.splice(index, 1);
    };
  }

  onStatusChange(handler: (status: ConnectionStatus, error?: string) => void): () => void {
    this.statusHandlers.push(handler);
    return () => {
      const index = this.statusHandlers.indexOf(handler);
      if (index !== -1) this.statusHandlers.splice(index, 1);
    };
  }

  /**
   * Register a callback invoked each time a `connected` server message arrives.
   * The callback receives the session id and whether the session was resumed.
   * Returns an unsubscribe function.
   */
  onConnected(handler: (info: ConnectedEventInfo) => void): () => void {
    this.connectedHandlers.push(handler);
    return () => {
      const index = this.connectedHandlers.indexOf(handler);
      if (index !== -1) this.connectedHandlers.splice(index, 1);
    };
  }

  /**
   * Send a hangup control message to the facade so it knows the client is
   * intentionally ending the session (not a transient blip). The facade will
   * NOT park the session on receiving this message.
   */
  sendHangup(): void {
    if (this.ws?.readyState !== WebSocket.OPEN) {
      return;
    }
    this.ws.send(
      JSON.stringify({ type: "hangup", session_id: this.sessionId ?? undefined }),
    );
  }

  getStatus(): ConnectionStatus {
    return this.status;
  }

  getSessionId(): string | null {
    return this.sessionId;
  }

  getMaxPayloadSize(): number | null {
    return this.maxPayloadSize;
  }

  private handleBinary(buf: ArrayBuffer): void {
    const f = decodeOmniFrame(buf);
    if (f.messageType !== OMNI_MEDIA_CHUNK) return;
    const rate = typeof f.metadata.sample_rate === "number" ? f.metadata.sample_rate : undefined;
    for (const h of this.binaryHandlers) h(f.payload, f.sequence, f.isLast, rate);
  }

  private setStatus(status: ConnectionStatus, error?: string): void {
    this.status = status;
    this.statusHandlers.forEach((h) => h(status, error));
  }

  private emitMessage(message: ServerMessage): void {
    this.messageHandlers.forEach((h) => h(message));
  }

  private emitConnected(info: ConnectedEventInfo): void {
    this.connectedHandlers.forEach((h) => h(info));
  }

  private scheduleReconnect(): void {
    if (this.intentionalDisconnect) return;

    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null;
      this.connect();
    }, this.reconnectDelay);

    // Exponential backoff: double delay up to max
    this.reconnectDelay = Math.min(this.reconnectDelay * 2, RECONNECT_MAX_DELAY_MS);
  }

  private clearReconnectTimer(): void {
    if (this.reconnectTimer !== null) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
  }
}
