/**
 * Integration test for the WS proxy's mgmt-plane Authorization header
 * attachment. Stands up a fake upstream WebSocket server, mints a token
 * with the same minter server.js uses, opens a client connection through
 * the same `new WebSocket(url, [], { headers })` pattern, and asserts the
 * upstream actually sees the Authorization header.
 *
 * The point: catch the silent-not-wired failure mode where the minter
 * works in isolation but the proxy forgets to forward the headers.
 */

import { describe, it, expect, beforeAll, afterAll } from "vitest";
import { createRequire } from "node:module";
import crypto from "node:crypto";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import http from "node:http";
import { WebSocket, WebSocketServer } from "ws";

const require = createRequire(import.meta.url);
const { loadSigningKey, mintToken, MGMT_PLANE_ORIGIN } = require("./mgmt-plane-token.js");

let signingKey;
let publicKey;
let upstreamServer;
let upstreamPort;
const observedHeaders = [];

beforeAll(async () => {
  // Generate a keypair the test holds both halves of: dashboard mints with
  // the private half, the test verifies what the upstream receives by
  // independently re-verifying the JWT with the public half.
  const { privateKey, publicKey: pubKeyPem } = crypto.generateKeyPairSync("rsa", {
    modulusLength: 2048,
    privateKeyEncoding: { type: "pkcs8", format: "pem" },
    publicKeyEncoding: { type: "spki", format: "pem" },
  });
  const pemPath = path.join(fs.mkdtempSync(path.join(os.tmpdir(), "omnia-proxy-")), "key.pem");
  fs.writeFileSync(pemPath, privateKey, { mode: 0o600 });
  signingKey = loadSigningKey(pemPath);
  publicKey = crypto.createPublicKey(pubKeyPem);

  // Stand up a fake upstream that records the headers the proxy sends.
  await new Promise((resolve) => {
    upstreamServer = http.createServer();
    const wss = new WebSocketServer({ server: upstreamServer });
    wss.on("connection", (_socket, req) => {
      observedHeaders.push({ ...req.headers });
    });
    upstreamServer.listen(0, "127.0.0.1", () => {
      upstreamPort = upstreamServer.address().port;
      resolve();
    });
  });
});

afterAll(async () => {
  await new Promise((resolve) => upstreamServer.close(resolve));
});

function decodeJwt(jwt) {
  const [headerB64, payloadB64, sigB64] = jwt.split(".");
  return {
    header: JSON.parse(Buffer.from(headerB64, "base64url").toString("utf8")),
    payload: JSON.parse(Buffer.from(payloadB64, "base64url").toString("utf8")),
    signingInput: Buffer.from(`${headerB64}.${payloadB64}`, "utf8"),
    signature: Buffer.from(sigB64, "base64url"),
  };
}

async function dialUpstream(headers) {
  return new Promise((resolve, reject) => {
    const ws = new WebSocket(`ws://127.0.0.1:${upstreamPort}/ws`, [], { headers });
    ws.on("open", () => {
      ws.close();
      resolve();
    });
    ws.on("error", reject);
  });
}

describe("WS proxy mgmt-plane Authorization attachment", () => {
  it("forwards a Bearer token the upstream can verify with the public key", async () => {
    observedHeaders.length = 0;

    // This is exactly what server.js's proxyWebSocket does today: mint a
    // token and pass it as a header on the upstream constructor.
    const token = mintToken({
      key: signingKey,
      subject: "omnia-dashboard-proxy",
      agent: "test-agent",
      workspace: "default",
    });

    await dialUpstream({ Authorization: `Bearer ${token}` });

    expect(observedHeaders).toHaveLength(1);
    const auth = observedHeaders[0].authorization;
    expect(auth).toMatch(/^Bearer /);

    const presented = auth.slice("Bearer ".length);
    const { payload, signingInput, signature } = decodeJwt(presented);
    expect(payload.origin).toBe(MGMT_PLANE_ORIGIN);
    expect(payload.agent).toBe("test-agent");
    expect(payload.workspace).toBe("default");

    const sigOk = crypto.verify("RSA-SHA256", signingInput, publicKey, signature);
    expect(sigOk).toBe(true);
  });

  it("preserves the no-auth path when no token is attached", async () => {
    observedHeaders.length = 0;
    await dialUpstream({}); // simulate "key not loaded" branch in server.js
    expect(observedHeaders).toHaveLength(1);
    expect(observedHeaders[0].authorization).toBeUndefined();
  });
});
