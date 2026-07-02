/**
 * Tests for the app-level loading boundary — a token-styled, brand-aware
 * suspense fallback.
 */

import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import Loading from "./loading";
import { OMNIA_BRAND } from "@/lib/branding/types";

describe("Loading boundary", () => {
  it("renders a brand-aware loading indicator", () => {
    render(<Loading />);
    expect(screen.getByText(new RegExp(OMNIA_BRAND.productName))).toBeInTheDocument();
    expect(screen.getByRole("status")).toBeInTheDocument();
  });
});
