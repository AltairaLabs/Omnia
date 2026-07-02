import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { BrandProvider } from "./brand-provider";
import { useBrand } from "@/hooks/use-brand";
import type { BrandConfig } from "@/lib/branding/types";

const BASE: BrandConfig = {
  productName: "Acme AI",
  logo: { light: "/l.svg", dark: "/d.svg" },
  favicon: "/f.svg",
  colors: { primary: "#ff0000" },
  customCss: "--radius: 1rem;",
};

const state = vi.hoisted(() => ({ brand: null as BrandConfig | null }));

vi.mock("@/lib/config", () => ({
  getRuntimeConfig: () => Promise.resolve({ brand: state.brand }),
}));

function Probe() {
  const { brand } = useBrand();
  return <span>{brand.productName}</span>;
}

describe("BrandProvider", () => {
  beforeEach(() => {
    state.brand = { ...BASE };
    document
      .querySelectorAll("link[data-brand-font]")
      .forEach((l) => l.remove());
  });

  it("provides the resolved brand and injects :root vars", async () => {
    render(
      <BrandProvider>
        <Probe />
      </BrandProvider>,
    );
    await waitFor(() => expect(screen.getByText("Acme AI")).toBeInTheDocument());
    const style = document.getElementById("brand-vars");
    expect(style?.textContent).toContain("--primary: #ff0000");
  });

  it("appends token-scoped customCss inside :root only", async () => {
    render(
      <BrandProvider>
        <Probe />
      </BrandProvider>,
    );
    await waitFor(() => expect(screen.getByText("Acme AI")).toBeInTheDocument());
    const css = document.getElementById("brand-vars")?.textContent ?? "";
    expect(css).toContain("--radius: 1rem;");
    // customCss is only ever wrapped in :root {…}, never arbitrary selectors
    expect(css).not.toMatch(/\.[a-z]/i);
  });

  it("loads the brand webfont stylesheet and re-points the font family when fonts.url is set", async () => {
    state.brand = {
      ...BASE,
      fonts: { family: "Brand Sans", url: "https://fonts.example/brand.css" },
    };
    render(
      <BrandProvider>
        <Probe />
      </BrandProvider>,
    );
    await waitFor(() => expect(screen.getByText("Acme AI")).toBeInTheDocument());
    await waitFor(() => {
      const link = document.querySelector<HTMLLinkElement>("link[data-brand-font]");
      expect(link).not.toBeNull();
      expect(link?.getAttribute("rel")).toBe("stylesheet");
      expect(link?.getAttribute("href")).toBe("https://fonts.example/brand.css");
    });
    // The family also overrides --font-sans so the loaded webfont is applied.
    expect(document.getElementById("brand-vars")?.textContent).toContain("Brand Sans");
  });

  it("emits a .dark block for dark-mode surface overrides", async () => {
    state.brand = { ...BASE, colorsDark: { background: "#0a0a0a", card: "#151515" } };
    render(
      <BrandProvider>
        <Probe />
      </BrandProvider>,
    );
    await waitFor(() => expect(screen.getByText("Acme AI")).toBeInTheDocument());
    const css = document.getElementById("brand-vars")?.textContent ?? "";
    expect(css).toMatch(/\.dark\s*\{[^}]*--background:\s*#0a0a0a/);
    expect(css).toContain("--card: #151515");
  });

  it("injects no font link when the brand has no fonts.url", async () => {
    state.brand = { ...BASE, fonts: { family: "Brand Sans" } };
    render(
      <BrandProvider>
        <Probe />
      </BrandProvider>,
    );
    await waitFor(() => expect(screen.getByText("Acme AI")).toBeInTheDocument());
    expect(document.querySelector("link[data-brand-font]")).toBeNull();
  });
});
