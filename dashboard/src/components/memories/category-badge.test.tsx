/**
 * Tests for CategoryBadge, getCategoryColor, getCategoryLabel.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { CategoryBadge, getCategoryColor, getCategoryLabel } from "./category-badge";

describe("CategoryBadge", () => {
  const knownCategories: Array<[string, string]> = [
    ["memory:identity", "Identity"],
    ["memory:context", "Context"],
    ["memory:health", "Health"],
    ["memory:location", "Location"],
    ["memory:preferences", "Preferences"],
    ["memory:history", "History"],
  ];

  it.each(knownCategories)(
    "renders correct label for category %s",
    (category, expectedLabel) => {
      render(<CategoryBadge category={category} />);
      expect(screen.getByTestId("category-badge")).toHaveTextContent(expectedLabel);
    }
  );

  it("renders 'Unknown' for an unknown category", () => {
    render(<CategoryBadge category="memory:banana" />);
    expect(screen.getByTestId("category-badge")).toHaveTextContent("Unknown");
  });

  it("renders 'Unknown' when category is undefined", () => {
    render(<CategoryBadge />);
    expect(screen.getByTestId("category-badge")).toHaveTextContent("Unknown");
  });
});

describe("getCategoryColor", () => {
  it("returns the category token var for known categories", () => {
    expect(getCategoryColor("memory:identity")).toBe("var(--category-1)");
    expect(getCategoryColor("memory:health")).toBe("var(--category-7)");
  });

  it("returns the neutral category token var for unknown/undefined", () => {
    expect(getCategoryColor("memory:unknown")).toBe("var(--category-8)");
    expect(getCategoryColor()).toBe("var(--category-8)");
  });

  it("never returns a raw hex color", () => {
    expect(getCategoryColor("memory:identity")).not.toMatch(/^#/);
  });
});

describe("getCategoryLabel", () => {
  it("returns correct label for known categories", () => {
    expect(getCategoryLabel("memory:identity")).toBe("Identity");
    expect(getCategoryLabel("memory:health")).toBe("Health");
  });

  it("returns 'Context' for unknown category", () => {
    expect(getCategoryLabel("memory:banana")).toBe("Context");
  });

  it("returns 'Context' when category is undefined", () => {
    expect(getCategoryLabel()).toBe("Context");
  });
});
