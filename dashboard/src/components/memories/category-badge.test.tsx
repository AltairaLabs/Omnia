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
  it("returns blue hex for memory:identity", () => {
    expect(getCategoryColor("memory:identity")).toBe("#3b82f6");
  });

  it("returns gray hex for memory:context", () => {
    expect(getCategoryColor("memory:context")).toBe("#6b7280");
  });

  it("returns red hex for memory:health", () => {
    expect(getCategoryColor("memory:health")).toBe("#ef4444");
  });

  it("returns green hex for memory:location", () => {
    expect(getCategoryColor("memory:location")).toBe("#22c55e");
  });

  it("returns purple hex for memory:preferences", () => {
    expect(getCategoryColor("memory:preferences")).toBe("#a855f7");
  });

  it("returns amber hex for memory:history", () => {
    expect(getCategoryColor("memory:history")).toBe("#f59e0b");
  });

  it("returns gray fallback for unknown category", () => {
    expect(getCategoryColor("memory:unknown")).toBe("#6b7280");
  });

  it("returns gray fallback when category is undefined", () => {
    expect(getCategoryColor()).toBe("#6b7280");
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
