import { describe, it, expect } from "vitest";
import { render } from "@testing-library/react";
import { ProviderBindingIndicator } from "./provider-binding-indicator";
import type { ProviderBindingInfo } from "@/hooks/use-provider-binding-status";

describe("ProviderBindingIndicator", () => {
  it("should render green dot for bound status", () => {
    const info: ProviderBindingInfo = {
      status: "bound",
      providerName: "my-provider",
      providerNamespace: "default",
      message: "Bound to my-provider",
    };
    const { container } = render(<ProviderBindingIndicator bindingInfo={info} />);
    const dot = container.querySelector("span.bg-green-500");
    expect(dot).toBeInTheDocument();
  });

  it("should render blue dot for stale status", () => {
    const info: ProviderBindingInfo = {
      status: "stale",
      providerName: "deleted-provider",
      providerNamespace: "default",
      message: 'Provider "deleted-provider" not found in cluster',
    };
    const { container } = render(<ProviderBindingIndicator bindingInfo={info} />);
    const dot = container.querySelector("span.bg-blue-500");
    expect(dot).toBeInTheDocument();
  });

  it("should render yellow dot for unbound status", () => {
    const info: ProviderBindingInfo = {
      status: "unbound",
      message: "Not bound to a cluster provider",
    };
    const { container } = render(<ProviderBindingIndicator bindingInfo={info} />);
    const dot = container.querySelector("span.bg-yellow-500");
    expect(dot).toBeInTheDocument();
  });

  it("should have correct dimensions", () => {
    const info: ProviderBindingInfo = {
      status: "bound",
      message: "Bound to my-provider",
    };
    const { container } = render(<ProviderBindingIndicator bindingInfo={info} />);
    const dot = container.querySelector("span");
    expect(dot).toHaveClass("h-2", "w-2", "rounded-full");
  });
});
