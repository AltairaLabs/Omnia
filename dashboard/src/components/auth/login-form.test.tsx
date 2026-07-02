/**
 * Tests for LoginForm — verifies the heading is brand-aware (no hardcoded
 * "Omnia") so OAuth login is white-labeled for SI partners.
 */

import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { LoginForm } from "./login-form";
import { BrandContext } from "@/components/branding/brand-provider";
import { OMNIA_BRAND } from "@/lib/branding/types";

describe("LoginForm", () => {
  it("renders the default product name in the heading", () => {
    render(<LoginForm providerName="Google" />);
    expect(screen.getByText("Sign in to Omnia")).toBeInTheDocument();
  });

  it("renders a white-label product name from the brand config", () => {
    render(
      <BrandContext.Provider
        value={{
          brand: { ...OMNIA_BRAND, productName: "Acme Cloud" },
          setBrandOverride: () => {},
        }}
      >
        <LoginForm providerName="Google" />
      </BrandContext.Provider>,
    );
    expect(screen.getByText("Sign in to Acme Cloud")).toBeInTheDocument();
    expect(screen.queryByText("Sign in to Omnia")).not.toBeInTheDocument();
  });
});
