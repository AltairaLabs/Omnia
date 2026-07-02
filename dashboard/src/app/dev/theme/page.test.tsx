/**
 * Smoke + gating tests for the /dev/theme kitchen-sink page. The full visual
 * surface is verified by Playwright; here we assert it 404s outside dev/demo
 * and renders the themed primitives in dev.
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import ThemePreviewPage from "./page";

const mockNotFound = vi.fn();
vi.mock("next/navigation", () => ({ notFound: () => mockNotFound() }));

const mockDev = vi.fn();
const mockDemo = vi.fn();
vi.mock("@/hooks/core", () => ({
  useDevMode: () => mockDev(),
  useDemoMode: () => mockDemo(),
}));

vi.mock("@/lib/flow/use-color-mode", () => ({ useFlowColorMode: () => "light" }));

vi.mock("@xyflow/react", () => ({
  ReactFlow: ({ children }: { children?: React.ReactNode }) => (
    <div data-testid="rf">{children}</div>
  ),
  Background: () => <div />,
}));

describe("ThemePreviewPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockDev.mockReturnValue({ isDevMode: true, loading: false });
    mockDemo.mockReturnValue({ isDemoMode: false, loading: false });
  });

  it("404s when not in dev or demo mode", () => {
    mockDev.mockReturnValue({ isDevMode: false, loading: false });
    mockDemo.mockReturnValue({ isDemoMode: false, loading: false });
    render(<ThemePreviewPage />);
    expect(mockNotFound).toHaveBeenCalled();
  });

  it("renders nothing while runtime config is loading", () => {
    mockDev.mockReturnValue({ isDevMode: false, loading: true });
    mockDemo.mockReturnValue({ isDemoMode: false, loading: true });
    const { container } = render(<ThemePreviewPage />);
    expect(container).toBeEmptyDOMElement();
    expect(mockNotFound).not.toHaveBeenCalled();
  });

  it("renders the themed primitives in dev mode", () => {
    render(<ThemePreviewPage />);
    expect(screen.getByTestId("theme-preview")).toBeInTheDocument();
    expect(screen.getByText("Status badges")).toBeInTheDocument();
    expect(screen.getByText("category-1")).toBeInTheDocument();
    expect(screen.getByText("chart-5")).toBeInTheDocument();
    expect(screen.getByTestId("theme-preview-flow")).toBeInTheDocument();
    expect(mockNotFound).not.toHaveBeenCalled();
  });
});
