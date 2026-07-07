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

## Scope a key to workspaces

By default a key carries the minting user's full access — it can act in **every** workspace that user can reach. You can instead confine a key to a specific set of workspaces (a per-key allowlist), so a leaked or misused credential is limited to that blast radius.

### Create a scoped key

When creating a key in the dashboard (**Settings → API keys**), the create dialog shows a **Restrict to workspaces** multi-select listing the workspaces you can access. Check one or more to build the key's allowlist. Leaving all unchecked produces an unrestricted key that can act in every workspace you can access.

The API accepts the same allowlist directly — `POST /api/settings/api-keys` with a `workspaces` array:

```json
{
  "name": "ci-prod-only",
  "workspaces": ["prod-workspace"]
}
```

At mint time the key also snapshots the owner's identity (email/username) and group membership, so it resolves the owner's per-workspace role later without a live session.

### How the allowlist is enforced

The allowlist is enforced on **both** the access and listing paths:

- **Access** — a request made with a scoped key for a workspace outside its allowlist is denied up front, before any role is computed.
- **Listing** — a scoped key only ever enumerates workspaces in its allowlist; workspaces outside it are skipped entirely.

A scoped key never receives the platform-admin shortcut (which would span every workspace), even if its owner is a platform admin — that would defeat least-privilege. Within an allowed workspace the key still acts with the owner's own role; the allowlist narrows *which* workspaces the key reaches, not *what* it can do inside them. An empty or omitted allowlist means unrestricted.

### Deploy-profile tokens are auto-scoped

When you export a workspace's deploy profile (**Export deploy profile**), the dashboard mints a fresh `omnia_sk_…` token pre-scoped to just that workspace (`workspaces: [<workspace>]`). The downloadable credential can therefore only deploy to the workspace it was exported for — least privilege by default.
