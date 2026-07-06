---
title: "Manage User Consent and Opt-Out"
description: "Grant and revoke a user's consent categories, set opt-out preferences, and read consent and enforcement stats through the per-workspace privacy-api"
sidebar:
  order: 15.5
  badge:
    text: Enterprise
    variant: tip
---

:::note[Enterprise Feature]
User consent and opt-out management is an Enterprise feature owned by the
per-workspace **privacy-api** service. It requires an active Enterprise
license — see [Install an Enterprise License](/how-to/operations/install-license/).
:::

This guide shows how to manage a single user's privacy preferences against the
per-workspace privacy-api: granting and revoking granular **consent categories**,
setting scope-based **opt-out** preferences, and reading aggregate **consent** and
**enforcement** stats.

privacy-api owns per-user consent grants and opt-out preferences (each workspace
has its own consent database). Memory and PII-redaction enforcement in memory-api
and session-api reads this state to decide what may be stored — so these are the
knobs that drive per-user enforcement.

## Prerequisites

- A Workspace with [`spec.privacy`](/reference/core/workspace/#privacy) set. This is
  the trigger that provisions the privacy-api; without it there is no consent
  service to call.
- The user's **pseudonymized** identifier (`virtual_user_id` — the same
  `PseudonymizeID` value the facade writes for sessions and memory, never a raw
  email or user id). Consent, opt-out, and session data are all keyed by this
  pseudonym, so a raw id matches nothing.
- Network access to privacy-api. Its JSON API is authenticated with a Kubernetes
  ServiceAccount token and is not exposed externally by default, so call it from
  inside the cluster with a valid token.

```bash
# Forward the workspace's privacy-api locally (service name is privacy-<workspace>).
kubectl port-forward -n <workspace-namespace> svc/privacy-<workspace> 8080:8080
```

All examples below assume `$SA_TOKEN` holds a valid ServiceAccount bearer token
and the service is reachable at `http://localhost:8080`.

## Consent categories

Consent is granted per **category**. The platform defines a fixed set of
categories; two tiers exist:

| Category | Requires explicit grant? |
|----------|--------------------------|
| `memory:preferences` | No — granted by default |
| `memory:context` | No — granted by default |
| `memory:history` | No — granted by default |
| `memory:identity` | Yes |
| `memory:location` | Yes |
| `memory:health` | Yes |
| `analytics:aggregate` | Yes |

Categories that do not require an explicit grant are treated as consented unless
the user opts out; the sensitive categories (`memory:identity`, `memory:location`,
`memory:health`, `analytics:aggregate`) are **denied until explicitly granted**.

:::caution[Only registered categories are accepted]
Consent writes naming an **unregistered** consent category are rejected with
`400 Bad Request` (`unknown consent category: …`) (#1662). You cannot invent
ad-hoc categories; use one of the values in the table above.
:::

## Grant or revoke consent

`PUT /api/v1/privacy/preferences/{userID}/consent` applies grants and/or
revocations in one call. The body has two lists — `grants` and `revocations`:

```bash
curl -sS -X PUT \
  http://localhost:8080/api/v1/privacy/preferences/<pseudonymized-user-id>/consent \
  -H "Authorization: Bearer $SA_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
        "grants": ["memory:identity", "memory:location"],
        "revocations": ["analytics:aggregate"]
      }'
```

The response is the user's resulting consent state:

```json
{
  "grants": ["memory:identity", "memory:location"],
  "defaults": ["memory:preferences", "memory:context", "memory:history"],
  "denied": ["memory:health", "analytics:aggregate"]
}
```

- `grants` — categories the user has explicitly granted.
- `defaults` — categories consented by default (no explicit grant needed).
- `denied` — sensitive categories not currently granted.

Revoking a category that was never granted is a silent no-op. Revocations are
also fanned out to downstream memory-api instances so already-stored memories in a
revoked category can be enforced. Each grant and revocation is written to the
privacy audit log (`consent_granted` / `consent_revoked`).

## Read a user's consent state

`GET /api/v1/privacy/preferences/{userID}/consent` returns the same
`grants` / `defaults` / `denied` shape without mutating anything:

```bash
curl -sS \
  http://localhost:8080/api/v1/privacy/preferences/<pseudonymized-user-id>/consent \
  -H "Authorization: Bearer $SA_TOKEN"
```

## Set an opt-out preference

Opt-out is separate from consent: it suppresses recording/enforcement at a
**scope** rather than per category. `POST /api/v1/privacy/opt-out` sets one:

```bash
curl -sS -X POST http://localhost:8080/api/v1/privacy/opt-out \
  -H "Authorization: Bearer $SA_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
        "userId": "<pseudonymized-user-id>",
        "scope": "workspace",
        "target": "my-workspace"
      }'
```

Fields:

| Field | Required | Values / notes |
|-------|----------|----------------|
| `userId` | yes | The pseudonymized subject id. |
| `scope` | yes | `all`, `workspace`, or `agent`. |
| `target` | for `workspace` / `agent` | The workspace or agent name. Not needed for `all`. |

A successful set returns `204 No Content`.

Remove an opt-out with the same body and `DELETE /api/v1/privacy/opt-out`:

```bash
curl -sS -X DELETE http://localhost:8080/api/v1/privacy/opt-out \
  -H "Authorization: Bearer $SA_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"userId": "<pseudonymized-user-id>", "scope": "workspace", "target": "my-workspace"}'
```

Fetch a user's current opt-out preferences with
`GET /api/v1/privacy/preferences/{userID}`:

```bash
curl -sS http://localhost:8080/api/v1/privacy/preferences/<pseudonymized-user-id> \
  -H "Authorization: Bearer $SA_TOKEN"
```

A `404 Not Found` means the user has no stored preferences yet.

## Read enforcement and consent stats

Both stats endpoints are **workspace-scoped** and require a `workspace` query
parameter.

Enforcement stats — how much privacy enforcement has actually happened (derived
from the central audit hub):

```bash
curl -sS "http://localhost:8080/api/v1/privacy/enforcement-stats?workspace=my-workspace" \
  -H "Authorization: Bearer $SA_TOKEN"
```

```json
{
  "piiBlocked": 42,
  "redactions": 128
}
```

`piiBlocked` counts opt-out write blocks; `redactions` counts PII redaction
events. This is the same read path the dashboard's enforcement-stats view uses.

Aggregate consent stats across users in the workspace:

```bash
curl -sS "http://localhost:8080/api/v1/privacy/consent/stats?workspace=my-workspace" \
  -H "Authorization: Bearer $SA_TOKEN"
```

## Related

- [Handle Data Subject Erasure (DSAR)](/how-to/privacy/handle-data-subject-erasure/) — erase all of a user's data (right-to-erasure).
- [Configure Privacy Policies](/how-to/privacy/configure-privacy-policies/) — SessionPrivacyPolicy recording, PII redaction, and encryption rules.
- [SessionPrivacyPolicy CRD](/reference/policies/sessionprivacypolicy/) — full policy field reference.
- [Workspace CRD](/reference/core/workspace/#privacy) — enabling `spec.privacy` to provision the privacy-api.
