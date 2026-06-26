import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, act } from "@testing-library/react";
import { SessionWatcher } from "./session-watcher";
import type { RuntimeConfig } from "@/lib/config";

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

const mockUseAuth = vi.fn();
vi.mock("@/hooks/use-auth", () => ({
  useAuth: () => mockUseAuth(),
}));

const mockUseRuntimeConfig = vi.fn();
vi.mock("@/hooks/use-runtime-config", () => ({
  useRuntimeConfig: () => mockUseRuntimeConfig(),
}));

const mockRedirectToLogin = vi.fn();
vi.mock("@/lib/auth/redirect-to-login", () => ({
  redirectToLogin: () => mockRedirectToLogin(),
}));

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeAuthValue(isAuthenticated: boolean) {
  return {
    isAuthenticated,
    user: {
      id: isAuthenticated ? "u1" : "anonymous",
      username: isAuthenticated ? "alice" : "anonymous",
      groups: [],
      role: "viewer" as const,
      provider: isAuthenticated ? ("oauth" as const) : ("anonymous" as const),
    },
    hasMemoryIdentity: false,
    memoryUserId: undefined,
    role: "viewer" as const,
    hasRole: () => false,
    canWrite: false,
    canAdmin: false,
    logout: vi.fn(),
  };
}

function makeConfig(
  overrides: Partial<RuntimeConfig> = {},
): { config: RuntimeConfig; loading: boolean } {
  return {
    config: {
      devMode: false,
      demoMode: false,
      readOnlyMode: false,
      readOnlyMessage: "",
      wsProxyUrl: "",
      grafanaUrl: "",
      lokiEnabled: false,
      tempoEnabled: false,
      enterpriseEnabled: false,
      hideEnterprise: false,
      authMode: "oauth",
      sessionPollIntervalSeconds: 30,
      ...overrides,
    },
    loading: false,
  };
}

// ---------------------------------------------------------------------------

beforeEach(() => {
  vi.useFakeTimers();
  vi.clearAllMocks();
});

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
  // Reset document.hidden to default
  Object.defineProperty(document, "hidden", { value: false, configurable: true });
});

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("SessionWatcher", () => {
  describe("when not authenticated", () => {
    it("does not poll /api/auth/session", async () => {
      mockUseAuth.mockReturnValue(makeAuthValue(false));
      mockUseRuntimeConfig.mockReturnValue(makeConfig({ authMode: "oauth" }));

      const fetchSpy = vi.spyOn(globalThis, "fetch").mockResolvedValue(
        new Response(JSON.stringify({ authenticated: false }), { status: 401 }),
      );

      render(<SessionWatcher />);
      await act(async () => {
        vi.advanceTimersByTime(120_000);
      });

      expect(fetchSpy).not.toHaveBeenCalled();
    });
  });

  describe("when auth mode is anonymous", () => {
    it("does not poll even when isAuthenticated is true", async () => {
      mockUseAuth.mockReturnValue(makeAuthValue(true));
      mockUseRuntimeConfig.mockReturnValue(makeConfig({ authMode: "anonymous" }));

      const fetchSpy = vi.spyOn(globalThis, "fetch").mockResolvedValue(
        new Response(JSON.stringify({ authenticated: true }), { status: 200 }),
      );

      render(<SessionWatcher />);
      await act(async () => {
        vi.advanceTimersByTime(120_000);
      });

      expect(fetchSpy).not.toHaveBeenCalled();
    });
  });

  describe("when authenticated in a non-anonymous mode", () => {
    it("polls /api/auth/session on the configured interval", async () => {
      mockUseAuth.mockReturnValue(makeAuthValue(true));
      mockUseRuntimeConfig.mockReturnValue(
        makeConfig({ authMode: "oauth", sessionPollIntervalSeconds: 30 }),
      );

      const fetchSpy = vi.spyOn(globalThis, "fetch").mockResolvedValue(
        new Response(JSON.stringify({ authenticated: true }), { status: 200 }),
      );

      render(<SessionWatcher />);

      // No immediate poll
      expect(fetchSpy).not.toHaveBeenCalled();

      await act(async () => {
        vi.advanceTimersByTime(30_000);
      });
      expect(fetchSpy).toHaveBeenCalledTimes(1);
      expect(fetchSpy).toHaveBeenCalledWith("/api/auth/session");

      await act(async () => {
        vi.advanceTimersByTime(30_000);
      });
      expect(fetchSpy).toHaveBeenCalledTimes(2);
    });

    it("redirects to login when session check returns 401", async () => {
      mockUseAuth.mockReturnValue(makeAuthValue(true));
      mockUseRuntimeConfig.mockReturnValue(
        makeConfig({ authMode: "oauth", sessionPollIntervalSeconds: 15 }),
      );

      vi.spyOn(globalThis, "fetch").mockResolvedValue(
        new Response(JSON.stringify({ authenticated: false }), { status: 401 }),
      );

      render(<SessionWatcher />);
      await act(async () => {
        vi.advanceTimersByTime(15_000);
      });

      expect(mockRedirectToLogin).toHaveBeenCalledTimes(1);
    });

    it("does not redirect when session check returns 200", async () => {
      mockUseAuth.mockReturnValue(makeAuthValue(true));
      mockUseRuntimeConfig.mockReturnValue(
        makeConfig({ authMode: "builtin", sessionPollIntervalSeconds: 15 }),
      );

      vi.spyOn(globalThis, "fetch").mockResolvedValue(
        new Response(JSON.stringify({ authenticated: true }), { status: 200 }),
      );

      render(<SessionWatcher />);
      await act(async () => {
        vi.advanceTimersByTime(15_000);
      });

      expect(mockRedirectToLogin).not.toHaveBeenCalled();
    });

    it("does not redirect on network error (fail-open)", async () => {
      mockUseAuth.mockReturnValue(makeAuthValue(true));
      mockUseRuntimeConfig.mockReturnValue(
        makeConfig({ authMode: "oauth", sessionPollIntervalSeconds: 15 }),
      );

      vi.spyOn(globalThis, "fetch").mockRejectedValue(new Error("network error"));

      render(<SessionWatcher />);
      await act(async () => {
        vi.advanceTimersByTime(15_000);
      });

      expect(mockRedirectToLogin).not.toHaveBeenCalled();
    });

    it("enforces a minimum poll interval of 15 s", async () => {
      mockUseAuth.mockReturnValue(makeAuthValue(true));
      // Request an interval below the minimum
      mockUseRuntimeConfig.mockReturnValue(
        makeConfig({ authMode: "oauth", sessionPollIntervalSeconds: 5 }),
      );

      const fetchSpy = vi.spyOn(globalThis, "fetch").mockResolvedValue(
        new Response(JSON.stringify({ authenticated: true }), { status: 200 }),
      );

      render(<SessionWatcher />);

      await act(async () => {
        vi.advanceTimersByTime(10_000); // below 15 s minimum
      });
      expect(fetchSpy).not.toHaveBeenCalled();

      await act(async () => {
        vi.advanceTimersByTime(5_000); // now at 15 s total
      });
      expect(fetchSpy).toHaveBeenCalledTimes(1);
    });

    it("polls on window focus", async () => {
      mockUseAuth.mockReturnValue(makeAuthValue(true));
      mockUseRuntimeConfig.mockReturnValue(
        makeConfig({ authMode: "oauth", sessionPollIntervalSeconds: 60 }),
      );

      vi.spyOn(globalThis, "fetch").mockResolvedValue(
        new Response(JSON.stringify({ authenticated: false }), { status: 401 }),
      );

      render(<SessionWatcher />);

      await act(async () => {
        window.dispatchEvent(new Event("focus"));
      });

      expect(mockRedirectToLogin).toHaveBeenCalledTimes(1);
    });

    it("polls on visibility change back to visible", async () => {
      mockUseAuth.mockReturnValue(makeAuthValue(true));
      mockUseRuntimeConfig.mockReturnValue(
        makeConfig({ authMode: "oauth", sessionPollIntervalSeconds: 60 }),
      );

      vi.spyOn(globalThis, "fetch").mockResolvedValue(
        new Response(JSON.stringify({ authenticated: false }), { status: 401 }),
      );

      render(<SessionWatcher />);

      Object.defineProperty(document, "hidden", { value: false, configurable: true });
      await act(async () => {
        document.dispatchEvent(new Event("visibilitychange"));
      });

      expect(mockRedirectToLogin).toHaveBeenCalledTimes(1);
    });

    it("does not poll on visibility change when document remains hidden", async () => {
      mockUseAuth.mockReturnValue(makeAuthValue(true));
      mockUseRuntimeConfig.mockReturnValue(
        makeConfig({ authMode: "oauth", sessionPollIntervalSeconds: 60 }),
      );

      vi.spyOn(globalThis, "fetch").mockResolvedValue(
        new Response(JSON.stringify({ authenticated: false }), { status: 401 }),
      );

      render(<SessionWatcher />);

      Object.defineProperty(document, "hidden", { value: true, configurable: true });
      await act(async () => {
        document.dispatchEvent(new Event("visibilitychange"));
      });

      expect(mockRedirectToLogin).not.toHaveBeenCalled();
    });

    it("cleans up interval and event listeners on unmount", async () => {
      mockUseAuth.mockReturnValue(makeAuthValue(true));
      mockUseRuntimeConfig.mockReturnValue(
        makeConfig({ authMode: "oauth", sessionPollIntervalSeconds: 15 }),
      );

      const fetchSpy = vi.spyOn(globalThis, "fetch").mockResolvedValue(
        new Response(JSON.stringify({ authenticated: true }), { status: 200 }),
      );

      const { unmount } = render(<SessionWatcher />);
      unmount();

      await act(async () => {
        vi.advanceTimersByTime(30_000);
      });

      expect(fetchSpy).not.toHaveBeenCalled();
    });
  });
});
