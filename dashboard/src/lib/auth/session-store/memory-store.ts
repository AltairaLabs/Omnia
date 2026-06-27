/**
 * In-memory `SessionStore` for development and tests.
 *
 * Records expire lazily on read — a background sweep is unnecessary for
 * the scale of a single dev process. The production implementation is
 * `RedisSessionStore`; do not use this for multi-replica deployments.
 */

import type { CliCodeRecord, CliFlowRecord, PkceRecord, SessionRecord, SessionStore } from "./types";

interface Entry<T> {
  value: T;
  expiresAt: number; // ms epoch
}

function requirePositiveTtl(ttlSeconds: number): void {
  if (!Number.isFinite(ttlSeconds) || ttlSeconds <= 0) {
    throw new Error(`ttlSeconds must be > 0, got ${ttlSeconds}`);
  }
}

export class MemorySessionStore implements SessionStore {
  private readonly sessions = new Map<string, Entry<SessionRecord>>();
  private readonly pkce = new Map<string, Entry<PkceRecord>>();
  private readonly cliFlows = new Map<string, Entry<CliFlowRecord>>();
  private readonly cliCodes = new Map<string, Entry<CliCodeRecord>>();

  async getSession(sid: string): Promise<SessionRecord | null> {
    return this.readAndExpire(this.sessions, sid);
  }

  async putSession(sid: string, record: SessionRecord, ttlSeconds: number): Promise<void> {
    requirePositiveTtl(ttlSeconds);
    this.sessions.set(sid, {
      value: record,
      expiresAt: Date.now() + ttlSeconds * 1000,
    });
  }

  async deleteSession(sid: string): Promise<void> {
    this.sessions.delete(sid);
  }

  async putPkce(state: string, record: PkceRecord, ttlSeconds: number): Promise<void> {
    requirePositiveTtl(ttlSeconds);
    this.pkce.set(state, {
      value: record,
      expiresAt: Date.now() + ttlSeconds * 1000,
    });
  }

  async consumePkce(state: string): Promise<PkceRecord | null> {
    const entry = this.pkce.get(state);
    if (!entry) return null;
    // Single-use: always delete on consume, even if expired.
    this.pkce.delete(state);
    if (entry.expiresAt <= Date.now()) return null;
    return entry.value;
  }

  async putCliFlow(flowId: string, record: CliFlowRecord, ttlSeconds: number): Promise<void> {
    requirePositiveTtl(ttlSeconds);
    this.cliFlows.set(flowId, { value: record, expiresAt: Date.now() + ttlSeconds * 1000 });
  }

  async getCliFlow(flowId: string): Promise<CliFlowRecord | null> {
    return this.readAndExpire(this.cliFlows, flowId);
  }

  async consumeCliFlow(flowId: string): Promise<CliFlowRecord | null> {
    const entry = this.cliFlows.get(flowId);
    if (!entry) return null;
    this.cliFlows.delete(flowId);
    if (entry.expiresAt <= Date.now()) return null;
    return entry.value;
  }

  async putCliCode(code: string, record: CliCodeRecord, ttlSeconds: number): Promise<void> {
    requirePositiveTtl(ttlSeconds);
    this.cliCodes.set(code, { value: record, expiresAt: Date.now() + ttlSeconds * 1000 });
  }

  async consumeCliCode(code: string): Promise<CliCodeRecord | null> {
    const entry = this.cliCodes.get(code);
    if (!entry) return null;
    this.cliCodes.delete(code);
    if (entry.expiresAt <= Date.now()) return null;
    return entry.value;
  }

  private readAndExpire<T>(map: Map<string, Entry<T>>, key: string): T | null {
    const entry = map.get(key);
    if (!entry) return null;
    if (entry.expiresAt <= Date.now()) {
      map.delete(key);
      return null;
    }
    return entry.value;
  }
}
