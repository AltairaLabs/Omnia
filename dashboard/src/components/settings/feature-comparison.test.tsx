/**
 * Tests for FeatureComparison component.
 */

import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { FeatureComparison } from "./feature-comparison";

describe("FeatureComparison", () => {
  it("should render the feature comparison card", () => {
    render(<FeatureComparison />);

    expect(screen.getByText("Feature Comparison")).toBeInTheDocument();
    expect(screen.getByText(/Compare Open Core and Enterprise/)).toBeInTheDocument();
  });

  it("should render table headers for Open Core and Enterprise", () => {
    render(<FeatureComparison />);

    expect(screen.getByRole("columnheader", { name: "Open Core" })).toBeInTheDocument();
    expect(screen.getByRole("columnheader", { name: "Enterprise" })).toBeInTheDocument();
  });

  it("should render Source Types category", () => {
    render(<FeatureComparison />);

    expect(screen.getByText("Source Types")).toBeInTheDocument();
    expect(screen.getByText("ConfigMap Sources")).toBeInTheDocument();
    expect(screen.getByText("Git Repository Sources")).toBeInTheDocument();
    expect(screen.getByText("OCI Registry Sources")).toBeInTheDocument();
    expect(screen.getByText("S3 Bucket Sources")).toBeInTheDocument();
  });

  it("should render Job Types category", () => {
    render(<FeatureComparison />);

    expect(screen.getByText("Job Types")).toBeInTheDocument();
    expect(screen.getByText("Evaluation Jobs")).toBeInTheDocument();
    expect(screen.getByText("Load Testing Jobs")).toBeInTheDocument();
    expect(screen.getByText("Data Generation Jobs")).toBeInTheDocument();
  });

  it("should render Execution Features category", () => {
    render(<FeatureComparison />);

    expect(screen.getByText("Execution Features")).toBeInTheDocument();
    expect(screen.getByText("Manual Job Execution")).toBeInTheDocument();
    expect(screen.getByText("Scheduled Jobs (Cron)")).toBeInTheDocument();
    expect(screen.getByText("Distributed Workers")).toBeInTheDocument();
  });

  it("should render Limits category", () => {
    render(<FeatureComparison />);

    expect(screen.getByText("Limits")).toBeInTheDocument();
    expect(screen.getByText("Max Scenarios per Job")).toBeInTheDocument();
    expect(screen.getByText("Max Worker Replicas")).toBeInTheDocument();
  });

  it("should show 10 as open core scenario limit", () => {
    render(<FeatureComparison />);

    expect(screen.getByText("10")).toBeInTheDocument();
  });

  it("should show 1 as open core replica limit", () => {
    render(<FeatureComparison />);

    expect(screen.getByText("1")).toBeInTheDocument();
  });

  it("should show Unlimited for enterprise limits", () => {
    render(<FeatureComparison />);

    const unlimitedElements = screen.getAllByText("Unlimited");
    expect(unlimitedElements.length).toBe(2);
  });

  it("should render checkmark icons for enabled features", () => {
    render(<FeatureComparison />);

    // ConfigMap Sources should have checkmarks for both tiers
    // Lucide icons may have different class name patterns
    const checkIcons = document.querySelectorAll('[class*="lucide"][class*="check"]');
    expect(checkIcons.length).toBeGreaterThan(0);
  });

  it("should render x icons for disabled open-core features", () => {
    render(<FeatureComparison />);

    // Features like Git Sources should have X for open core
    // Lucide icons use class like "lucide lucide-circle-x"
    const xIcons = document.querySelectorAll('[class*="lucide-circle-x"]');
    expect(xIcons.length).toBeGreaterThan(0);
  });
});
