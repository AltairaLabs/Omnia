import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { PrivacyPosture } from "./privacy-posture";

describe("PrivacyPosture", () => {
  it("renders the three sub-panels with values", () => {
    render(
      <PrivacyPosture
        stats={{
          totalUsers: 100,
          optedOutAll: 5,
          grantsByCategory: { "memory:context": 90, "memory:identity": 30 },
        }}
      />,
    );
    expect(screen.getByText(/Privacy posture/i)).toBeInTheDocument();
    expect(screen.getByText(/Consent grants by category/i)).toBeInTheDocument();
    expect(screen.getByText(/Opt-out rate/i)).toBeInTheDocument();
    expect(screen.getByText(/Redaction activity/i)).toBeInTheDocument();
    expect(screen.getByText("5.0%")).toBeInTheDocument(); // 5/100
    expect(screen.getByText("5 of 100 users")).toBeInTheDocument();
  });

  it("handles zero users gracefully", () => {
    render(
      <PrivacyPosture
        stats={{ totalUsers: 0, optedOutAll: 0, grantsByCategory: {} }}
      />,
    );
    expect(screen.getByText("0.0%")).toBeInTheDocument();
    expect(screen.getByText(/No consent data yet/i)).toBeInTheDocument();
  });

  it("flags the redaction panel as pending", () => {
    render(
      <PrivacyPosture
        stats={{ totalUsers: 1, optedOutAll: 0, grantsByCategory: {} }}
      />,
    );
    expect(screen.getByText(/Audit integration pending/i)).toBeInTheDocument();
  });
});
