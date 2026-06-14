import { describe, it, expect, beforeEach } from "vitest";
import { useSidebarStore } from "./sidebar-store";

describe("sidebar-store", () => {
  beforeEach(() => {
    useSidebarStore.setState({ collapsed: false });
    window.localStorage.clear();
  });

  it("defaults to expanded", () => {
    expect(useSidebarStore.getState().collapsed).toBe(false);
  });

  it("toggle flips the collapsed flag", () => {
    useSidebarStore.getState().toggle();
    expect(useSidebarStore.getState().collapsed).toBe(true);
    useSidebarStore.getState().toggle();
    expect(useSidebarStore.getState().collapsed).toBe(false);
  });

  it("setCollapsed sets the flag explicitly", () => {
    useSidebarStore.getState().setCollapsed(true);
    expect(useSidebarStore.getState().collapsed).toBe(true);
  });

  it("persists collapsed under the omnia-sidebar key", () => {
    useSidebarStore.getState().setCollapsed(true);
    expect(window.localStorage.getItem("omnia-sidebar")).toContain("\"collapsed\":true");
  });
});
