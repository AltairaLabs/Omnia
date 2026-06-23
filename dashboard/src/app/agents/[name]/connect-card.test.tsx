import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import { ConnectCard } from "./connect-card";
import type { AgentRuntime, FacadeEndpoint } from "@/types/agent-runtime";

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
  });

  describe("(a) valid websocket endpoint", () => {
    it("renders the URL in a code element", () => {
      const agent = makeAgent({
        status: { facade: { endpoints: [validWssEndpoint] } },
      });
      render(<ConnectCard agent={agent} />);
      expect(screen.getByText("wss://agents.example.com/test-agent/ws")).toBeInTheDocument();
    });

    it("renders a copy button", () => {
      const agent = makeAgent({
        status: { facade: { endpoints: [validWssEndpoint] } },
      });
      render(<ConnectCard agent={agent} />);
      const btn = screen.getByRole("button", { name: /copy url/i });
      expect(btn).toBeInTheDocument();
    });

    it("renders scheme badge", () => {
      const agent = makeAgent({
        status: { facade: { endpoints: [validWssEndpoint] } },
      });
      render(<ConnectCard agent={agent} />);
      expect(screen.getByText("WSS")).toBeInTheDocument();
    });

    it("renders protocol badge", () => {
      const agent = makeAgent({
        status: { facade: { endpoints: [validWssEndpoint] } },
      });
      render(<ConnectCard agent={agent} />);
      expect(screen.getByText("websocket")).toBeInTheDocument();
    });
  });

  describe("(b) valid:false endpoint renders warning + reason", () => {
    it("renders Not connectable label", () => {
      const agent = makeAgent({
        status: { facade: { endpoints: [invalidEndpoint] } },
      });
      render(<ConnectCard agent={agent} />);
      expect(screen.getByText("Not connectable")).toBeInTheDocument();
    });

    it("renders the reason text", () => {
      const agent = makeAgent({
        status: { facade: { endpoints: [invalidEndpoint] } },
      });
      render(<ConnectCard agent={agent} />);
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
      render(<ConnectCard agent={agent} />);
      expect(
        screen.getByText(/no external endpoints/i),
      ).toBeInTheDocument();
    });

    it("renders internal-only empty state when facade is undefined", () => {
      const agent = makeAgent({ status: {} });
      render(<ConnectCard agent={agent} />);
      expect(
        screen.getByText(/no external endpoints/i),
      ).toBeInTheDocument();
    });

    it("renders internal-only empty state when status is undefined", () => {
      const agent = makeAgent({ status: undefined });
      render(<ConnectCard agent={agent} />);
      expect(
        screen.getByText(/no external endpoints/i),
      ).toBeInTheDocument();
    });

    it("renders a link to expose-agents docs", () => {
      const agent = makeAgent({ status: {} });
      render(<ConnectCard agent={agent} />);
      const link = screen.getByRole("link", { name: /expose agents externally/i });
      expect(link).toBeInTheDocument();
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
      render(<ConnectCard agent={agent} />);
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
      render(<ConnectCard agent={agent} />);
      expect(screen.getByText("OIDC")).toBeInTheDocument();
      expect(screen.getByText("https://auth.example.com")).toBeInTheDocument();
    });

    it("renders Management-plane only when no externalAuth", () => {
      const agent = makeAgent({
        status: { facade: { endpoints: [validWssEndpoint] } },
      });
      render(<ConnectCard agent={agent} />);
      expect(screen.getByText("Management-plane only")).toBeInTheDocument();
    });
  });
});
