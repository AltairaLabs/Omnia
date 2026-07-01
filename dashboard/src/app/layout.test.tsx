/**
 * Tests for root layout component.
 */

import { describe, it, expect, vi } from "vitest";

// Mock next/font/google before importing layout
vi.mock("next/font/google", () => ({
  Inter: () => ({ variable: "--font-inter" }),
  JetBrains_Mono: () => ({ variable: "--font-jetbrains-mono" }),
}));

// Mock globals.css
vi.mock("./globals.css", () => ({}));

// Now import the layout to test
import RootLayout, { generateMetadata } from "./layout";

// Mock only the child components, not the layout itself
vi.mock("@/components/providers", () => ({
  Providers: ({ children }: { children: React.ReactNode }) => children,
}));

vi.mock("@/components/auth-wrapper", () => ({
  AuthWrapper: ({ children }: { children: React.ReactNode }) => children,
}));

vi.mock("@/components/layout", () => ({
  Sidebar: () => null,
  ReadOnlyBanner: () => null,
  DemoModeBanner: () => null,
  LicenseExpiryBanner: () => null,
  DevModeLicenseBanner: () => null,
  WorkspaceContent: ({ children }: { children: React.ReactNode }) => children,
  AppShell: ({ children }: { children: React.ReactNode }) => children,
}));

describe("RootLayout", () => {
  it("should be a valid React component", () => {
    expect(typeof RootLayout).toBe("function");
  });

  it("should return an html element when called", () => {
    const element = RootLayout({ children: <div>Test</div> });
    expect(element).toBeDefined();
    expect(element.type).toBe("html");
  });

  it("should set lang attribute to en", () => {
    const element = RootLayout({ children: <div>Test</div> });
    expect(element.props.lang).toBe("en");
  });

  it("should set suppressHydrationWarning", () => {
    const element = RootLayout({ children: <div>Test</div> });
    expect(element.props.suppressHydrationWarning).toBe(true);
  });

  it("should render body element", () => {
    const element = RootLayout({ children: <div>Test</div> });
    const body = element.props.children;
    expect(body.type).toBe("body");
  });

  it("should apply font-sans class to body", () => {
    const element = RootLayout({ children: <div>Test</div> });
    const body = element.props.children;
    expect(body.props.className).toContain("font-sans");
  });

  it("should apply antialiased class to body", () => {
    const element = RootLayout({ children: <div>Test</div> });
    const body = element.props.children;
    expect(body.props.className).toContain("antialiased");
  });

  it("should include inter font variable in body", () => {
    const element = RootLayout({ children: <div>Test</div> });
    const body = element.props.children;
    expect(body.props.className).toContain("--font-inter");
  });

  it("should include jetbrains mono font variable in body", () => {
    const element = RootLayout({ children: <div>Test</div> });
    const body = element.props.children;
    expect(body.props.className).toContain("--font-jetbrains-mono");
  });

  it("should render children inside layout structure", () => {
    const testChild = <div data-testid="test">Content</div>;
    const element = RootLayout({ children: testChild });
    // The element should contain Providers > AuthWrapper > children structure
    expect(element).toBeDefined();
  });
});

describe("generateMetadata", () => {
  it("should default the title to the Omnia product name", () => {
    expect(generateMetadata().title).toBe("Omnia Dashboard");
  });

  it("should have description mentioning AI Agent", () => {
    expect(generateMetadata().description).toContain("AI Agent");
  });

  it("should have description mentioning Kubernetes", () => {
    expect(generateMetadata().description).toContain("Kubernetes");
  });

  it("should have favicon icon configured", () => {
    const icons = generateMetadata().icons as { icon: string };
    expect(icons).toBeDefined();
    expect(icons.icon).toBe("/favicon.svg");
  });
});
