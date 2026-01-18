/**
 * Tests for providers component.
 */

import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
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
    render(
      <Providers>
        <span>Content</span>
      </Providers>
    );

    expect(screen.getByTestId("theme-provider")).toBeInTheDocument();
  });

  it("includes WorkspaceProvider", () => {
    render(
      <Providers>
        <span>Content</span>
      </Providers>
    );

    expect(screen.getByTestId("workspace-provider")).toBeInTheDocument();
  });

  it("includes DataServiceProvider", () => {
    render(
      <Providers>
        <span>Content</span>
      </Providers>
    );

    expect(screen.getByTestId("data-service-provider")).toBeInTheDocument();
  });

  it("nests providers in correct order", () => {
    render(
      <Providers>
        <span data-testid="content">Content</span>
      </Providers>
    );

    // Check that providers are nested correctly
    const themeProvider = screen.getByTestId("theme-provider");
    const workspaceProvider = screen.getByTestId("workspace-provider");
    const dataServiceProvider = screen.getByTestId("data-service-provider");
    const content = screen.getByTestId("content");

    // ThemeProvider should contain WorkspaceProvider
    expect(themeProvider).toContainElement(workspaceProvider);
    // WorkspaceProvider should contain DataServiceProvider
    expect(workspaceProvider).toContainElement(dataServiceProvider);
    // DataServiceProvider should contain the content
    expect(dataServiceProvider).toContainElement(content);
  });
});
