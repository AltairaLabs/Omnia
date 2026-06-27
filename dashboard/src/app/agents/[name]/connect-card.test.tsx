import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ConnectCard } from "./connect-card";
import type { AgentRuntime, FacadeEndpoint } from "@/types/agent-runtime";

const mockIsEditor = vi.fn(() => true);
vi.mock("@/hooks/use-workspace-permissions", () => ({
  useWorkspacePermissions: () => ({ isEditor: mockIsEditor() }),
}));

const mockSave = vi.fn(async () => true);
vi.mock("@/hooks/use-set-agent-expose", () => ({
  useSetAgentExpose: () => ({ save: mockSave, saving: false, error: null }),
}));

// Mock navigator.clipboard
const mockWriteText = vi.fn();
Object.defineProperty(globalThis, "navigator", {
  writable: true,
  value: {
    clipboard: { writeText: mockWriteText },
  },
});

function makeAgent(overrides: Partial<AgentRuntime> = {}): AgentRuntime {
  return {
    apiVersion: "omnia.altairalabs.ai/v1alpha1",
    kind: "AgentRuntime",
    metadata: {
      name: "test-agent",
      namespace: "default",
      creationTimestamp: "2024-01-01T00:00:00Z",
    },
    spec: {
      promptPackRef: { name: "my-pack" },
      facade: { type: "websocket" },
    },
    status: {},
    ...overrides,
  };
}

const validWssEndpoint: FacadeEndpoint = {
  host: "agents.example.com",
  path: "/test-agent/ws",
  port: 443,
  protocol: "websocket",
  routeName: "test-agent-ws",
  routeNamespace: "default",
  scheme: "wss",
  url: "wss://agents.example.com/test-agent/ws",
  valid: true,
};

const invalidEndpoint: FacadeEndpoint = {
  host: "agents.example.com",
  path: "/test-agent/ws",
  port: 443,
  protocol: "websocket",
  routeName: "test-agent-ws-broken",
  routeNamespace: "default",
  scheme: "wss",
  url: "wss://agents.example.com/test-agent/ws",
  valid: false,
  reason: "path prefix is not stripped before the facade",
};

describe("ConnectCard", () => {
  beforeEach(() => {
    mockWriteText.mockReset();
    mockSave.mockReset();
    mockSave.mockResolvedValue(true);
    mockIsEditor.mockReturnValue(true);
  });

  describe("expose toggle (#1611)", () => {
    it("reflects spec.facade.expose.enabled and saves a toggle", async () => {
      const onExposeChange = vi.fn();
      const agent = makeAgent({
        spec: { promptPackRef: { name: "p" }, facade: { type: "websocket", expose: { enabled: false } } },
      });
      render(<ConnectCard agent={agent} workspace="ws1" onExposeChange={onExposeChange} />);

      const toggle = screen.getByRole("switch", { name: /expose externally/i });
      expect(toggle).not.toBeChecked();
      await userEvent.click(toggle);
      await userEvent.click(screen.getByRole("button", { name: /save/i }));

      expect(mockSave).toHaveBeenCalledWith(true, "");
      expect(onExposeChange).toHaveBeenCalled();
    });

    it("warns that exposure is unauthenticated when there is no externalAuth", async () => {
      const agent = makeAgent({
        spec: { promptPackRef: { name: "p" }, facade: { type: "websocket" } },
      });
      render(<ConnectCard agent={agent} workspace="ws1" />);
      await userEvent.click(screen.getByRole("switch", { name: /expose externally/i }));
      expect(screen.getByText(/does not authenticate it/i)).toBeInTheDocument();
    });

    it("disables the toggle for non-editors", () => {
      mockIsEditor.mockReturnValue(false);
      const agent = makeAgent({
        spec: { promptPackRef: { name: "p" }, facade: { type: "websocket", expose: { enabled: true } } },
      });
      render(<ConnectCard agent={agent} workspace="ws1" />);
      expect(screen.getByRole("switch", { name: /expose externally/i })).toBeDisabled();
      expect(screen.getByText(/editor access is required/i)).toBeInTheDocument();
    });
  });

  describe("(a) valid websocket endpoint", () => {
    it("renders the URL in a code element", () => {
      const agent = makeAgent({
        status: { facade: { endpoints: [validWssEndpoint] } },
      });
      render(<ConnectCard agent={agent} workspace="ws1" />);
      expect(screen.getByText("wss://agents.example.com/test-agent/ws")).toBeInTheDocument();
    });

    it("renders a copy button", () => {
      const agent = makeAgent({
        status: { facade: { endpoints: [validWssEndpoint] } },
      });
      render(<ConnectCard agent={agent} workspace="ws1" />);
      const btn = screen.getByRole("button", { name: /copy url/i });
      expect(btn).toBeInTheDocument();
    });

    it("renders scheme badge", () => {
      const agent = makeAgent({
        status: { facade: { endpoints: [validWssEndpoint] } },
      });
      render(<ConnectCard agent={agent} workspace="ws1" />);
      expect(screen.getByText("WSS")).toBeInTheDocument();
    });

    it("renders protocol badge", () => {
      const agent = makeAgent({
        status: { facade: { endpoints: [validWssEndpoint] } },
      });
      render(<ConnectCard agent={agent} workspace="ws1" />);
      expect(screen.getByText("websocket")).toBeInTheDocument();
    });
  });

  describe("(b) valid:false endpoint renders warning + reason", () => {
    it("renders Not connectable label", () => {
      const agent = makeAgent({
        status: { facade: { endpoints: [invalidEndpoint] } },
      });
      render(<ConnectCard agent={agent} workspace="ws1" />);
      expect(screen.getByText("Not connectable")).toBeInTheDocument();
    });

    it("renders the reason text", () => {
      const agent = makeAgent({
        status: { facade: { endpoints: [invalidEndpoint] } },
      });
      render(<ConnectCard agent={agent} workspace="ws1" />);
      expect(
        screen.getByText("path prefix is not stripped before the facade"),
      ).toBeInTheDocument();
    });
  });

  describe("(c) empty/undefined endpoints", () => {
    it("renders internal-only empty state when endpoints is empty array", () => {
      const agent = makeAgent({
        status: { facade: { endpoints: [] } },
      });
      render(<ConnectCard agent={agent} workspace="ws1" />);
      expect(
        screen.getByText(/no external endpoints/i),
      ).toBeInTheDocument();
    });

    it("renders internal-only empty state when facade is undefined", () => {
      const agent = makeAgent({ status: {} });
      render(<ConnectCard agent={agent} workspace="ws1" />);
      expect(
        screen.getByText(/no external endpoints/i),
      ).toBeInTheDocument();
    });

    it("renders internal-only empty state when status is undefined", () => {
      const agent = makeAgent({ status: undefined });
      render(<ConnectCard agent={agent} workspace="ws1" />);
      expect(
        screen.getByText(/no external endpoints/i),
      ).toBeInTheDocument();
    });

    it("renders a link to expose-agents docs", () => {
      const agent = makeAgent({ status: {} });
      render(<ConnectCard agent={agent} workspace="ws1" />);
      const link = screen.getByRole("link", { name: /expose agents externally/i });
      expect(link).toBeInTheDocument();
      expect(link).toHaveAttribute("href", "https://omnia.altairalabs.ai/how-to/expose-agents/");
    });
  });

  describe("(d) auth hint rendering", () => {
    it("renders Bearer token + Secret name for sharedToken", () => {
      const agent = makeAgent({
        spec: {
          promptPackRef: { name: "my-pack" },
          facade: { type: "websocket" },
          externalAuth: {
            sharedToken: { secretRef: { name: "my-agent-token" } },
          },
        },
        status: { facade: { endpoints: [validWssEndpoint] } },
      });
      render(<ConnectCard agent={agent} workspace="ws1" />);
      expect(screen.getByText("Bearer token")).toBeInTheDocument();
      expect(screen.getByText("Secret `my-agent-token`")).toBeInTheDocument();
    });

    it("renders OIDC hint when oidc is set", () => {
      const agent = makeAgent({
        spec: {
          promptPackRef: { name: "my-pack" },
          facade: { type: "websocket" },
          externalAuth: {
            oidc: { issuer: "https://auth.example.com", audience: "my-agent" },
          },
        },
        status: { facade: { endpoints: [validWssEndpoint] } },
      });
      render(<ConnectCard agent={agent} workspace="ws1" />);
      expect(screen.getByText("OIDC")).toBeInTheDocument();
      expect(screen.getByText("https://auth.example.com")).toBeInTheDocument();
    });

    it("renders Management-plane only when no externalAuth", () => {
      const agent = makeAgent({
        status: { facade: { endpoints: [validWssEndpoint] } },
      });
      render(<ConnectCard agent={agent} workspace="ws1" />);
      expect(screen.getByText("Management-plane only")).toBeInTheDocument();
    });
  });
});
