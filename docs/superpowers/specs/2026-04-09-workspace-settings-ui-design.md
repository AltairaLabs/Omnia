# Workspace Settings UI

Admin-only page for viewing workspace status and managing access control.

## Scope

**In scope (v1):**
- Overview tab: read-only workspace status and diagnostics
- Services tab: read-only service group status (session-api, memory-api)
- Access tab: editable anonymous access, role bindings, direct grants

**Deferred:**
- Cost controls editing
- Network policy editing
- Storage configuration
- Switching between managed/external service modes

## Routing & Navigation

- **Route**: `/workspaces/[name]/settings` — dynamic `[name]` param
- **Entry point**: gear icon on the `WorkspaceSwitcher` component, navigates to settings for the currently selected workspace
- **Access control**:
  - API: `withWorkspaceAccess("owner")` on the PATCH route
  - UI: gear icon only rendered when `RequirePermission permission="manageMembers"` passes (owner role)
- **Layout**: standard `Header` component with title "Workspace Settings", workspace display name as description. Three horizontal tabs below: Overview | Services | Access.

## Overview Tab (read-only)

Diagnostic view of workspace health.

### Status Badge
- Workspace phase: Ready (green), Pending (yellow), Error (red), Suspended (gray)

### Details Grid
Key-value pairs:
- Display name
- Description
- Environment (development/staging/production)
- Namespace name
- Created timestamp
- Observed generation

### Service Accounts
List the three SA names: owner, editor, viewer.

### Conditions Table
Collapsible table of K8s conditions:
- Columns: type, status, reason, message, last transition time
- Error conditions (status: "False") highlighted in red
- Makes operator failures immediately visible (e.g., RBAC escalation errors)

## Services Tab (read-only)

Per-workspace service group status.

### Per Service Group (e.g., "default")

**Group header**: name + mode badge (managed/external)

**Session API card**:
- URL with copy button
- Ready status indicator (green/red dot)
- Database secret ref name

**Memory API card**:
- URL with copy button
- Ready status indicator
- Database secret ref name
- Embedding provider ref (if configured)

### Empty States
- Services provisioning: info alert — "Services are being provisioned by the operator. This may take a minute."
- No service groups configured: notice explaining services need to be added to the Workspace CRD.

## Access Tab (editable)

### Anonymous Access
- Toggle switch: enabled/disabled
- Role dropdown (viewer/editor/owner) — shown when enabled
- Warning badge when role is editor or owner
- Changes PATCH immediately (no save button)

### Role Bindings
Table of OIDC/ServiceAccount group-to-role mappings.

- Columns: group name, role, actions (delete)
- "Add binding" button opens an inline row with group name input + role dropdown
- Group field is free-text (OIDC group names or K8s SA groups)

### Direct Grants
Table of individual user grants.

- Columns: user ID, role, expires (optional date), actions (delete)
- "Add grant" button opens an inline row with user ID input, role dropdown, optional date picker

### Update Behavior
- Optimistic updates: UI updates immediately, rolls back on PATCH failure with toast error
- No separate save button — inline edits trigger PATCH immediately

## API

### PATCH /api/workspaces/[name]

- Protected with `withWorkspaceAccess("owner")`
- Accepts partial `WorkspaceSpec` body — only these fields:
  - `anonymousAccess`: `{ enabled: boolean, role?: WorkspaceRole }`
  - `roleBindings`: `RoleBinding[]`
  - `directGrants`: `DirectGrant[]`
- Uses K8s client to merge-patch the Workspace CRD
- Returns updated workspace object

### GET /api/workspaces/[name] (existing)

Already exists — returns full workspace spec + status. Used by all three tabs.

## File Structure

```
dashboard/src/app/workspaces/[name]/settings/
  page.tsx              — main settings page with tabs
  overview-tab.tsx      — Overview tab component
  services-tab.tsx      — Services tab component
  access-tab.tsx        — Access tab component

dashboard/src/app/api/workspaces/[name]/
  route.ts              — add PATCH handler to existing route

dashboard/src/hooks/
  use-workspace-settings.ts  — React Query hook for workspace detail + mutations

dashboard/src/components/workspace-switcher.tsx
  — add gear icon linking to settings
```

## Patterns

- Follows existing dashboard patterns: `Header`, `Tabs`, `Alert`, `Badge`, `Table` from shadcn/ui
- Permission gating via `RequirePermission` and `useWorkspacePermissions`
- API protection via `withWorkspaceAccess`
- React Query for data fetching and mutations with optimistic updates
- TypeScript types from `dashboard/src/types/workspace.ts` (hand-maintained)
