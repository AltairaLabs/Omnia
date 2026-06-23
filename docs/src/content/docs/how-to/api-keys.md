---
title: "Durable API keys"
description: "Choose and configure a persistent API-key store for the Omnia dashboard."
sidebar:
  order: 35
---

API keys authenticate programmatic clients (CI, the deploy adapter, scripts). The dashboard supports three stores via `dashboard.apiKeys.store`:

- **`memory`** (default) — in-process; keys are lost on every dashboard restart. Dev only.
- **`postgres`** — durable; keys (bcrypt-hashed) live in Postgres. Set `dashboard.apiKeys.postgresUrl` or `dashboard.apiKeys.existingPostgresSecret` (falls back to the builtin store's Postgres URL when running `auth.mode=builtin`). UI create/revoke works.
- **`file`** — read-only; a Kubernetes Secret of bcrypt-hashed keys mounted at `dashboard.apiKeys.filePath`. GitOps-friendly; UI creation is disabled. Set `dashboard.apiKeys.keysSecret`.

Only bcrypt hashes are ever stored; the full key is shown once at creation.

### Generating file-store keys

The file store reads `keys.json` (`{ "keys": [ { id, userId, name, keyPrefix, keyHash, role, expiresAt, createdAt } ] }`); `keyHash` is a bcrypt hash of the full `omnia_sk_…` key.
