---
title: "Handle data subject erasure (DSAR)"
description: "Submit and track right-to-erasure (GDPR/CCPA) requests that delete a user's sessions, media, and memories across a workspace"
sidebar:
  order: 16
---

This guide shows how to erase all data for a single user — a Data Subject Access Request (DSAR) / right-to-erasure — and how to track the request to completion.

Erasure is an Enterprise feature owned by the per-workspace **privacy-api** service. You need an active Enterprise license and a workspace with `spec.privacy` enabled.

## What gets erased

A deletion request is keyed by a user's **pseudonymized** identifier (`virtual_user_id` — the same `PseudonymizeID` value the facade writes for sessions and memory, never a raw email or user id). privacy-api owns the request lifecycle and fans the erasure out across **every service group** in the workspace:

- **Sessions and their media** — deleted by each service group's session-api (`delete-by-user`).
- **Memories** — deleted by each service group's memory-api (batch delete, scoped to the workspace).

privacy-api holds no session or memory credentials itself; each service erases its own tier. Every request and its outcome are recorded in the workspace's `deletion_requests` table, and lifecycle events (`deletion_requested`, `deletion_completed`, `deletion_failed`) are written to the central privacy audit log.

## Prerequisites

- Omnia operator with Enterprise enabled and `spec.privacy` set on the Workspace
- The user's pseudonymized `virtual_user_id`
- Network access to privacy-api. Its JSON API is authenticated with a Kubernetes ServiceAccount token and is not exposed externally by default, so call it from inside the cluster (for example, `kubectl port-forward`) with a valid token.

```bash
# Forward the workspace's privacy-api locally (service name is privacy-<workspace>).
kubectl port-forward -n <workspace-namespace> svc/privacy-<workspace> 8087:8080
```

## Step 1: submit the deletion request

`POST /api/v1/privacy/deletion-request`. The call returns `202 Accepted` immediately and processing continues asynchronously.

```bash
curl -sS -X POST http://localhost:8087/api/v1/privacy/deletion-request \
  -H "Authorization: Bearer $SA_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
        "virtualUserId": "<pseudonymized-user-id>",
        "reason": "gdpr_erasure",
        "scope": "all"
      }'
```

Fields:

| Field | Required | Values / notes |
|-------|----------|----------------|
| `virtualUserId` | yes | The pseudonymized subject id. An empty value is rejected (the request never deletes all users). |
| `reason` | yes | `gdpr_erasure`, `ccpa_delete`, or `user_request`. |
| `scope` | no (default `all`) | `all`, `workspace`, or `date_range`. |
| `workspace` | no | Restrict to a named workspace when `scope` is `workspace`. |
| `dateFrom` / `dateTo` | for `date_range` | RFC 3339 timestamps; at least one is required for `date_range`. |

The response body is the created request, including its `id` and `status: "pending"`.

## Step 2: track the request to completion

`GET /api/v1/privacy/deletion-request/{id}`.

```bash
curl -sS http://localhost:8087/api/v1/privacy/deletion-request/<id> \
  -H "Authorization: Bearer $SA_TOKEN"
```

The `status` field moves through `pending` → `in_progress` → `completed` (or `failed`). The response also reports `sessionsDeleted` and any per-target `errors`:

```json
{
  "id": "…",
  "virtualUserId": "…",
  "status": "completed",
  "sessionsDeleted": 12,
  "errors": []
}
```

A request is marked `failed` if any tier or service group reported an error; the `errors` array names the failing group and tier so you can retry or investigate. The successfully erased data is still gone — erasure is best-effort per target and does not roll back.

## Step 3: list a user's requests (optional)

`GET /api/v1/privacy/deletion-requests?virtual_user_id=<id>` returns every request submitted for a subject, newest first — useful for audit or to confirm a prior erasure.

```bash
curl -sS "http://localhost:8087/api/v1/privacy/deletion-requests?virtual_user_id=<id>" \
  -H "Authorization: Bearer $SA_TOKEN"
```

## Notes

- **Idempotency of identity:** the `virtualUserId` must be the pseudonym, not a raw identifier. Sessions, memories, and deletion requests are all keyed by the same pseudonym, so passing a raw id matches nothing.
- **Multi-group workspaces:** erasure covers every service group in the workspace automatically — you submit one request, not one per group.
- **Audit:** deletion lifecycle events are queryable from the central privacy audit log alongside other privacy/compliance events.
