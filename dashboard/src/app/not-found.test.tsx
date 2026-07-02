/**
 * Tests for the app-level 404 boundary. It must render a branded, token-styled
 * page (not the unbranded Next.js default) with a link back to the dashboard.
 */

import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import NotFound from "./not-found";
import { OMNIA_BRAND } from "@/lib/branding/types";

describe("NotFound boundary", () => {
  it("renders a branded 404 with a link home", () => {
    render(<NotFound />);
    expect(screen.getByText(OMNIA_BRAND.productName)).toBeInTheDocument();
    expect(screen.getByText("404")).toBeInTheDocument();
    expect(screen.getByText(/page not found/i)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /dashboard/i })).toHaveAttribute("href", "/");
  });
});
