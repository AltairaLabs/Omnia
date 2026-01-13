import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { ProviderIcon, getProviderColor, getProviderLabel } from "./provider-icons";

describe("ProviderIcon", () => {
  it.each([
    ["claude", "C", "#D97757"],
    ["openai", "O", "#10A37F"],
    ["gemini", "G", "#4285F4"],
    ["ollama", "L", "#1A1A1A"],
    ["mock", "M", "#6B7280"],
  ] as const)("renders %s provider icon with letter %s and color %s", (type, letter, color) => {
    render(<ProviderIcon type={type} />);

    const svg = screen.getByLabelText(getProviderLabel(type));
    expect(svg).toBeInTheDocument();

    // Check the circle has correct fill color
    const circle = svg.querySelector("circle");
    expect(circle).toHaveAttribute("fill", color);

    // Check the text contains the correct letter
    const text = svg.querySelector("text");
    expect(text).toHaveTextContent(letter);
  });

  it("renders with custom size", () => {
    render(<ProviderIcon type="claude" size={48} />);

    const svg = screen.getByLabelText("Claude");
    expect(svg).toHaveAttribute("width", "48");
    expect(svg).toHaveAttribute("height", "48");
  });

  it("renders with default size of 24", () => {
    render(<ProviderIcon type="openai" />);

    const svg = screen.getByLabelText("OpenAI");
    expect(svg).toHaveAttribute("width", "24");
    expect(svg).toHaveAttribute("height", "24");
  });

  it("applies custom className", () => {
    render(<ProviderIcon type="gemini" className="custom-class" />);

    const svg = screen.getByLabelText("Gemini");
    expect(svg).toHaveClass("custom-class");
  });

  it("falls back to mock config for unknown provider type", () => {
    // @ts-expect-error - testing unknown provider type
    render(<ProviderIcon type="unknown" />);

    // Should fall back to mock styling
    const svg = document.querySelector("svg");
    const circle = svg?.querySelector("circle");
    expect(circle).toHaveAttribute("fill", "#6B7280");
  });
});

describe("getProviderColor", () => {
  it.each([
    ["claude", "#D97757"],
    ["openai", "#10A37F"],
    ["gemini", "#4285F4"],
    ["ollama", "#1A1A1A"],
    ["mock", "#6B7280"],
  ] as const)("returns correct color for %s", (type, expectedColor) => {
    expect(getProviderColor(type)).toBe(expectedColor);
  });

  it("returns mock color for unknown provider", () => {
    // @ts-expect-error - testing unknown provider type
    expect(getProviderColor("unknown")).toBe("#6B7280");
  });
});

describe("getProviderLabel", () => {
  it.each([
    ["claude", "Claude"],
    ["openai", "OpenAI"],
    ["gemini", "Gemini"],
    ["ollama", "Ollama"],
    ["mock", "Mock"],
  ] as const)("returns correct label for %s", (type, expectedLabel) => {
    expect(getProviderLabel(type)).toBe(expectedLabel);
  });

  it("returns Unknown for unknown provider", () => {
    // @ts-expect-error - testing unknown provider type
    expect(getProviderLabel("unknown")).toBe("Unknown");
  });
});
