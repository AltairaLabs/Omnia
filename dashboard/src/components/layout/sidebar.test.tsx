import { describe, it, expect, beforeEach, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Sidebar } from "./sidebar";
import { useSidebarStore } from "@/stores/sidebar-store";

// next/navigation
vi.mock("next/navigation", () => ({ usePathname: () => "/" }));
// enterprise gates → show everything, enterprise disabled (badge visible)
vi.mock("@/components/license/license-gate", () => ({
  useShowEnterpriseNav: () => ({ showEnterpriseNav: true }),
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
});
