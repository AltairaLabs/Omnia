/**
 * Classifies error strings emitted by the agent runtime into structured
 * categories the UI can render actionably.
 *
 * Issue #1037 part 2. Pre-this, an authentication failure surfaced as a
 * raw stack trace in the chat banner — operators saw 400 lines of Go
 * runtime detail and had no signal pointing them at "your API key is
 * invalid; check the provider." Even worse, Google's Gemini API
 * returns 429 for some invalid-key scenarios, sending diagnostics down
 * a quota-exhaustion rabbit hole.
 *
 * The classifier is INTENTIONALLY conservative — when a string doesn't
 * match a known pattern it falls through to "unknown" and the existing
 * raw-text banner renders. False positives (claiming "auth failed"
 * when it's really a network blip) are worse than false negatives.
 */

/** Structured error classification. */
export interface AgentErrorInfo {
  /** The error category. "unknown" preserves existing raw-text behaviour. */
  kind: AgentErrorKind;
  /** Best-guess provider name when identifiable from the error text. */
  provider?: KnownProvider;
  /**
   * The raw error string that was classified. Always the input, returned
   * verbatim so the UI can show it in a "details" disclosure even when
   * a structured banner is shown above.
   */
  raw: string;
}

export type AgentErrorKind =
  | "invalid_credential"
  | "rate_limited"
  | "provider_unavailable"
  | "unknown";

export type KnownProvider = "gemini" | "openai" | "claude";

/**
 * Provider markers — substrings that identify which LLM provider the
 * error came from. Order matters when multiple could match (we hit
 * the first); each provider's string is distinctive enough that
 * collisions don't happen in practice.
 */
const providerMarkers: ReadonlyArray<{ marker: RegExp; provider: KnownProvider }> = [
  { marker: /generativelanguage\.googleapis\.com|gemini/i, provider: "gemini" },
  { marker: /api\.openai\.com|openai/i, provider: "openai" },
  { marker: /api\.anthropic\.com|anthropic|claude/i, provider: "claude" },
];

/**
 * Auth-class markers — patterns that identify "this is a credential
 * problem, not a transient network issue." Drawn from the actual
 * provider error strings hit during the issue #1037 audit:
 *   Gemini: "API_KEY_INVALID", "API key not valid"
 *   OpenAI: "invalid_api_key", "Incorrect API key"
 *   Anthropic: "authentication_error", "invalid x-api-key"
 *   Generic HTTP: "401" / "403" with an API URL nearby
 */
const invalidCredentialMarkers: RegExp[] = [
  /API_KEY_INVALID/,
  /api key not valid/i,
  /invalid_api_key/,
  /incorrect api key/i,
  /authentication_error/,
  /invalid x-api-key/i,
  /\bunauthorized\b/i,
  /\bforbidden\b/i,
  /status\s*[:=]\s*401\b/i,
  /status\s*[:=]\s*403\b/i,
  /\bplaceholder\b/i, // operator's PlaceholderCredential condition (#1037 part 1)
];

const rateLimitMarkers: RegExp[] = [
  /\brate.?limit/i,
  /resource_exhausted/i,
  /\btoo many requests\b/i,
  /status\s*[:=]\s*429\b/i,
  // Note: Gemini returns 429 RESOURCE_EXHAUSTED for SOME invalid-key
  // cases, so we leave a guard in classifyAgentError to demote
  // RESOURCE_EXHAUSTED to invalid_credential when an INVALID_API_KEY
  // marker also appears in the same string.
];

const providerUnavailableMarkers: RegExp[] = [
  /no such host/i,
  /connection refused/i,
  /context deadline exceeded/i,
  /timeout/i,
  /status\s*[:=]\s*5\d{2}\b/i,
];

/**
 * Classifies an error string from the agent. Returns "unknown" for
 * strings that don't match any known pattern — callers should keep
 * showing the raw message in that case.
 */
export function classifyAgentError(raw: string): AgentErrorInfo {
  if (!raw) {
    return { kind: "unknown", raw: "" };
  }

  const provider = detectProvider(raw);

  // Invalid-credential check FIRST: a string that mentions both
  // INVALID_API_KEY and 429 RESOURCE_EXHAUSTED is the Gemini "I
  // returned 429 for an invalid key" failure mode — don't classify
  // it as rate-limited.
  if (invalidCredentialMarkers.some((re) => re.test(raw))) {
    return { kind: "invalid_credential", provider, raw };
  }

  if (rateLimitMarkers.some((re) => re.test(raw))) {
    return { kind: "rate_limited", provider, raw };
  }

  if (providerUnavailableMarkers.some((re) => re.test(raw))) {
    return { kind: "provider_unavailable", provider, raw };
  }

  return { kind: "unknown", provider, raw };
}

function detectProvider(raw: string): KnownProvider | undefined {
  for (const { marker, provider } of providerMarkers) {
    if (marker.test(raw)) return provider;
  }
  return undefined;
}

/**
 * Human-readable summary line for a classified error. Used as the
 * banner headline; the raw text remains available below in details.
 */
export function summariseAgentError(info: AgentErrorInfo): string {
  const providerLabel = info.provider ? ` (${info.provider})` : "";
  switch (info.kind) {
    case "invalid_credential":
      return `Provider authentication failed${providerLabel}. The API key looks invalid or expired.`;
    case "rate_limited":
      return `Provider rate limit hit${providerLabel}. Wait a moment or check the quota.`;
    case "provider_unavailable":
      return `Provider unreachable${providerLabel}. Network error or service outage.`;
    case "unknown":
      return info.raw;
  }
}
