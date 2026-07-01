import { describe, it, expect, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { BrandProvider } from "./brand-provider";
import { useBrand } from "@/hooks/use-brand";

vi.mock("@/lib/config", () => ({
  getRuntimeConfig: vi.fn().mockResolvedValue({
    brand: {
      productName: "Acme AI",
      logo: { light: "/l.svg", dark: "/d.svg" },
      favicon: "/f.svg",
      colors: { primary: "#ff0000" },
      customCss: "--radius: 1rem;",
    },
  }),
}));

function Probe() {
  const { brand } = useBrand();
  return <span>{brand.productName}</span>;
}

describe("BrandProvider", () => {
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
});
