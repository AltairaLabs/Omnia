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
// WebSocket proxy runs on separate port to avoid interfering with Next.js HMR
const wsProxyPort = parseInt(process.env.WS_PROXY_PORT || "3002", 10);

// Service domain for K8s cluster DNS
const SERVICE_DOMAIN = process.env.SERVICE_DOMAIN || "svc.cluster.local";
// Default facade port
const DEFAULT_FACADE_PORT = parseInt(process.env.DEFAULT_FACADE_PORT || "8080", 10);
// PromptKit LSP service URL (overridable via env var)
const LSP_SERVICE_URL = process.env.LSP_SERVICE_URL || `ws://omnia-promptkit-lsp.omnia-system.${SERVICE_DOMAIN}:8080/lsp`;

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
 * Check if the path is an LSP WebSocket request.
 * Expected format: /api/lsp
 */
function isLspPath(pathname) {
  return pathname === "/api/lsp";
}

/**
 * Parse query parameters from URL.
 */
function parseQueryParams(url) {
  const params = {};
  const queryStart = url.indexOf("?");
  if (queryStart !== -1) {
    const queryString = url.slice(queryStart + 1);
    for (const pair of queryString.split("&")) {
      const [key, value] = pair.split("=");
      if (key) {
        params[decodeURIComponent(key)] = value ? decodeURIComponent(value) : "";
      }
    }
  }
  return params;
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
  console.log(`[WS Proxy] SERVICE_DOMAIN=${SERVICE_DOMAIN}, DEFAULT_FACADE_PORT=${DEFAULT_FACADE_PORT}`);

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

    upstream.on("message", (data, isBinary) => {
      if (clientSocket.readyState === WebSocket.OPEN) {
        clientSocket.send(data, { binary: isBinary });
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
      console.error(`[WS Proxy] Upstream error for ${namespace}/${name}:`);
      console.error(`[WS Proxy]   message: ${err.message}`);
      console.error(`[WS Proxy]   code: ${err.code}`);
      console.error(`[WS Proxy]   errno: ${err.errno}`);
      console.error(`[WS Proxy]   syscall: ${err.syscall}`);
      console.error(`[WS Proxy]   address: ${err.address}`);
      console.error(`[WS Proxy]   port: ${err.port}`);

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

    // Forward client messages to upstream (preserve binary/text frame type)
    clientSocket.on("message", (data, isBinary) => {
      if (upstream && upstream.readyState === WebSocket.OPEN) {
        upstream.send(data, { binary: isBinary });
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

/**
 * Proxy a WebSocket connection to the LSP service.
 */
function proxyLspWebSocket(clientSocket, workspace, project) {
  // Build upstream URL with query params
  const upstreamUrl = workspace && project
    ? `${LSP_SERVICE_URL}?workspace=${encodeURIComponent(workspace)}&project=${encodeURIComponent(project)}`
    : LSP_SERVICE_URL;

  console.log(`[WS LSP Proxy] Connecting to upstream: ${upstreamUrl}`);

  let upstream = null;
  let upstreamConnected = false;
  let connectionTimeout = null;

  // Set a connection timeout (10 seconds)
  connectionTimeout = setTimeout(() => {
    if (!upstreamConnected) {
      console.error(`[WS LSP Proxy] Connection timeout`);
      sendError(clientSocket, `Connection to LSP service timed out`, "CONNECTION_TIMEOUT");
      clientSocket.close(1011, "Connection timeout");
    }
  }, 10000);

  try {
    upstream = new WebSocket(upstreamUrl);

    upstream.on("open", () => {
      upstreamConnected = true;
      clearTimeout(connectionTimeout);
      console.log(`[WS LSP Proxy] Connected to LSP service`);
    });

    upstream.on("message", (data, isBinary) => {
      if (clientSocket.readyState === WebSocket.OPEN) {
        clientSocket.send(data, { binary: isBinary });
      }
    });

    upstream.on("close", (code, reason) => {
      clearTimeout(connectionTimeout);
      const reasonStr = reason ? reason.toString() : "";
      console.log(`[WS LSP Proxy] Upstream closed: ${code} ${reasonStr}`);
      if (clientSocket.readyState === WebSocket.OPEN) {
        if (!upstreamConnected) {
          sendError(clientSocket, `LSP service is not available`, "LSP_UNAVAILABLE");
        }
        clientSocket.close(sanitizeCloseCode(code), reasonStr || "Connection closed");
      }
    });

    upstream.on("error", (err) => {
      clearTimeout(connectionTimeout);
      console.error(`[WS LSP Proxy] Upstream error:`);
      console.error(`[WS LSP Proxy]   message: ${err.message}`);
      console.error(`[WS LSP Proxy]   code: ${err.code}`);

      let errorMessage = `Failed to connect to LSP service`;
      let errorCode = "CONNECTION_ERROR";

      if (err.code === "ENOTFOUND" || err.code === "EAI_AGAIN") {
        errorMessage = `LSP service not found. Check that enterprise features are enabled.`;
        errorCode = "LSP_NOT_FOUND";
      } else if (err.code === "ECONNREFUSED") {
        errorMessage = `LSP service is not accepting connections.`;
        errorCode = "CONNECTION_REFUSED";
      } else if (err.code === "ETIMEDOUT") {
        errorMessage = `Connection to LSP service timed out.`;
        errorCode = "CONNECTION_TIMEOUT";
      }

      sendError(clientSocket, errorMessage, errorCode);

      if (clientSocket.readyState === WebSocket.OPEN) {
        clientSocket.close(1011, "Upstream connection failed");
      }
    });

    // Forward client messages to upstream
    clientSocket.on("message", (data, isBinary) => {
      if (upstream && upstream.readyState === WebSocket.OPEN) {
        upstream.send(data, { binary: isBinary });
      } else if (!upstreamConnected) {
        console.warn(`[WS LSP Proxy] Client sent message before upstream connected`);
      }
    });

    clientSocket.on("close", (code, reason) => {
      clearTimeout(connectionTimeout);
      const reasonStr = reason ? reason.toString() : "";
      console.log(`[WS LSP Proxy] Client closed: ${code} ${reasonStr}`);
      if (upstream && upstream.readyState === WebSocket.OPEN) {
        upstream.close(sanitizeCloseCode(code), reasonStr);
      }
    });

    clientSocket.on("error", (err) => {
      clearTimeout(connectionTimeout);
      console.error(`[WS LSP Proxy] Client error:`, err.message);
      if (upstream && upstream.readyState === WebSocket.OPEN) {
        upstream.close(1011, "Client connection error");
      }
    });
  } catch (err) {
    clearTimeout(connectionTimeout);
    console.error(`[WS LSP Proxy] Failed to create upstream connection:`, err.message);
    sendError(clientSocket, `Failed to connect to LSP service: ${err.message}`, "CONNECTION_ERROR");
    clientSocket.close(1011, "Failed to connect to LSP service");
  }
}

app.prepare().then(() => {
  // Create main HTTP server for Next.js (no WebSocket handling - let HMR work)
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

  // Create separate WebSocket proxy server on different port
  const wsServer = createServer((req, res) => {
    res.writeHead(200, { "Content-Type": "text/plain" });
    res.end("WebSocket proxy server. Connect via WebSocket to /api/agents/{namespace}/{name}/ws");
  });

  const wss = new WebSocketServer({ noServer: true });

  wss.on("connection", (ws, req) => {
    const { pathname } = parse(req.url);
    const agent = parseAgentWsPath(pathname);

    if (agent) {
      proxyWebSocket(ws, agent.namespace, agent.name);
    } else if (isLspPath(pathname)) {
      // Parse query params for LSP context
      const params = parseQueryParams(req.url);
      proxyLspWebSocket(ws, params.workspace, params.project);
    } else {
      console.warn(`[WS] Unknown WebSocket path: ${pathname}`);
      ws.close(1008, "Unknown path");
    }
  });

  // Handle WebSocket upgrades on the proxy server
  wsServer.on("upgrade", (req, socket, head) => {
    const { pathname } = parse(req.url);
    console.log(`[WS Upgrade] Received upgrade request for: ${pathname}`);
    const agent = parseAgentWsPath(pathname);

    if (agent) {
      console.log(`[WS Upgrade] Parsed agent: namespace=${agent.namespace}, name=${agent.name}`);
      wss.handleUpgrade(req, socket, head, (ws) => {
        wss.emit("connection", ws, req);
      });
    } else if (isLspPath(pathname)) {
      console.log(`[WS Upgrade] LSP connection request`);
      wss.handleUpgrade(req, socket, head, (ws) => {
        wss.emit("connection", ws, req);
      });
    } else {
      console.log(`[WS Upgrade] Rejecting unknown path: ${pathname}`);
      socket.destroy();
    }
  });

  // Start both servers
  server.listen(port, hostname, () => {
    console.log(`> Ready on http://${hostname}:${port}`);
    console.log(`> Environment: NODE_ENV=${process.env.NODE_ENV}`);
  });

  wsServer.listen(wsProxyPort, hostname, () => {
    console.log(`> WebSocket proxy on ws://${hostname}:${wsProxyPort}`);
    console.log(`> Connect to /api/agents/{namespace}/{name}/ws`);
    console.log(`> Service domain: ${SERVICE_DOMAIN}`);
  });
});
