import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { PromptPackCard, tierSummary } from "./promptpack-card";
import type { PromptPack } from "@/types";

// Mock next/link
vi.mock("next/link", () => ({
  default: ({
    children,
    href,
  }: {
    children: React.ReactNode;
    href: string;
  }) => <a href={href}>{children}</a>,
}));

// Stub the workload tier hook (the real one needs a QueryClient + data service).
vi.mock("@/hooks/use-workload-tier", () => ({
  useWorkloadTier: () => ({ tier: "multiagent", agents: 3, tools: 8, states: 2, isLoading: false }),
}));

const mockPromptPack: PromptPack = {
  apiVersion: "omnia.altairalabs.ai/v1alpha1",
  kind: "PromptPack",
  metadata: {
    name: "test-pack",
    namespace: "production",
    uid: "pack-001",
  },
  spec: {
    source: { type: "configmap", configMapRef: { name: "test-pack-configmap" } },
    version: "1.0.0",
  },
  status: {
    phase: "Active",
    activeVersion: "1.0.0",
    lastUpdated: new Date(Date.now() - 2 * 60 * 60 * 1000).toISOString(), // 2h ago
  },
};

describe("PromptPackCard", () => {
  it("renders the pack name", () => {
    render(<PromptPackCard promptPack={mockPromptPack} />);
    expect(screen.getByText("test-pack")).toBeInTheDocument();
  });

  it("renders the namespace", () => {
    render(<PromptPackCard promptPack={mockPromptPack} />);
    expect(screen.getByText("production")).toBeInTheDocument();
  });

  it("renders the active version from status", () => {
    render(<PromptPackCard promptPack={mockPromptPack} />);
    expect(screen.getByText("v1.0.0")).toBeInTheDocument();
  });

  it("falls back to spec version when status has no activeVersion", () => {
    const pack: PromptPack = {
      ...mockPromptPack,
      status: { phase: "Pending" },
    };
    render(<PromptPackCard promptPack={pack} />);
    expect(screen.getByText("v1.0.0")).toBeInTheDocument();
  });

  it("renders the configmap source name", () => {
    render(<PromptPackCard promptPack={mockPromptPack} />);
    expect(screen.getByText("test-pack-configmap")).toBeInTheDocument();
  });

  it('shows "unknown" when configMapRef is missing', () => {
    const pack: PromptPack = {
      ...mockPromptPack,
      spec: {
        source: { type: "configmap" },
        version: "1.0.0",
      },
    };
    render(<PromptPackCard promptPack={pack} />);
    expect(screen.getByText("unknown")).toBeInTheDocument();
  });

  it("links to the promptpack detail page", () => {
    render(<PromptPackCard promptPack={mockPromptPack} />);
    const link = screen.getByRole("link");
    expect(link).toHaveAttribute(
      "href",
      "/promptpacks/test-pack?namespace=production"
    );
  });

  it("shows relative time when lastUpdated is set", () => {
    render(<PromptPackCard promptPack={mockPromptPack} />);
    expect(screen.getByText("2h ago")).toBeInTheDocument();
  });

  it('shows "-" when lastUpdated is not set', () => {
    const pack: PromptPack = {
      ...mockPromptPack,
      status: { phase: "Active", activeVersion: "1.0.0" },
    };
    render(<PromptPackCard promptPack={pack} />);
    expect(screen.getByText("-")).toBeInTheDocument();
  });

  it("renders the promptpack-card test id", () => {
    render(<PromptPackCard promptPack={mockPromptPack} />);
    expect(screen.getByTestId("promptpack-card")).toBeInTheDocument();
  });

  it("shows minutes ago for recent timestamps", () => {
    const pack: PromptPack = {
      ...mockPromptPack,
      status: {
        ...mockPromptPack.status,
        lastUpdated: new Date(Date.now() - 30 * 60 * 1000).toISOString(), // 30m ago
      },
    };
    render(<PromptPackCard promptPack={pack} />);
    expect(screen.getByText("30m ago")).toBeInTheDocument();
  });

  it("shows days ago for old timestamps", () => {
    const pack: PromptPack = {
      ...mockPromptPack,
      status: {
        ...mockPromptPack.status,
        lastUpdated: new Date(Date.now() - 3 * 24 * 60 * 60 * 1000).toISOString(), // 3d ago
      },
    };
    render(<PromptPackCard promptPack={pack} />);
    expect(screen.getByText("3d ago")).toBeInTheDocument();
  });

  it("renders the workload tier chip with agent and tool counts", () => {
    render(<PromptPackCard promptPack={mockPromptPack} />);
    expect(screen.getByText(/Multi-agent/)).toBeInTheDocument();
    expect(screen.getByText(/3 agents/)).toBeInTheDocument();
    expect(screen.getByText(/8 tools/)).toBeInTheDocument();
  });
});

describe("tierSummary", () => {
  const base = { agents: 3, tools: 8, states: 2, isLoading: false } as const;

  it("labels and summarises a multi-agent tier", () => {
    expect(tierSummary({ ...base, tier: "multiagent" })).toEqual({
      label: "Multi-agent",
      parts: ["3 agents", "8 tools"],
    });
  });

  it("labels and summarises a workflow tier", () => {
    expect(tierSummary({ ...base, tier: "workflow" })).toEqual({
      label: "Workflow",
      parts: ["2 states", "8 tools"],
    });
  });

  it("labels and summarises a single tier (and the undefined fallback)", () => {
    expect(tierSummary({ ...base, tier: "single" })).toEqual({
      label: "Single",
      parts: ["8 tools"],
    });
    expect(tierSummary({ ...base, tier: undefined }).label).toBe("Single");
  });
});
