import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import ExportDeployProfile from "./export-deploy-profile";

const discovery = {
  api_endpoint: "https://omnia.example.com",
  workspace: "team-acme",
  providers: [{ name: "default", role: "llm", type: "claude", model: "m" }],
  skills: [{ name: "docs-search", type: "git" }],
};

type Handler = (init?: RequestInit) => Partial<Response>;

function mockFetch(handlers: Record<string, Handler>) {
  global.fetch = vi.fn(async (url: string | URL | Request, init?: RequestInit) => {
    const u = url.toString();
    const key = Object.keys(handlers).find((k) => u.includes(k));
    if (!key) throw new Error(`unexpected fetch: ${u}`);
    const r = handlers[key](init);
    return { ok: true, status: 200, json: async () => ({}), ...r } as Response;
  }) as typeof fetch;
}

describe("ExportDeployProfile", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    Object.assign(navigator, { clipboard: { writeText: vi.fn() } });
    URL.createObjectURL = vi.fn(() => "blob:x");
    URL.revokeObjectURL = vi.fn();
    HTMLAnchorElement.prototype.click = vi.fn();
  });
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("mints a token, shows the config block, and supports copy + download", async () => {
    mockFetch({
      "/api/settings/api-keys": (init) =>
        init?.method === "POST"
          ? { json: async () => ({ key: { key: "omnia_sk_LIVE" } }), status: 201 }
          : { json: async () => ({ config: { allowCreate: true }, keys: [] }) },
      "/deploy-profile": () => ({ json: async () => discovery }),
    });
    render(<ExportDeployProfile workspace="team-acme" />);
    await userEvent.click(await screen.findByRole("button", { name: /export deploy profile/i }));

    const output = await screen.findByTestId("deploy-profile-output");
    await waitFor(() => expect(output.textContent).toContain("api_token: omnia_sk_LIVE"));
    expect(output.textContent).toContain("ref: default");
    expect(output.textContent).toContain("docs-search");

    await userEvent.click(screen.getByRole("button", { name: /copy/i }));
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith(output.textContent);

    await userEvent.click(screen.getByRole("button", { name: /download/i }));
    expect(URL.createObjectURL).toHaveBeenCalled();
  });

  it("shows a degraded note when the key store is read-only", async () => {
    mockFetch({
      "/api/settings/api-keys": () => ({ json: async () => ({ config: { allowCreate: false }, keys: [] }) }),
      "/deploy-profile": () => ({ json: async () => discovery }),
    });
    render(<ExportDeployProfile workspace="team-acme" />);
    await userEvent.click(await screen.findByRole("button", { name: /export deploy profile/i }));
    await waitFor(() => expect(screen.getByText(/minting is unavailable/i)).toBeInTheDocument());
  });

  it("surfaces the mint error when token minting fails", async () => {
    mockFetch({
      "/api/settings/api-keys": (init) =>
        init?.method === "POST"
          ? { ok: false, json: async () => ({ error: "store full" }) }
          : { json: async () => ({ config: { allowCreate: true }, keys: [] }) },
      "/deploy-profile": () => ({ json: async () => discovery }),
    });
    render(<ExportDeployProfile workspace="team-acme" />);
    await userEvent.click(await screen.findByRole("button", { name: /export deploy profile/i }));
    await waitFor(() => expect(screen.getByText(/store full/i)).toBeInTheDocument());
  });

  it("surfaces an error when discovery fails", async () => {
    mockFetch({
      "/api/settings/api-keys": () => ({ json: async () => ({ config: { allowCreate: true }, keys: [] }) }),
      "/deploy-profile": () => ({ ok: false, statusText: "nope" }),
    });
    render(<ExportDeployProfile workspace="team-acme" />);
    await userEvent.click(await screen.findByRole("button", { name: /export deploy profile/i }));
    await waitFor(() => expect(screen.getByText(/failed to fetch deploy profile/i)).toBeInTheDocument());
  });
});
