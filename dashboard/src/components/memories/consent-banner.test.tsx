/**
 * Tests for ConsentBanner.
 *
 * Copyright 2026 Altaira Labs.
 * SPDX-License-Identifier: Apache-2.0
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

const { mockUseConsent, mockUseUpdateConsent } = vi.hoisted(() => ({
  mockUseConsent: vi.fn(),
  mockUseUpdateConsent: vi.fn(),
}));

vi.mock("@/hooks/use-consent", () => ({
  useConsent: mockUseConsent,
  useUpdateConsent: mockUseUpdateConsent,
}));

import { ConsentBanner } from "./consent-banner";

const mockMutate = vi.fn();

function setupMocks(grants: string[] = [], isLoading = false) {
  mockUseConsent.mockReturnValue({
    data: { grants, defaults: [], denied: [] },
    isLoading,
  });
  mockUseUpdateConsent.mockReturnValue({
    mutate: mockMutate,
    isPending: false,
  });
}

describe("ConsentBanner", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockMutate.mockReset();
  });

  it("renders loading skeleton when isLoading is true", () => {
    mockUseConsent.mockReturnValue({ data: undefined, isLoading: true });
    mockUseUpdateConsent.mockReturnValue({ mutate: mockMutate, isPending: false });

    render(<ConsentBanner />);

    const banner = screen.getByTestId("consent-banner");
    expect(banner).toBeInTheDocument();
    // No toggles rendered during loading
    expect(screen.queryByTestId("consent-toggle-memory:identity")).not.toBeInTheDocument();
  });

  it("renders all 6 category toggles when loaded", () => {
    setupMocks([]);
    render(<ConsentBanner />);

    expect(screen.getByTestId("consent-toggle-memory:identity")).toBeInTheDocument();
    expect(screen.getByTestId("consent-toggle-memory:location")).toBeInTheDocument();
    expect(screen.getByTestId("consent-toggle-memory:health")).toBeInTheDocument();
    expect(screen.getByTestId("consent-toggle-memory:preferences")).toBeInTheDocument();
    expect(screen.getByTestId("consent-toggle-memory:context")).toBeInTheDocument();
    expect(screen.getByTestId("consent-toggle-memory:history")).toBeInTheDocument();
  });

  it("shows Privacy Consent title with shield icon", () => {
    setupMocks([]);
    render(<ConsentBanner />);
    expect(screen.getByText("Privacy Consent")).toBeInTheDocument();
  });

  it("shows PII toggle as checked when category is in grants", () => {
    setupMocks(["memory:identity", "memory:health"]);
    render(<ConsentBanner />);

    const identityToggle = screen.getByTestId("consent-toggle-memory:identity");
    const locationToggle = screen.getByTestId("consent-toggle-memory:location");
    const healthToggle = screen.getByTestId("consent-toggle-memory:health");

    expect(identityToggle).toBeChecked();
    expect(locationToggle).not.toBeChecked();
    expect(healthToggle).toBeChecked();
  });

  it("shows PII toggle as unchecked when category is not in grants", () => {
    setupMocks([]);
    render(<ConsentBanner />);

    expect(screen.getByTestId("consent-toggle-memory:identity")).not.toBeChecked();
    expect(screen.getByTestId("consent-toggle-memory:location")).not.toBeChecked();
    expect(screen.getByTestId("consent-toggle-memory:health")).not.toBeChecked();
  });

  it("calls mutate with grants when PII toggle is turned on", async () => {
    const user = userEvent.setup();
    setupMocks([]);
    render(<ConsentBanner />);

    await user.click(screen.getByTestId("consent-toggle-memory:identity"));
    expect(mockMutate).toHaveBeenCalledWith({ grants: ["memory:identity"] });
  });

  it("calls mutate with revocations when PII toggle is turned off", async () => {
    const user = userEvent.setup();
    setupMocks(["memory:identity"]);
    render(<ConsentBanner />);

    await user.click(screen.getByTestId("consent-toggle-memory:identity"));
    expect(mockMutate).toHaveBeenCalledWith({ revocations: ["memory:identity"] });
  });

  it("default categories are always checked and disabled", () => {
    setupMocks([]);
    render(<ConsentBanner />);

    const prefsToggle = screen.getByTestId("consent-toggle-memory:preferences");
    const contextToggle = screen.getByTestId("consent-toggle-memory:context");
    const historyToggle = screen.getByTestId("consent-toggle-memory:history");

    expect(prefsToggle).toBeChecked();
    expect(prefsToggle).toBeDisabled();
    expect(contextToggle).toBeChecked();
    expect(contextToggle).toBeDisabled();
    expect(historyToggle).toBeChecked();
    expect(historyToggle).toBeDisabled();
  });

  it("shows (default) label for non-PII categories", () => {
    setupMocks([]);
    render(<ConsentBanner />);

    const defaultLabels = screen.getAllByText("(default)");
    expect(defaultLabels).toHaveLength(3);
  });

  it("disables PII toggles when mutation is pending", () => {
    mockUseConsent.mockReturnValue({ data: { grants: [], defaults: [], denied: [] }, isLoading: false });
    mockUseUpdateConsent.mockReturnValue({ mutate: mockMutate, isPending: true });

    render(<ConsentBanner />);

    expect(screen.getByTestId("consent-toggle-memory:identity")).toBeDisabled();
    expect(screen.getByTestId("consent-toggle-memory:location")).toBeDisabled();
    expect(screen.getByTestId("consent-toggle-memory:health")).toBeDisabled();
  });
});
