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
const port = Number.parseInt(process.env.PORT || "3000", 10);
// WebSocket proxy runs on separate port to avoid interfering with Next.js HMR
const wsProxyPort = Number.parseInt(process.env.WS_PROXY_PORT || "3002", 10);

// Service domain for K8s cluster DNS
const SERVICE_DOMAIN = process.env.SERVICE_DOMAIN || "svc.cluster.local";
// Default facade port
const DEFAULT_FACADE_PORT = Number.parseInt(process.env.DEFAULT_FACADE_PORT || "8080", 10);
// PromptKit LSP service URL (overridable via env var)
const LSP_SERVICE_URL = process.env.LSP_SERVICE_URL || `ws://omnia-promptkit-lsp.omnia-system.${SERVICE_DOMAIN}:8080/lsp`;
// Arena Dev Console service name (deployed per workspace namespace)
const DEV_CONSOLE_SERVICE_NAME = process.env.DEV_CONSOLE_SERVICE_NAME || "arena-dev-console";
const DEV_CONSOLE_SERVICE_PORT = process.env.DEV_CONSOLE_SERVICE_PORT || "8080";

// Common WebSocket close reasons
const WS_CLOSE_REASON_TIMEOUT = "Connection timeout";
const WS_CLOSE_REASON_UPSTREAM_FAILED = "Upstream connection failed";
const WS_CLOSE_REASON_CONNECTION_CLOSED = "Connection closed";
const WS_CLOSE_REASON_CLIENT_ERROR = "Client connection error";
const WS_CLOSE_CODE_INTERNAL_ERROR = 1011;

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
 * Check if the path is a Dev Console WebSocket request.
 * Expected format: /api/dev-console
 */
function isDevConsolePath(pathname) {
  return pathname === "/api/dev-console";
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
  return `ws://${name}.${namespace}.${SERVICE_DOMAIN}:${facadePort}/ws?agent=${encodeURIComponent(name)}&namespace=${encodeURIComponent(namespace)}`;
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
      clientSocket.close(WS_CLOSE_CODE_INTERNAL_ERROR, WS_CLOSE_REASON_TIMEOUT);
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
        clientSocket.close(sanitizeCloseCode(code), reasonStr || WS_CLOSE_REASON_CONNECTION_CLOSED);
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
        clientSocket.close(WS_CLOSE_CODE_INTERNAL_ERROR, WS_CLOSE_REASON_UPSTREAM_FAILED);
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
        upstream.close(WS_CLOSE_CODE_INTERNAL_ERROR, WS_CLOSE_REASON_CLIENT_ERROR);
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
      clientSocket.close(WS_CLOSE_CODE_INTERNAL_ERROR, WS_CLOSE_REASON_TIMEOUT);
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
        clientSocket.close(sanitizeCloseCode(code), reasonStr || WS_CLOSE_REASON_CONNECTION_CLOSED);
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
        clientSocket.close(WS_CLOSE_CODE_INTERNAL_ERROR, WS_CLOSE_REASON_UPSTREAM_FAILED);
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
        upstream.close(WS_CLOSE_CODE_INTERNAL_ERROR, WS_CLOSE_REASON_CLIENT_ERROR);
      }
    });
  } catch (err) {
    clearTimeout(connectionTimeout);
    console.error(`[WS LSP Proxy] Failed to create upstream connection:`, err.message);
    sendError(clientSocket, `Failed to connect to LSP service: ${err.message}`, "CONNECTION_ERROR");
    clientSocket.close(1011, "Failed to connect to LSP service");
  }
}

/**
 * Build the dev console service URL for a given namespace and service name.
 * Dev console is deployed per-workspace/namespace for security isolation.
 * With dynamic sessions, each session creates its own service (arena-dev-console-{sessionId}).
 * @param {string} namespace - Namespace where the dev console is deployed
 * @param {string} serviceName - Service name (defaults to static DEV_CONSOLE_SERVICE_NAME)
 */
function getDevConsoleUrl(namespace, serviceName) {
  if (!namespace) {
    namespace = "dev-agents"; // Default test namespace
  }
  const svcName = serviceName || DEV_CONSOLE_SERVICE_NAME;
  return `ws://${svcName}.${namespace}.${SERVICE_DOMAIN}:${DEV_CONSOLE_SERVICE_PORT}/ws`;
}

/**
 * Proxy a WebSocket connection to the Arena Dev Console service.
 * The dev console is deployed per-namespace for security isolation.
 * With dynamic sessions (ArenaDevSession), each session creates its own service.
 * @param {WebSocket} clientSocket - Client WebSocket connection
 * @param {string} agentName - Agent identifier for the facade pattern
 * @param {string} workspace - Workspace name (for context)
 * @param {string} namespace - Namespace where the dev console is deployed
 * @param {string} serviceName - Service name (for dynamic sessions like arena-dev-console-{sessionId})
 */
function proxyDevConsoleWebSocket(clientSocket, agentName, workspace, namespace, serviceName) {
  // Build upstream URL - connect to dev console in the workspace's namespace
  const params = new URLSearchParams();
  params.set("agent", agentName || "dev-console");
  if (workspace) params.set("workspace", workspace);

  const baseUrl = getDevConsoleUrl(namespace, serviceName);
  const upstreamUrl = `${baseUrl}?${params.toString()}`;

  console.log(`[WS DevConsole Proxy] Connecting to dev console service '${serviceName || DEV_CONSOLE_SERVICE_NAME}' in namespace '${namespace || "dev-agents"}': ${upstreamUrl}`);

  let upstream = null;
  let upstreamConnected = false;
  let connectionTimeout = null;

  // Set a connection timeout (10 seconds)
  connectionTimeout = setTimeout(() => {
    if (!upstreamConnected) {
      console.error(`[WS DevConsole Proxy] Connection timeout`);
      sendError(clientSocket, `Connection to Dev Console service timed out`, "CONNECTION_TIMEOUT");
      clientSocket.close(WS_CLOSE_CODE_INTERNAL_ERROR, WS_CLOSE_REASON_TIMEOUT);
    }
  }, 10000);

  try {
    upstream = new WebSocket(upstreamUrl);

    upstream.on("open", () => {
      upstreamConnected = true;
      clearTimeout(connectionTimeout);
      console.log(`[WS DevConsole Proxy] Connected to Dev Console service`);
    });

    upstream.on("message", (data, isBinary) => {
      if (clientSocket.readyState === WebSocket.OPEN) {
        clientSocket.send(data, { binary: isBinary });
      }
    });

    upstream.on("close", (code, reason) => {
      clearTimeout(connectionTimeout);
      const reasonStr = reason ? reason.toString() : "";
      console.log(`[WS DevConsole Proxy] Upstream closed: ${code} ${reasonStr}`);
      if (clientSocket.readyState === WebSocket.OPEN) {
        if (!upstreamConnected) {
          sendError(clientSocket, `Dev Console service is not available`, "DEV_CONSOLE_UNAVAILABLE");
        }
        clientSocket.close(sanitizeCloseCode(code), reasonStr || WS_CLOSE_REASON_CONNECTION_CLOSED);
      }
    });

    upstream.on("error", (err) => {
      clearTimeout(connectionTimeout);
      console.error(`[WS DevConsole Proxy] Upstream error:`);
      console.error(`[WS DevConsole Proxy]   message: ${err.message}`);
      console.error(`[WS DevConsole Proxy]   code: ${err.code}`);

      let errorMessage = `Failed to connect to Dev Console service`;
      let errorCode = "CONNECTION_ERROR";

      if (err.code === "ENOTFOUND" || err.code === "EAI_AGAIN") {
        errorMessage = `Dev Console service not found. Check that enterprise features are enabled.`;
        errorCode = "DEV_CONSOLE_NOT_FOUND";
      } else if (err.code === "ECONNREFUSED") {
        errorMessage = `Dev Console service is not accepting connections.`;
        errorCode = "CONNECTION_REFUSED";
      } else if (err.code === "ETIMEDOUT") {
        errorMessage = `Connection to Dev Console service timed out.`;
        errorCode = "CONNECTION_TIMEOUT";
      }

      sendError(clientSocket, errorMessage, errorCode);

      if (clientSocket.readyState === WebSocket.OPEN) {
        clientSocket.close(WS_CLOSE_CODE_INTERNAL_ERROR, WS_CLOSE_REASON_UPSTREAM_FAILED);
      }
    });

    // Forward client messages to upstream
    clientSocket.on("message", (data, isBinary) => {
      if (upstream && upstream.readyState === WebSocket.OPEN) {
        upstream.send(data, { binary: isBinary });
      } else if (!upstreamConnected) {
        console.warn(`[WS DevConsole Proxy] Client sent message before upstream connected`);
      }
    });

    clientSocket.on("close", (code, reason) => {
      clearTimeout(connectionTimeout);
      const reasonStr = reason ? reason.toString() : "";
      console.log(`[WS DevConsole Proxy] Client closed: ${code} ${reasonStr}`);
      if (upstream && upstream.readyState === WebSocket.OPEN) {
        upstream.close(sanitizeCloseCode(code), reasonStr);
      }
    });

    clientSocket.on("error", (err) => {
      clearTimeout(connectionTimeout);
      console.error(`[WS DevConsole Proxy] Client error:`, err.message);
      if (upstream && upstream.readyState === WebSocket.OPEN) {
        upstream.close(WS_CLOSE_CODE_INTERNAL_ERROR, WS_CLOSE_REASON_CLIENT_ERROR);
      }
    });
  } catch (err) {
    clearTimeout(connectionTimeout);
    console.error(`[WS DevConsole Proxy] Failed to create upstream connection:`, err.message);
    sendError(clientSocket, `Failed to connect to Dev Console service: ${err.message}`, "CONNECTION_ERROR");
    clientSocket.close(1011, "Failed to connect to Dev Console service");
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
    } else if (isDevConsolePath(pathname)) {
      // Parse query params for dev console context (workspace/namespace for provider access)
      // service param is used for dynamic sessions (ArenaDevSession creates arena-dev-console-{sessionId})
      const params = parseQueryParams(req.url);
      proxyDevConsoleWebSocket(ws, params.agent || "dev-console", params.workspace, params.namespace, params.service);
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
    } else if (isDevConsolePath(pathname)) {
      console.log(`[WS Upgrade] Dev Console connection request`);
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
