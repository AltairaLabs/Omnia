/**
 * Tests for the Providers list page — role column + filter chips (Phase 3).
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, within } from "@testing-library/react";
import ProvidersPage from "./page";
import type { Provider } from "@/types/generated/provider";

// Hooks
vi.mock("@/hooks/resources", () => ({
  useProviders: vi.fn(),
  useSharedProviders: vi.fn(),
  useProviderMetrics: vi.fn(() => ({ data: undefined })),
  useProviderMutations: vi.fn(() => ({
    createProvider: vi.fn(),
    updateProvider: vi.fn(),
    loading: false,
    error: null,
  })),
}));

// Workspace context
vi.mock("@/contexts/workspace-context", () => ({
  useWorkspace: () => ({
    currentWorkspace: { name: "ws", namespace: "default", permissions: { write: true } },
  }),
}));

// Layout — simplify to keep the test focused on filter behaviour.
vi.mock("@/components/layout", () => ({
  Header: function MockHeader({ title }: { title: string }) {
    return <h1>{title}</h1>;
  },
}));

// NamespaceFilter — render a passthrough so we can ignore it in this test.
vi.mock("@/components/filters", () => ({
  NamespaceFilter: () => <div data-testid="ns-filter" />,
}));

// CostSparkline depends on recharts; stub it.
vi.mock("@/components/cost", () => ({
  CostSparkline: () => <div data-testid="sparkline" />,
}));

// next/link
vi.mock("next/link", () => ({
  default: function MockLink({ children }: { children: React.ReactNode }) {
    return <>{children}</>;
  },
}));

function makeProvider(name: string, role: Provider["spec"]["role"] | undefined, namespace = "default"): Provider {
  return {
    apiVersion: "omnia.altairalabs.ai/v1alpha1",
    kind: "Provider",
    metadata: {
      name,
      namespace,
      uid: `uid-${name}`,
      creationTimestamp: "2026-05-01T00:00:00Z",
    },
    spec: {
      type: role === "embedding" ? "voyageai" : "openai",
      ...(role === undefined ? {} : { role }),
    },
    status: { phase: "Ready" },
  };
}

const PROVIDERS: Provider[] = [
  makeProvider("inf-1", "llm"),
  makeProvider("inf-2", "llm"),
  makeProvider("embed-1", "embedding"),
  makeProvider("tts-1", "tts"),
  makeProvider("legacy-no-role", undefined), // pre-role: should count as inference
];

describe("ProvidersPage role filter chips", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  async function renderPage() {
    const { useProviders, useSharedProviders } = await import("@/hooks/resources");
    vi.mocked(useProviders).mockReturnValue({
      data: PROVIDERS,
      isLoading: false,
      refetch: vi.fn(),
    } as never);
    vi.mocked(useSharedProviders).mockReturnValue({
      data: [],
      isLoading: false,
    } as never);
    render(<ProvidersPage />);
  }

  it("renders one chip per role with correct counts (pre-role counts as llm)", async () => {
    await renderPage();

    const group = screen.getByRole("group", { name: /filter by role/i });
    // All providers across roles.
    expect(within(group).getByRole("button", { name: /all roles \(5\)/i })).toBeInTheDocument();
    // Two explicit llm + one legacy (no spec.role) = 3.
    expect(within(group).getByRole("button", { name: /llm \(3\)/i })).toBeInTheDocument();
    expect(within(group).getByRole("button", { name: /embedding \(1\)/i })).toBeInTheDocument();
    expect(within(group).getByRole("button", { name: /tts \(1\)/i })).toBeInTheDocument();
    expect(within(group).getByRole("button", { name: /stt \(0\)/i })).toBeInTheDocument();
    expect(within(group).getByRole("button", { name: /image \(0\)/i })).toBeInTheDocument();
  });

  it("filters cards down to the picked role", async () => {
    await renderPage();

    // Initially all 5 names visible.
    expect(screen.getByText("inf-1")).toBeInTheDocument();
    expect(screen.getByText("embed-1")).toBeInTheDocument();
    expect(screen.getByText("tts-1")).toBeInTheDocument();
    expect(screen.getByText("legacy-no-role")).toBeInTheDocument();

    const group = screen.getByRole("group", { name: /filter by role/i });
    fireEvent.click(within(group).getByRole("button", { name: /embedding/i }));

    // Only the embedding provider remains.
    expect(screen.getByText("embed-1")).toBeInTheDocument();
    expect(screen.queryByText("inf-1")).toBeNull();
    expect(screen.queryByText("tts-1")).toBeNull();
    expect(screen.queryByText("legacy-no-role")).toBeNull();
  });

  it("shows a 'Role' column with effective role value in cards", async () => {
    await renderPage();

    // Each card renders a "Role" label and the effective role beneath it.
    expect(screen.getAllByText("Role").length).toBe(PROVIDERS.length);
    // Filter to just the legacy (pre-role) provider and verify its card shows
    // "llm" (the effective default).
    const group = screen.getByRole("group", { name: /filter by role/i });
    fireEvent.click(within(group).getByRole("button", { name: /llm/i }));

    // 3 llm cards remain (inf-1, inf-2, legacy-no-role).
    expect(screen.getByText("legacy-no-role")).toBeInTheDocument();
    // The card body labels each role; one card per llm provider.
    expect(screen.getAllByText(/llm/i).length).toBeGreaterThanOrEqual(3);
  });
});
