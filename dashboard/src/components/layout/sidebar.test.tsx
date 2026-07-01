import { describe, it, expect, beforeEach, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Sidebar } from "./sidebar";
import { BrandContext } from "@/components/branding/brand-provider";
import { useSidebarStore } from "@/stores/sidebar-store";
import type { BrandConfig } from "@/lib/branding/types";

// next/navigation
vi.mock("next/navigation", () => ({ usePathname: () => "/" }));
// enterprise gates → controllable per-test; default true so existing tests pass
const mockShowEnterpriseNav = vi.fn(() => ({ showEnterpriseNav: true }));
vi.mock("@/components/license/license-gate", () => ({
  useShowEnterpriseNav: () => mockShowEnterpriseNav(),
}));
vi.mock("@/hooks/core", () => ({
  useEnterpriseConfig: () => ({ enterpriseEnabled: false }),
}));

function setMatchMedia(matches: boolean) {
  window.matchMedia = vi.fn().mockReturnValue({
    matches,
    media: "",
    onchange: null,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    addListener: vi.fn(),
    removeListener: vi.fn(),
    dispatchEvent: vi.fn(),
  }) as unknown as typeof window.matchMedia;
}

beforeEach(() => {
  mockShowEnterpriseNav.mockReturnValue({ showEnterpriseNav: true });
  useSidebarStore.setState({ collapsed: false });
  setMatchMedia(false);
  window.localStorage.clear();
});

describe("Sidebar", () => {
  it("shows the Omnia wordmark and labels when expanded", () => {
    render(<Sidebar />);
    expect(screen.getByText("Omnia")).toBeInTheDocument();
    expect(screen.getByText("Overview")).toBeInTheDocument();
  });

  it("renders a custom brand name and logo when provided", () => {
    const brand: BrandConfig = {
      productName: "Acme AI",
      logo: { light: "/l.svg", dark: "/acme-dark.svg" },
      favicon: "/f.svg",
    };
    render(
      <BrandContext.Provider value={{ brand, setBrandOverride: () => {} }}>
        <Sidebar />
      </BrandContext.Provider>,
    );
    expect(screen.getByText("Acme AI")).toBeInTheDocument();
    // logo alt reflects the brand name (proves the logo/alt swap off hardcoded "Omnia")
    expect(screen.getByAltText("Acme AI")).toBeInTheDocument();
    expect(screen.queryByText("Omnia")).not.toBeInTheDocument();
  });

  it("no longer lists Console (moved to the header)", () => {
    render(<Sidebar />);
    expect(screen.queryByText("Console")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Console")).not.toBeInTheDocument();
  });

  it("hides the wordmark and labels when collapsed", () => {
    useSidebarStore.setState({ collapsed: true });
    render(<Sidebar />);
    expect(screen.queryByText("Omnia")).not.toBeInTheDocument();
    expect(screen.queryByText("Overview")).not.toBeInTheDocument();
    // the link is still reachable by its aria-label
    expect(screen.getByLabelText("Overview")).toBeInTheDocument();
  });

  it("renders the enterprise tooltip label when collapsed", () => {
    useSidebarStore.setState({ collapsed: true });
    render(<Sidebar />);
    // Arena is the enterprise item; its link aria-label stays the plain name
    expect(screen.getByLabelText("Arena")).toBeInTheDocument();
  });

  it("toggle button flips the collapsed state", async () => {
    const user = userEvent.setup();
    render(<Sidebar />);
    expect(screen.getByText("Overview")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /collapse sidebar/i }));
    expect(useSidebarStore.getState().collapsed).toBe(true);
  });

  it("forces collapsed on a narrow viewport even when preference is expanded", () => {
    setMatchMedia(true);
    useSidebarStore.setState({ collapsed: false });
    render(<Sidebar />);
    expect(screen.queryByText("Omnia")).not.toBeInTheDocument();
    expect(screen.getByLabelText("Overview")).toBeInTheDocument();
  });

  it("hides Memories and Memory analytics nav when enterprise nav is off", () => {
    mockShowEnterpriseNav.mockReturnValue({ showEnterpriseNav: false });
    render(<Sidebar />);
    expect(screen.queryByText("Memories")).not.toBeInTheDocument();
    expect(screen.queryByText("Memory analytics")).not.toBeInTheDocument();
    expect(screen.getByText("Agents")).toBeInTheDocument(); // OSS item still shows
  });

  it("shows Memories and Memory analytics nav when enterprise nav is on", () => {
    mockShowEnterpriseNav.mockReturnValue({ showEnterpriseNav: true });
    render(<Sidebar />);
    expect(screen.getByText("Memories")).toBeInTheDocument();
    expect(screen.getByText("Memory analytics")).toBeInTheDocument();
  });
});
