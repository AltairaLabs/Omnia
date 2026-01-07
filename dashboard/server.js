/* eslint-disable @typescript-eslint/no-require-imports */
/**
 * Custom Next.js server with WebSocket proxy support.
 *
 * This server handles WebSocket upgrade requests for agent connections,
 * proxying them to the agent's facade service within Kubernetes.
 *
 * For regular HTTP requests, it delegates to Next.js.
 *
 * Architecture:
 *   Browser ──WebSocket──> /api/agents/{ns}/{name}/ws
 *                              │
 *                              ▼ (this proxy)
 *                          Agent Service {name}.{ns}.svc:{port}/ws
 */

const { createServer } = require("http");
const { parse } = require("url");
const next = require("next");
const { WebSocket, WebSocketServer } = require("ws");

const dev = process.env.NODE_ENV !== "production";
const hostname = process.env.HOSTNAME || "0.0.0.0";
const port = parseInt(process.env.PORT || "3000", 10);

// Service domain for K8s cluster DNS
const SERVICE_DOMAIN = process.env.SERVICE_DOMAIN || "svc.cluster.local";
// Default facade port
const DEFAULT_FACADE_PORT = parseInt(process.env.DEFAULT_FACADE_PORT || "8080", 10);

const app = next({ dev, hostname, port });
const handle = app.getRequestHandler();

/**
 * Parse WebSocket URL to extract namespace and agent name.
 * Expected format: /api/agents/{namespace}/{name}/ws
 */
function parseAgentWsPath(pathname) {
  const match = pathname.match(/^\/api\/agents\/([^/]+)\/([^/]+)\/ws$/);
  if (match) {
    return { namespace: match[1], name: match[2] };
  }
  return null;
}

/**
 * Build the upstream WebSocket URL for the agent's facade service.
 * The facade requires an `agent` query parameter.
 */
function getAgentWsUrl(namespace, name, facadePort = DEFAULT_FACADE_PORT) {
  return `ws://${name}.${namespace}.${SERVICE_DOMAIN}:${facadePort}/ws?agent=${encodeURIComponent(name)}`;
}

/**
 * Sanitize WebSocket close codes before sending.
 * Reserved codes (1004, 1005, 1006, 1015) cannot be sent in close frames.
 * Valid codes: 1000-1003, 1007-1011, 3000-4999
 */
function sanitizeCloseCode(code) {
  if (!code) return 1000;
  // Reserved codes that can't be sent
  if (code === 1004 || code === 1005 || code === 1006 || code === 1015) {
    return 1000; // Normal closure
  }
  // Check valid ranges
  if (code < 1000 || (code > 1011 && code < 3000) || code > 4999) {
    return 1000;
  }
  return code;
}

/**
 * Send an error message to the client in the protocol format.
 */
function sendError(clientSocket, message, code = "CONNECTION_ERROR") {
  if (clientSocket.readyState === WebSocket.OPEN) {
    try {
      clientSocket.send(JSON.stringify({
        type: "error",
        timestamp: new Date().toISOString(),
        error: {
          code,
          message,
        },
      }));
    } catch (err) {
      console.error(`[WS Proxy] Failed to send error message:`, err.message);
    }
  }
}

/**
 * Proxy a WebSocket connection to an agent's facade.
 */
function proxyWebSocket(clientSocket, namespace, name) {
  const upstreamUrl = getAgentWsUrl(namespace, name);
  console.log(`[WS Proxy] Connecting to upstream: ${upstreamUrl}`);

  let upstream = null;
  let upstreamConnected = false;
  let connectionTimeout = null;

  // Set a connection timeout (10 seconds)
  connectionTimeout = setTimeout(() => {
    if (!upstreamConnected) {
      console.error(`[WS Proxy] Connection timeout for ${namespace}/${name}`);
      sendError(clientSocket, `Connection to agent ${name} timed out`, "CONNECTION_TIMEOUT");
      clientSocket.close(1011, "Connection timeout");
    }
  }, 10000);

  try {
    upstream = new WebSocket(upstreamUrl);

    upstream.on("open", () => {
      upstreamConnected = true;
      clearTimeout(connectionTimeout);
      console.log(`[WS Proxy] Connected to ${namespace}/${name}`);
    });

    upstream.on("message", (data) => {
      if (clientSocket.readyState === WebSocket.OPEN) {
        clientSocket.send(data);
      }
    });

    upstream.on("close", (code, reason) => {
      clearTimeout(connectionTimeout);
      const reasonStr = reason ? reason.toString() : "";
      console.log(`[WS Proxy] Upstream closed: ${code} ${reasonStr}`);
      if (clientSocket.readyState === WebSocket.OPEN) {
        // If we never connected, send an error message first
        if (!upstreamConnected) {
          sendError(clientSocket, `Agent ${name} is not available`, "AGENT_UNAVAILABLE");
        }
        clientSocket.close(sanitizeCloseCode(code), reasonStr || "Connection closed");
      }
    });

    upstream.on("error", (err) => {
      clearTimeout(connectionTimeout);
      console.error(`[WS Proxy] Upstream error for ${namespace}/${name}:`, err.message);

      // Provide more helpful error messages based on error type
      let errorMessage = `Failed to connect to agent ${name}`;
      let errorCode = "CONNECTION_ERROR";

      if (err.code === "ENOTFOUND" || err.code === "EAI_AGAIN") {
        errorMessage = `Agent ${name} not found in namespace ${namespace}. Check that the agent exists and is running.`;
        errorCode = "AGENT_NOT_FOUND";
      } else if (err.code === "ECONNREFUSED") {
        errorMessage = `Agent ${name} is not accepting connections. The agent may be starting up.`;
        errorCode = "CONNECTION_REFUSED";
      } else if (err.code === "ETIMEDOUT") {
        errorMessage = `Connection to agent ${name} timed out.`;
        errorCode = "CONNECTION_TIMEOUT";
      }

      sendError(clientSocket, errorMessage, errorCode);

      if (clientSocket.readyState === WebSocket.OPEN) {
        clientSocket.close(1011, "Upstream connection failed");
      }
    });

    // Forward client messages to upstream
    clientSocket.on("message", (data) => {
      if (upstream && upstream.readyState === WebSocket.OPEN) {
        upstream.send(data);
      } else if (!upstreamConnected) {
        // Queue messages or inform client
        console.warn(`[WS Proxy] Client sent message before upstream connected`);
      }
    });

    clientSocket.on("close", (code, reason) => {
      clearTimeout(connectionTimeout);
      const reasonStr = reason ? reason.toString() : "";
      console.log(`[WS Proxy] Client closed: ${code} ${reasonStr}`);
      if (upstream && upstream.readyState === WebSocket.OPEN) {
        upstream.close(sanitizeCloseCode(code), reasonStr);
      }
    });

    clientSocket.on("error", (err) => {
      clearTimeout(connectionTimeout);
      console.error(`[WS Proxy] Client error:`, err.message);
      if (upstream && upstream.readyState === WebSocket.OPEN) {
        upstream.close(1011, "Client connection error");
      }
    });
  } catch (err) {
    clearTimeout(connectionTimeout);
    console.error(`[WS Proxy] Failed to create upstream connection:`, err.message);
    sendError(clientSocket, `Failed to connect to agent ${name}: ${err.message}`, "CONNECTION_ERROR");
    clientSocket.close(1011, "Failed to connect to agent");
  }
}

app.prepare().then(() => {
  // Create HTTP server
  const server = createServer(async (req, res) => {
    try {
      const parsedUrl = parse(req.url, true);
      await handle(req, res, parsedUrl);
    } catch (err) {
      console.error("Error handling request:", err);
      res.statusCode = 500;
      res.end("Internal Server Error");
    }
  });

  // Create WebSocket server (no server attached - we'll handle upgrades manually)
  const wss = new WebSocketServer({ noServer: true });

  wss.on("connection", (ws, req) => {
    const { pathname } = parse(req.url);
    const agent = parseAgentWsPath(pathname);

    if (agent) {
      proxyWebSocket(ws, agent.namespace, agent.name);
    } else {
      console.warn(`[WS] Unknown WebSocket path: ${pathname}`);
      ws.close(1008, "Unknown path");
    }
  });

  // Handle WebSocket upgrade requests
  server.on("upgrade", (req, socket, head) => {
    const { pathname } = parse(req.url);
    const agent = parseAgentWsPath(pathname);

    if (agent) {
      console.log(`[WS Upgrade] Agent connection: ${agent.namespace}/${agent.name}`);
      wss.handleUpgrade(req, socket, head, (ws) => {
        wss.emit("connection", ws, req);
      });
    } else if (pathname.startsWith("/_next/")) {
      // Let Next.js handle its own WebSocket paths (HMR, etc.)
      // In dev mode, Next.js manages HMR internally - just ignore these
      // The connection will be handled by Next.js's internal dev server
    } else {
      // Unknown WebSocket path - reject it
      console.log(`[WS Upgrade] Rejecting unknown path: ${pathname}`);
      socket.destroy();
    }
  });

  server.listen(port, hostname, () => {
    console.log(`> Ready on http://${hostname}:${port}`);
    console.log(`> WebSocket proxy enabled for /api/agents/{namespace}/{name}/ws`);
    console.log(`> Service domain: ${SERVICE_DOMAIN}`);
  });
});
