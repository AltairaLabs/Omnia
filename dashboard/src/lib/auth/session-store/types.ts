/**
 * Backend-agnostic OAuth session + PKCE store.
 *
 * The OAuth session payload (user, tokens, metadata) and the in-flight
 * PKCE data are kept out of the sealed iron-session cookie to avoid the
 * ~4KB browser limit that any IDP with non-trivial group claims will
 * trigger (Cognito, Okta, Auth0, Keycloak, Entra with AD groups).
 *
 * Implementations live alongside: `memory-store.ts` (dev/test) and
 * `redis-store.ts` (production).
 */

import type { OAuthTokens, PKCEData } from "../oauth/types";
import type { User } from "../types";
import type { UserRole } from "../config";
import type { WorkspaceRole } from "@/types/workspace";

/** Server-stored session record. The cookie only carries { sid }. */
export interface SessionRecord {
  user: User;
  oauth?: OAuthTokens;
  /** Wall-clock ms when the session was minted; diagnostic only — TTL is driven by ttlSeconds on putSession. */
  createdAt: number;
}

/** In-flight PKCE record. Stored under the IdP state as the key. */
export interface PkceRecord extends PKCEData {
  /** Wall-clock ms at which the record was created; used for diagnostics only. */
  createdAt: number;
}

/** In-flight CLI browser-login flow record. Stored under a server-issued flowId. */
export interface CliFlowRecord {
  /** Validated loopback callback URL (toString of parsed URL). */
  callback: string;
  /** CLI nonce echoed back on the redirect. */
  cliState: string;
  /** Wall-clock ms at which the record was created. */
  createdAt: number;
}

/** One-time CLI exchange code record. Stored under a single-use code. */
export interface CliCodeRecord {
  userId: string;
  /** Owner email or username snapshot. */
  email: string;
  groups: string[];
  /** Global role → minted key role. */
  userRole: UserRole;
  workspace: string;
  /** Workspace role → validateWorkspace clientOptions. */
  workspaceRole: WorkspaceRole;
  /** Wall-clock ms at which the record was created. */
  createdAt: number;
}

/**
 * Backend-agnostic session + PKCE store.
 *
 * All methods return plain data (no exceptions for "not found"). `null`
 * means "key does not exist" or "record has expired". Implementations
 * must enforce TTLs and must ensure `consumePkce` is atomic single-use
 * (e.g. Redis `GETDEL`).
 */
export interface SessionStore {
  /** Read a session by id. Returns null if missing or expired. */
  getSession(sid: string): Promise<SessionRecord | null>;

  /** Create or overwrite a session. `ttlSeconds` must be > 0. */
  putSession(sid: string, record: SessionRecord, ttlSeconds: number): Promise<void>;

  /** Delete a session. No-op if it does not exist. */
  deleteSession(sid: string): Promise<void>;

  /** Write an in-flight PKCE record keyed by its IdP state. The `state` argument must equal `record.state`. */
  putPkce(state: string, record: PkceRecord, ttlSeconds: number): Promise<void>;

  /** Atomic single-use read of a PKCE record. Returns null if missing, expired, or already consumed. */
  consumePkce(state: string): Promise<PkceRecord | null>;

  /** Write a CLI browser-login flow record keyed by a server-issued flowId. */
  putCliFlow(flowId: string, record: CliFlowRecord, ttlSeconds: number): Promise<void>;
  /** Non-consuming read of a CLI flow record (picker page reload-safe). */
  getCliFlow(flowId: string): Promise<CliFlowRecord | null>;
  /** Atomic single-use read of a CLI flow record. */
  consumeCliFlow(flowId: string): Promise<CliFlowRecord | null>;
  /** Write a one-time CLI exchange code. */
  putCliCode(code: string, record: CliCodeRecord, ttlSeconds: number): Promise<void>;
  /** Atomic single-use read of a one-time CLI exchange code. */
  consumeCliCode(code: string): Promise<CliCodeRecord | null>;
}
