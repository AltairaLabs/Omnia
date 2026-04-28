import { describe, it, expect } from "vitest";
import {
  classifyAgentError,
  summariseAgentError,
} from "./classify";

describe("classifyAgentError", () => {
  describe("invalid_credential", () => {
    it("identifies Gemini API_KEY_INVALID even when wrapped in 429 RESOURCE_EXHAUSTED", () => {
      // Real string from the issue #1037 audit. Gemini sometimes
      // returns 429 RESOURCE_EXHAUSTED for invalid keys; the auth
      // marker MUST win or the user is told "wait for quota".
      const raw = `provider stream failed: failed to send request: API request to generativelanguage.googleapis.com failed with status 429: [{"error":{"code":429,"message":"Resource exhausted","status":"RESOURCE_EXHAUSTED","details":[{"reason":"API_KEY_INVALID"}]}}]`;
      const info = classifyAgentError(raw);
      expect(info.kind).toBe("invalid_credential");
      expect(info.provider).toBe("gemini");
    });

    it("identifies Gemini status 400 + API key not valid", () => {
      const raw = `API request to generativelanguage.googleapis.com failed with status 400: API key not valid. Please pass a valid API key.`;
      const info = classifyAgentError(raw);
      expect(info.kind).toBe("invalid_credential");
      expect(info.provider).toBe("gemini");
    });

    it("identifies OpenAI invalid_api_key", () => {
      const raw = `openai 401 unauthorized: {"error":{"message":"Incorrect API key provided","type":"invalid_request_error","code":"invalid_api_key"}}`;
      const info = classifyAgentError(raw);
      expect(info.kind).toBe("invalid_credential");
      expect(info.provider).toBe("openai");
    });

    it("identifies Anthropic authentication_error", () => {
      const raw = `anthropic api error: {"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`;
      const info = classifyAgentError(raw);
      expect(info.kind).toBe("invalid_credential");
      expect(info.provider).toBe("claude");
    });

    it("identifies the operator's PlaceholderCredential marker", () => {
      // From #1037 part 1 — operator surfaces this as a Provider
      // condition, but it can also reach the runtime when the
      // pre-flight check is skipped. The classifier picks it up.
      const raw = `secret gemini-credentials contains a placeholder value (matches dev-sample marker like 'replace-with-real-key'); replace with a real key`;
      const info = classifyAgentError(raw);
      expect(info.kind).toBe("invalid_credential");
    });

    it("treats bare 401/403 as invalid_credential when an API URL is in the message", () => {
      const raw = `request to https://api.openai.com/v1/chat/completions returned status: 401`;
      const info = classifyAgentError(raw);
      expect(info.kind).toBe("invalid_credential");
      expect(info.provider).toBe("openai");
    });
  });

  describe("rate_limited", () => {
    it("identifies a generic 429 rate-limit", () => {
      // Plain rate limit, no INVALID_API_KEY marker.
      const raw = `provider stream failed: status 429: rate limited, retry after 30s`;
      const info = classifyAgentError(raw);
      expect(info.kind).toBe("rate_limited");
    });

    it("identifies 'too many requests'", () => {
      const raw = `429 Too Many Requests`;
      expect(classifyAgentError(raw).kind).toBe("rate_limited");
    });
  });

  describe("provider_unavailable", () => {
    it("identifies DNS failures", () => {
      const raw = `dial tcp: lookup api.openai.com on 10.96.0.10:53: no such host`;
      const info = classifyAgentError(raw);
      expect(info.kind).toBe("provider_unavailable");
      expect(info.provider).toBe("openai");
    });

    it("identifies connection-refused", () => {
      const raw = `dial tcp 1.2.3.4:443: connection refused`;
      expect(classifyAgentError(raw).kind).toBe("provider_unavailable");
    });

    it("identifies provider 5xx", () => {
      const raw = `api.anthropic.com returned status: 503 service unavailable`;
      const info = classifyAgentError(raw);
      expect(info.kind).toBe("provider_unavailable");
      expect(info.provider).toBe("claude");
    });
  });

  describe("unknown", () => {
    it("falls through for messages without known markers", () => {
      const raw = `something completely random happened in the runtime`;
      const info = classifyAgentError(raw);
      expect(info.kind).toBe("unknown");
      expect(info.raw).toBe(raw);
    });

    it("returns unknown with empty raw for empty input", () => {
      expect(classifyAgentError("").kind).toBe("unknown");
    });
  });
});

describe("summariseAgentError", () => {
  it("includes provider label when known", () => {
    const summary = summariseAgentError({
      kind: "invalid_credential",
      provider: "gemini",
      raw: "...",
    });
    expect(summary).toContain("(gemini)");
    expect(summary).toContain("authentication failed");
  });

  it("omits provider label when unknown", () => {
    const summary = summariseAgentError({
      kind: "invalid_credential",
      raw: "...",
    });
    expect(summary).not.toContain("()");
    expect(summary).toContain("authentication failed");
  });

  it("returns raw text for kind=unknown", () => {
    const raw = "weird unparseable error";
    const summary = summariseAgentError({ kind: "unknown", raw });
    expect(summary).toBe(raw);
  });
});
