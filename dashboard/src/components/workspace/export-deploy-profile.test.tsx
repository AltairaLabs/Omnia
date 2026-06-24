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

  it("scopes the minted token to the exporting workspace (#1561 P3)", async () => {
    let postBody: string | undefined;
    mockFetch({
      "/api/settings/api-keys": (init) => {
        if (init?.method === "POST") {
          postBody = init.body as string;
          return { json: async () => ({ key: { key: "omnia_sk_LIVE" } }), status: 201 };
        }
        return { json: async () => ({ config: { allowCreate: true }, keys: [] }) };
      },
      "/deploy-profile": () => ({ json: async () => discovery }),
    });
    render(<ExportDeployProfile workspace="team-acme" />);
    await userEvent.click(await screen.findByRole("button", { name: /export deploy profile/i }));
    await screen.findByTestId("deploy-profile-output");
    expect(JSON.parse(postBody as string)).toMatchObject({
      name: "deploy-team-acme",
      workspaces: ["team-acme"],
    });
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

  const existingKey = {
    id: "key-deploy",
    name: "deploy-team-acme",
    createdAt: new Date(Date.now() - 3 * 24 * 60 * 60 * 1000).toISOString(),
    lastUsedAt: null,
  };

  it("asks instead of minting when a deploy key already exists", async () => {
    const calls: { method?: string }[] = [];
    mockFetch({
      "/api/settings/api-keys": (init) => {
        calls.push({ method: init?.method });
        return { json: async () => ({ config: { allowCreate: true }, keys: [existingKey] }) };
      },
      "/deploy-profile": () => ({ json: async () => discovery }),
    });
    render(<ExportDeployProfile workspace="team-acme" />);
    await userEvent.click(await screen.findByRole("button", { name: /export deploy profile/i }));

    // The choice dialog appears; no POST (mint) was issued.
    await screen.findByText(/a deploy key already exists/i);
    expect(screen.getByRole("button", { name: /regenerate token/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /use saved token/i })).toBeInTheDocument();
    expect(calls.some((c) => c.method === "POST")).toBe(false);
  });

  it("regenerate revokes the old key, mints a new one, and shows the fresh token", async () => {
    let deleted = false;
    let posted = false;
    mockFetch({
      "/api/settings/api-keys": (init) => {
        if (init?.method === "DELETE") {
          deleted = true;
          return { json: async () => ({}) };
        }
        if (init?.method === "POST") {
          posted = true;
          return { json: async () => ({ key: { key: "omnia_sk_FRESH" } }), status: 201 };
        }
        return { json: async () => ({ config: { allowCreate: true }, keys: [existingKey] }) };
      },
      "/deploy-profile": () => ({ json: async () => discovery }),
    });
    render(<ExportDeployProfile workspace="team-acme" />);
    await userEvent.click(await screen.findByRole("button", { name: /export deploy profile/i }));
    await userEvent.click(await screen.findByRole("button", { name: /regenerate token/i }));

    const output = await screen.findByTestId("deploy-profile-output");
    await waitFor(() => expect(output.textContent).toContain("api_token: omnia_sk_FRESH"));
    expect(deleted).toBe(true);
    expect(posted).toBe(true);
  });

  it("use-saved-token shows a placeholder without revoking or minting", async () => {
    let mutated = false;
    mockFetch({
      "/api/settings/api-keys": (init) => {
        if (init?.method === "DELETE" || init?.method === "POST") mutated = true;
        return { json: async () => ({ config: { allowCreate: true }, keys: [existingKey] }) };
      },
      "/deploy-profile": () => ({ json: async () => discovery }),
    });
    render(<ExportDeployProfile workspace="team-acme" />);
    await userEvent.click(await screen.findByRole("button", { name: /export deploy profile/i }));
    await userEvent.click(await screen.findByRole("button", { name: /use saved token/i }));

    const output = await screen.findByTestId("deploy-profile-output");
    await waitFor(() =>
      expect(output.textContent).toContain("paste your saved deploy-team-acme token")
    );
    expect(mutated).toBe(false);
  });

  it("surfaces an error when regenerate fails to revoke the old key", async () => {
    mockFetch({
      "/api/settings/api-keys": (init) => {
        if (init?.method === "DELETE") {
          return { ok: false, json: async () => ({ error: "revoke failed" }) };
        }
        return { json: async () => ({ config: { allowCreate: true }, keys: [existingKey] }) };
      },
      "/deploy-profile": () => ({ json: async () => discovery }),
    });
    render(<ExportDeployProfile workspace="team-acme" />);
    await userEvent.click(await screen.findByRole("button", { name: /export deploy profile/i }));
    await userEvent.click(await screen.findByRole("button", { name: /regenerate token/i }));
    await waitFor(() => expect(screen.getByText(/revoke failed/i)).toBeInTheDocument());
  });

  it("treats an unreadable key listing as a read-only store", async () => {
    mockFetch({
      "/api/settings/api-keys": () => ({ ok: false, status: 500 }),
      "/deploy-profile": () => ({ json: async () => discovery }),
    });
    render(<ExportDeployProfile workspace="team-acme" />);
    await userEvent.click(await screen.findByRole("button", { name: /export deploy profile/i }));
    await waitFor(() => expect(screen.getByText(/minting is unavailable/i)).toBeInTheDocument());
  });
});
