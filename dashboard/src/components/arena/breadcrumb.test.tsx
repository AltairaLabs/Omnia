/**
 * Tests for ArenaBreadcrumb component.
 */

import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { ArenaBreadcrumb } from "./breadcrumb";

describe("ArenaBreadcrumb", () => {
  it("renders Arena link as root", () => {
    render(<ArenaBreadcrumb items={[]} />);

    const arenaLink = screen.getByRole("link", { name: "Arena" });
    expect(arenaLink).toBeInTheDocument();
    expect(arenaLink).toHaveAttribute("href", "/arena");
  });

  it("renders single item with link", () => {
    const items = [{ label: "Sources", href: "/arena/sources" }];
    render(<ArenaBreadcrumb items={items} />);

    const sourcesLink = screen.getByRole("link", { name: "Sources" });
    expect(sourcesLink).toBeInTheDocument();
    expect(sourcesLink).toHaveAttribute("href", "/arena/sources");
  });

  it("renders single item without link", () => {
    const items = [{ label: "git-source" }];
    render(<ArenaBreadcrumb items={items} />);

    expect(screen.getByText("git-source")).toBeInTheDocument();
    // Should not be a link
    expect(screen.queryByRole("link", { name: "git-source" })).not.toBeInTheDocument();
  });

  it("renders multiple items with chevrons", () => {
    const items = [
      { label: "Sources", href: "/arena/sources" },
      { label: "git-source" },
    ];
    render(<ArenaBreadcrumb items={items} />);

    // Arena root
    expect(screen.getByRole("link", { name: "Arena" })).toBeInTheDocument();
    // Sources with link
    expect(screen.getByRole("link", { name: "Sources" })).toBeInTheDocument();
    // git-source as text (last item)
    expect(screen.getByText("git-source")).toBeInTheDocument();
  });

  it("applies proper accessibility label", () => {
    render(<ArenaBreadcrumb items={[]} />);

    const nav = screen.getByRole("navigation", { name: "Breadcrumb" });
    expect(nav).toBeInTheDocument();
  });

  it("applies font-medium class to last item", () => {
    const items = [
      { label: "Sources", href: "/arena/sources" },
      { label: "Current Source", href: "/arena/sources/current" },
    ];
    render(<ArenaBreadcrumb items={items} />);

    const lastLink = screen.getByRole("link", { name: "Current Source" });
    expect(lastLink).toHaveClass("font-medium");
  });
});
