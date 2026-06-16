import { describe, it, expect } from "vitest";
import { buildAgentTopologyGraph } from "./agent-topology-model";

const base = {
  facades: [{ type: "websocket", port: 8080 }],
  framework: { type: "promptkit", version: "1.4.14" },
  promptPack: { name: "echo", version: "v3" },
  session: { type: "memory", ttl: "1h" },
  memoryEnabled: true,
};

describe("buildAgentTopologyGraph", () => {
  it("builds a facade node, a runtime node, and the three runtime children", () => {
    const { nodes } = buildAgentTopologyGraph(base);
    const types = nodes.map((n) => n.type);
    expect(types).toContain("agentFacade");
    expect(types).toContain("agentRuntime");
    expect(types).toContain("agentPromptPack");
    expect(types).toContain("agentSession");
    expect(types).toContain("agentMemory");
  });

  it("nests promptpack, session and memory inside the runtime node", () => {
    const { nodes } = buildAgentTopologyGraph(base);
    const children = nodes.filter((n) =>
      ["agentPromptPack", "agentSession", "agentMemory"].includes(n.type ?? ""),
    );
    expect(children).toHaveLength(3);
    for (const c of children) {
      expect(c.parentId).toBe("runtime");
    }
  });

  it("carries facade type and port into the facade node data", () => {
    const { nodes } = buildAgentTopologyGraph(base);
    const facade = nodes.find((n) => n.type === "agentFacade");
    expect(facade?.data).toMatchObject({ facadeType: "websocket", port: 8080 });
  });

  it("emits one facade node and one edge to runtime per facade", () => {
    const { nodes, edges } = buildAgentTopologyGraph({
      ...base,
      facades: [
        { type: "websocket", port: 8080 },
        { type: "grpc", port: 9090 },
      ],
    });
    expect(nodes.filter((n) => n.type === "agentFacade")).toHaveLength(2);
    expect(edges).toHaveLength(2);
    for (const e of edges) {
      expect(e.target).toBe("runtime");
    }
  });

  it("reflects memoryEnabled in the memory node data", () => {
    const on = buildAgentTopologyGraph(base);
    const off = buildAgentTopologyGraph({ ...base, memoryEnabled: false });
    expect(on.nodes.find((n) => n.type === "agentMemory")?.data).toMatchObject({ enabled: true });
    expect(off.nodes.find((n) => n.type === "agentMemory")?.data).toMatchObject({ enabled: false });
  });

  it("carries promptpack and session details into their node data", () => {
    const { nodes } = buildAgentTopologyGraph(base);
    expect(nodes.find((n) => n.type === "agentPromptPack")?.data).toMatchObject({
      name: "echo",
      version: "v3",
    });
    expect(nodes.find((n) => n.type === "agentSession")?.data).toMatchObject({
      sessionType: "memory",
      ttl: "1h",
    });
  });

  it("emits no edges and a single facade when given one facade", () => {
    const { nodes, edges } = buildAgentTopologyGraph(base);
    expect(nodes.filter((n) => n.type === "agentFacade")).toHaveLength(1);
    expect(edges).toHaveLength(1);
  });
});
