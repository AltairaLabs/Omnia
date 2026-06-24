import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import ExportDeployProfile from "./export-deploy-profile";

const discovery = {
  api_endpoint: "https://omnia.example.com",
  workspace: "team-acme",
  providers: [
    { name: "rag-baseline", role: "llm", type: "claude" },
    { name: "rag-candidate", role: "llm", type: "claude" },
    { name: "embedder", role: "embedding", type: "openai" },
  ],
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

const listing = (allowCreate: boolean, keys: unknown[] = []) => ({
  json: async () => ({ config: { allowCreate }, keys }),
});

async function openConfigure(workspace = "team-acme") {
  render(<ExportDeployProfile workspace={workspace} />);
  await userEvent.click(await screen.findByRole("button", { name: /export deploy profile/i }));
  await screen.findByText(/configure deploy profile/i);
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

  it("opens a configure dialog listing the Ready providers and skills", async () => {
    mockFetch({
      "/api/settings/api-keys": () => listing(true),
      "/deploy-profile": () => ({ json: async () => discovery }),
    });
    await openConfigure();
    expect(screen.getByText("rag-baseline")).toBeInTheDocument();
    expect(screen.getByText("rag-candidate")).toBeInTheDocument();
    expect(screen.getByText("embedder")).toBeInTheDocument();
    expect(screen.getByText("docs-search")).toBeInTheDocument();
  });

  it("generates with the first LLM as default + a minted token, supports copy/download", async () => {
    let postBody: string | undefined;
    mockFetch({
      "/api/settings/api-keys": (init) => {
        if (init?.method === "POST") {
          postBody = init.body as string;
          return { json: async () => ({ key: { key: "omnia_sk_LIVE" } }), status: 201 };
        }
        return listing(true);
      },
      "/deploy-profile": () => ({ json: async () => discovery }),
    });
    await openConfigure();
    await userEvent.click(screen.getByRole("button", { name: /^generate$/i }));

    const output = await screen.findByTestId("deploy-profile-output");
    await waitFor(() => expect(output.textContent).toContain("api_token: omnia_sk_LIVE"));
    // first llm (rag-baseline) promoted to the "default" binding
    expect(output.textContent).toContain("name: default");
    expect(output.textContent).toContain("ref: rag-baseline");
    expect(output.textContent).toContain("docs-search");
    // token scoped to the workspace
    expect(JSON.parse(postBody as string)).toMatchObject({
      name: "deploy-team-acme",
      workspaces: ["team-acme"],
    });

    await userEvent.click(screen.getByRole("button", { name: /copy/i }));
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith(output.textContent);
    await userEvent.click(screen.getByRole("button", { name: /download/i }));
    expect(URL.createObjectURL).toHaveBeenCalled();
  });

  it("lets the user choose a different default LLM", async () => {
    mockFetch({
      "/api/settings/api-keys": (init) =>
        init?.method === "POST"
          ? { json: async () => ({ key: { key: "omnia_sk_LIVE" } }), status: 201 }
          : listing(true),
      "/deploy-profile": () => ({ json: async () => discovery }),
    });
    await openConfigure();
    await userEvent.click(screen.getByRole("radio", { name: /set rag-candidate as default/i }));
    await userEvent.click(screen.getByRole("button", { name: /^generate$/i }));

    const output = await screen.findByTestId("deploy-profile-output");
    await waitFor(() => expect(output.textContent).toContain("ref: rag-candidate"));
    expect(output.textContent).toContain("name: default");
  });

  it("excludes an unchecked provider from the config", async () => {
    mockFetch({
      "/api/settings/api-keys": (init) =>
        init?.method === "POST"
          ? { json: async () => ({ key: { key: "omnia_sk_LIVE" } }), status: 201 }
          : listing(true),
      "/deploy-profile": () => ({ json: async () => discovery }),
    });
    await openConfigure();
    await userEvent.click(screen.getByLabelText("embedder")); // uncheck
    await userEvent.click(screen.getByRole("button", { name: /^generate$/i }));

    const output = await screen.findByTestId("deploy-profile-output");
    await waitFor(() => expect(output.textContent).toContain("name: default"));
    expect(output.textContent).not.toContain("embedder");
  });

  it("asks regenerate/reuse when a deploy key exists; regenerate revokes + mints", async () => {
    const existing = {
      id: "key-deploy",
      name: "deploy-team-acme",
      createdAt: new Date(Date.now() - 3 * 86400000).toISOString(),
      lastUsedAt: null,
    };
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
        return listing(true, [existing]);
      },
      "/deploy-profile": () => ({ json: async () => discovery }),
    });
    await openConfigure();
    expect(screen.getByText(/a deploy key/i)).toBeInTheDocument();
    // regenerate is the default selection
    await userEvent.click(screen.getByRole("button", { name: /^generate$/i }));

    const output = await screen.findByTestId("deploy-profile-output");
    await waitFor(() => expect(output.textContent).toContain("api_token: omnia_sk_FRESH"));
    expect(deleted).toBe(true);
    expect(posted).toBe(true);
  });

  it("reuse emits a placeholder without revoking or minting", async () => {
    const existing = {
      id: "key-deploy",
      name: "deploy-team-acme",
      createdAt: new Date().toISOString(),
      lastUsedAt: null,
    };
    let mutated = false;
    mockFetch({
      "/api/settings/api-keys": (init) => {
        if (init?.method === "DELETE" || init?.method === "POST") mutated = true;
        return listing(true, [existing]);
      },
      "/deploy-profile": () => ({ json: async () => discovery }),
    });
    await openConfigure();
    await userEvent.click(screen.getByRole("radio", { name: "reuse" }));
    await userEvent.click(screen.getByRole("button", { name: /^generate$/i }));

    const output = await screen.findByTestId("deploy-profile-output");
    await waitFor(() =>
      expect(output.textContent).toContain("paste your saved deploy-team-acme token")
    );
    expect(mutated).toBe(false);
  });

  it("read-only store → placeholder token, no mint", async () => {
    let mutated = false;
    mockFetch({
      "/api/settings/api-keys": (init) => {
        if (init?.method === "POST") mutated = true;
        return listing(false);
      },
      "/deploy-profile": () => ({ json: async () => discovery }),
    });
    await openConfigure();
    await userEvent.click(screen.getByRole("button", { name: /^generate$/i }));
    const output = await screen.findByTestId("deploy-profile-output");
    await waitFor(() => expect(output.textContent).toContain("mint a token in Settings"));
    expect(mutated).toBe(false);
  });

  it("disables Generate and warns when there is no Ready LLM", async () => {
    mockFetch({
      "/api/settings/api-keys": () => listing(true),
      "/deploy-profile": () => ({
        json: async () => ({
          ...discovery,
          providers: [{ name: "embedder", role: "embedding", type: "openai" }],
        }),
      }),
    });
    await openConfigure();
    expect(screen.getByText(/no ready llm provider/i)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^generate$/i })).toBeDisabled();
  });

  it("surfaces an error when discovery fails", async () => {
    mockFetch({
      "/api/settings/api-keys": () => listing(true),
      "/deploy-profile": () => ({ ok: false, statusText: "nope" }),
    });
    render(<ExportDeployProfile workspace="team-acme" />);
    await userEvent.click(await screen.findByRole("button", { name: /export deploy profile/i }));
    await waitFor(() => expect(screen.getByText(/failed to fetch deploy profile/i)).toBeInTheDocument());
  });

  it("surfaces an error when regenerate fails to revoke", async () => {
    const existing = {
      id: "key-deploy", name: "deploy-team-acme",
      createdAt: new Date().toISOString(), lastUsedAt: null,
    };
    mockFetch({
      "/api/settings/api-keys": (init) =>
        init?.method === "DELETE"
          ? { ok: false, json: async () => ({ error: "revoke failed" }) }
          : listing(true, [existing]),
      "/deploy-profile": () => ({ json: async () => discovery }),
    });
    await openConfigure();
    await userEvent.click(screen.getByRole("button", { name: /^generate$/i }));
    await waitFor(() => expect(screen.getByText(/revoke failed/i)).toBeInTheDocument());
  });
});
