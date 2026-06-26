/**
 * Tests for providers component.
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, act } from "@testing-library/react";
import { useQueryClient } from "@tanstack/react-query";
import { _resetRedirectGuard } from "@/lib/auth/redirect-to-login";
import { Providers } from "./providers";

// Mock the dependencies
vi.mock("@/lib/data", () => ({
  DataServiceProvider: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="data-service-provider">{children}</div>
  ),
}));

vi.mock("@/contexts/workspace-context", () => ({
  WorkspaceProvider: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="workspace-provider">{children}</div>
  ),
}));

vi.mock("next-themes", () => ({
  ThemeProvider: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="theme-provider">{children}</div>
  ),
}));

const mockAssign = vi.fn();

beforeEach(() => {
  vi.resetAllMocks();
  // Reset the in-flight redirect guard so each test starts clean.
  _resetRedirectGuard();
  Object.defineProperty(window, "location", {
    writable: true,
    value: { assign: mockAssign, pathname: "/dashboard", search: "" },
  });
});

// Helper: triggers the QueryCache onError handler directly.
function CacheErrorTrigger({
  error,
  testId,
}: {
  error: unknown;
  testId: string;
}) {
  const qc = useQueryClient();
  return (
    <button
      data-testid={testId}
      onClick={() => {
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        (qc.getQueryCache() as any).config?.onError?.(error, {} as any);
      }}
    >
      trigger
    </button>
  );
}

// Helper: triggers the MutationCache onError handler directly.
function MutationErrorTrigger({
  error,
  testId,
}: {
  error: unknown;
  testId: string;
}) {
  const qc = useQueryClient();
  return (
    <button
      data-testid={testId}
      onClick={() => {
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        (qc.getMutationCache() as any).config?.onError?.(error, {} as any, {} as any, {} as any);
      }}
    >
      trigger
    </button>
  );
}

describe("Providers", () => {
  it("renders children wrapped in all providers", () => {
    render(
      <Providers>
        <div data-testid="child">Child content</div>
      </Providers>
    );
    expect(screen.getByTestId("child")).toBeInTheDocument();
    expect(screen.getByText("Child content")).toBeInTheDocument();
  });

  it("includes ThemeProvider", () => {
    render(<Providers><span>Content</span></Providers>);
    expect(screen.getByTestId("theme-provider")).toBeInTheDocument();
  });

  it("includes WorkspaceProvider", () => {
    render(<Providers><span>Content</span></Providers>);
    expect(screen.getByTestId("workspace-provider")).toBeInTheDocument();
  });

  it("includes DataServiceProvider", () => {
    render(<Providers><span>Content</span></Providers>);
    expect(screen.getByTestId("data-service-provider")).toBeInTheDocument();
  });

  it("nests providers in correct order", () => {
    render(<Providers><span data-testid="content">Content</span></Providers>);
    expect(screen.getByTestId("theme-provider")).toContainElement(
      screen.getByTestId("workspace-provider"),
    );
    expect(screen.getByTestId("workspace-provider")).toContainElement(
      screen.getByTestId("data-service-provider"),
    );
    expect(screen.getByTestId("data-service-provider")).toContainElement(
      screen.getByTestId("content"),
    );
  });

  describe("global 401 handler — QueryCache", () => {
    it("redirects to login when QueryCache emits an error with status 401", async () => {
      const err401 = Object.assign(new Error("Unauthorized"), { status: 401 });
      render(
        <Providers>
          <CacheErrorTrigger error={err401} testId="qc-401" />
        </Providers>
      );
      await act(async () => {
        screen.getByTestId("qc-401").click();
      });
      expect(mockAssign).toHaveBeenCalledWith(
        expect.stringContaining("/login?returnTo="),
      );
    });

    it("does not redirect when QueryCache emits a non-401 status error", async () => {
      const err500 = Object.assign(new Error("Server Error"), { status: 500 });
      render(
        <Providers>
          <CacheErrorTrigger error={err500} testId="qc-500" />
        </Providers>
      );
      await act(async () => {
        screen.getByTestId("qc-500").click();
      });
      expect(mockAssign).not.toHaveBeenCalled();
    });

    it("does not redirect when QueryCache emits a non-Error value", async () => {
      render(
        <Providers>
          <CacheErrorTrigger error="string error" testId="qc-str" />
        </Providers>
      );
      await act(async () => {
        screen.getByTestId("qc-str").click();
      });
      expect(mockAssign).not.toHaveBeenCalled();
    });
  });

  describe("global 401 handler — MutationCache", () => {
    it("redirects to login when MutationCache emits an error with status 401", async () => {
      const err401 = Object.assign(new Error("Unauthorized"), { status: 401 });
      render(
        <Providers>
          <MutationErrorTrigger error={err401} testId="mc-401" />
        </Providers>
      );
      await act(async () => {
        screen.getByTestId("mc-401").click();
      });
      expect(mockAssign).toHaveBeenCalledWith(
        expect.stringContaining("/login?returnTo="),
      );
    });

    it("does not redirect when MutationCache emits a non-401 status error", async () => {
      const err403 = Object.assign(new Error("Forbidden"), { status: 403 });
      render(
        <Providers>
          <MutationErrorTrigger error={err403} testId="mc-403" />
        </Providers>
      );
      await act(async () => {
        screen.getByTestId("mc-403").click();
      });
      expect(mockAssign).not.toHaveBeenCalled();
    });
  });
});
