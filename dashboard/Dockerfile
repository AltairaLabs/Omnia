# syntax=docker/dockerfile:1

# Build stage
FROM node:20-alpine AS builder

WORKDIR /app

# better-sqlite3 ships prebuilt binaries for x64-musl but not arm64-musl,
# so on arm64 hosts npm falls back to source compilation via node-gyp.
# Install Python + a C++ toolchain so the native build succeeds. The
# runtime stage stays slim — it only copies the resolved node_modules.
RUN apk add --no-cache python3 make g++

# Install dependencies first (for better caching)
COPY package.json package-lock.json ./
RUN --mount=type=cache,target=/root/.npm \
    npm ci

# Copy source and build
COPY . .
RUN --mount=type=cache,target=/app/.next/cache \
    npm run build

# Production stage
FROM node:20-alpine AS runner

WORKDIR /app

ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1

# Create non-root user
RUN addgroup --system --gid 1001 nodejs && \
    adduser --system --uid 1001 nextjs

# Copy production dependencies. Reuse the builder's already-compiled
# node_modules so we don't need build tools (python3/make/g++) in the
# slim runner image — `npm prune --omit=dev` strips devDependencies in
# place without triggering a native rebuild.
COPY --from=builder /app/package.json ./
COPY --from=builder /app/package-lock.json ./
COPY --from=builder /app/node_modules ./node_modules
RUN npm prune --omit=dev && \
    # Remove the bundled npm CLI once /app/node_modules is pruned.
    # `node server.js` is the only entrypoint at runtime; the CLI's own
    # transitive deps (tar, minimatch, glob, cross-spawn, etc.) are
    # unused attack surface and trip Trivy scans on every release.
    rm -rf /usr/local/lib/node_modules/npm \
           /usr/local/bin/npm \
           /usr/local/bin/npx

# Copy build output
COPY --from=builder /app/public ./public
COPY --from=builder --chown=nextjs:nodejs /app/.next ./.next
COPY --from=builder /app/server.js ./server.js
# server.js requires modules from /app/lib at runtime (e.g. auth-boot-guard)
COPY --from=builder /app/lib ./lib

USER nextjs

EXPOSE 3000

ENV PORT=3000
ENV HOSTNAME="0.0.0.0"
# K8s service DNS domain for agent connections
ENV SERVICE_DOMAIN="svc.cluster.local"

CMD ["node", "server.js"]
