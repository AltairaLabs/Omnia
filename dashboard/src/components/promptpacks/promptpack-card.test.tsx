import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { PromptPackCard } from "./promptpack-card";
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
});
