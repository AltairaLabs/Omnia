import { describe, it, expect } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { AgentErrorBanner } from "./agent-error-banner";

describe("AgentErrorBanner", () => {
  it("renders the legacy raw-text banner for unknown errors", () => {
    // Regression guard: the issue #1037 banner must NOT swallow
    // unrecognised errors. Anything the classifier doesn't match
    // falls through to the original plain-text display so debugging
    // surface doesn't shrink.
    const raw = "something completely random in the runtime";
    render(<AgentErrorBanner error={raw} />);
    expect(screen.getByText(raw)).toBeInTheDocument();
    // Structured banner affordances should be absent.
    expect(screen.queryByText(/show details/i)).not.toBeInTheDocument();
  });

  it("renders an invalid-credential headline + check-provider link with workspace", () => {
    const raw = "API_KEY_INVALID for generativelanguage.googleapis.com";
    render(<AgentErrorBanner error={raw} workspace="dev-agents" />);

    // Headline mentions auth + provider name.
    expect(screen.getByText(/authentication failed/i)).toBeInTheDocument();
    expect(screen.getByText(/gemini/i)).toBeInTheDocument();

    // Action link goes to the right provider page.
    const link = screen.getByRole("link", { name: /check provider/i });
    expect(link).toHaveAttribute(
      "href",
      "/providers/gemini-provider?namespace=dev-agents",
    );
  });

  it("falls back to /providers when provider can't be detected", () => {
    // No provider URL or name in the message, but still an
    // identifiable invalid-credential.
    const raw = "Authentication failed: invalid token (placeholder)";
    render(<AgentErrorBanner error={raw} workspace="dev-agents" />);
    const link = screen.getByRole("link", { name: /check providers/i });
    expect(link).toHaveAttribute("href", "/providers");
  });

  it("toggles raw-text disclosure", () => {
    const raw = "openai 401 unauthorized invalid_api_key";
    render(<AgentErrorBanner error={raw} workspace="dev-agents" />);

    // Raw text hidden by default — the headline is enough.
    expect(screen.queryByText(raw)).not.toBeInTheDocument();

    fireEvent.click(screen.getByText(/show details/i));
    // Now visible (rendered in a <pre>).
    expect(screen.getByText(raw)).toBeInTheDocument();

    fireEvent.click(screen.getByText(/hide details/i));
    expect(screen.queryByText(raw)).not.toBeInTheDocument();
  });

  it("handles rate_limited errors with neutral guidance instead of a link", () => {
    const raw = "status 429: rate limited, retry after 30s";
    render(<AgentErrorBanner error={raw} workspace="dev-agents" />);
    expect(screen.getByText(/rate limit hit/i)).toBeInTheDocument();
    // No "check provider" link — there's nothing to fix on the
    // provider page for a quota issue.
    expect(
      screen.queryByRole("link", { name: /check provider/i }),
    ).not.toBeInTheDocument();
  });

  it("handles provider_unavailable errors with retry guidance", () => {
    const raw = "dial tcp: lookup api.openai.com on 10.96.0.10:53: no such host";
    render(<AgentErrorBanner error={raw} workspace="dev-agents" />);
    expect(screen.getByText(/unreachable/i)).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: /check provider/i })).not.toBeInTheDocument();
  });
});
