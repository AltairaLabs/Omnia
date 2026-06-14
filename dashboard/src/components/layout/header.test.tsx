/**
 * Tests for header component.
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { Header } from "./header";

// Mock next-themes
const mockSetTheme = vi.fn();
vi.mock("next-themes", () => ({
  useTheme: () => ({
    theme: "light",
    setTheme: mockSetTheme,
  }),
}));

// Mock the UserMenu
vi.mock("./user-menu", () => ({
  UserMenu: () => <div data-testid="user-menu">UserMenu</div>,
}));

// Mock the WorkspaceSwitcher
vi.mock("@/components/workspace-switcher", () => ({
  WorkspaceSwitcher: () => <div data-testid="workspace-switcher">WorkspaceSwitcher</div>,
}));

// Mock next/navigation's usePathname (drives the Console active state)
const mockUsePathname = vi.fn(() => "/");
vi.mock("next/navigation", () => ({
  usePathname: () => mockUsePathname(),
}));

// Mock react-query so Header's useQueryClient/useIsFetching work without a provider.
const mockInvalidateQueries = vi.fn();
vi.mock("@tanstack/react-query", () => ({
  useQueryClient: () => ({ invalidateQueries: mockInvalidateQueries }),
  useIsFetching: () => 0,
}));

describe("Header", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUsePathname.mockReturnValue("/");
  });

  it("renders title", () => {
    render(<Header title="Test Title" />);

    expect(screen.getByText("Test Title")).toBeInTheDocument();
  });

  it("renders description when provided", () => {
    render(<Header title="Title" description="Test description" />);

    expect(screen.getByText("Test description")).toBeInTheDocument();
  });

  it("does not render description when not provided", () => {
    render(<Header title="Title" />);

    const description = screen.queryByText("Test description");
    expect(description).not.toBeInTheDocument();
  });

  it("renders children when provided", () => {
    render(
      <Header title="Title">
        <button>Custom Button</button>
      </Header>
    );

    expect(screen.getByText("Custom Button")).toBeInTheDocument();
  });

  it("renders UserMenu component", () => {
    render(<Header title="Title" />);

    expect(screen.getByTestId("user-menu")).toBeInTheDocument();
  });

  it("renders WorkspaceSwitcher component", () => {
    render(<Header title="Title" />);

    expect(screen.getByTestId("workspace-switcher")).toBeInTheDocument();
  });

  it("renders theme toggle button", () => {
    render(<Header title="Title" />);

    expect(screen.getByTestId("theme-toggle")).toBeInTheDocument();
  });

  it("toggles theme when theme button is clicked", () => {
    render(<Header title="Title" />);

    const themeToggle = screen.getByTestId("theme-toggle");
    fireEvent.click(themeToggle);

    expect(mockSetTheme).toHaveBeenCalledWith("dark");
  });

  it("renders refresh button", () => {
    render(<Header title="Title" />);
    expect(screen.getByRole("button", { name: "Refresh data" })).toBeInTheDocument();
  });

  it("invalidates queries when the refresh button is clicked", () => {
    render(<Header title="Title" />);
    fireEvent.click(screen.getByRole("button", { name: "Refresh data" }));
    expect(mockInvalidateQueries).toHaveBeenCalled();
  });

  it("renders a Console link pointing at /console", () => {
    render(<Header title="Title" />);
    const link = screen.getByTestId("console-link");
    expect(link).toHaveAttribute("href", "/console");
    expect(link).toHaveAccessibleName("Open Console");
  });

  it("does not highlight the Console link off the console route", () => {
    mockUsePathname.mockReturnValue("/agents");
    render(<Header title="Title" />);
    expect(screen.getByTestId("console-link")).toHaveAttribute("data-active", "false");
  });

  it("highlights the Console link on the console route", () => {
    mockUsePathname.mockReturnValue("/console");
    render(<Header title="Title" />);
    expect(screen.getByTestId("console-link")).toHaveAttribute("data-active", "true");
  });

  it("accepts ReactNode as title", () => {
    render(
      <Header title={<span data-testid="custom-title">Custom Title</span>} />
    );

    expect(screen.getByTestId("custom-title")).toBeInTheDocument();
  });

  it("accepts ReactNode as description", () => {
    render(
      <Header
        title="Title"
        description={<span data-testid="custom-description">Custom Desc</span>}
      />
    );

    expect(screen.getByTestId("custom-description")).toBeInTheDocument();
  });
});
