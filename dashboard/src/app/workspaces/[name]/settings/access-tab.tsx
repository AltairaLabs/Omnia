"use client";

import { useState } from "react";
import { AlertTriangle, Plus, Trash2 } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import type { Workspace, WorkspaceRole, WorkspaceSpec } from "@/types/workspace";

const WORKSPACE_ROLES: WorkspaceRole[] = ["viewer", "editor", "owner"];
const ELEVATED_ROLES: WorkspaceRole[] = ["editor", "owner"];
const DEFAULT_ROLE: WorkspaceRole = "viewer";

function formatExpiry(expires?: string): string {
  if (!expires) return "Never";
  return new Date(expires).toLocaleDateString();
}

interface AccessTabProps {
  workspace: Workspace;
  onPatch: (updates: Partial<WorkspaceSpec>) => void;
}

function RoleSelect({
  value,
  onValueChange,
}: {
  value: WorkspaceRole;
  onValueChange: (role: WorkspaceRole) => void;
}) {
  return (
    <Select value={value} onValueChange={onValueChange}>
      <SelectTrigger className="w-32">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {WORKSPACE_ROLES.map((r) => (
          <SelectItem key={r} value={r}>
            {r}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

function AnonymousAccessCard({
  workspace,
  onPatch,
}: AccessTabProps) {
  const anonymousAccess = workspace.spec.anonymousAccess;
  const enabled = anonymousAccess?.enabled ?? false;
  const role = anonymousAccess?.role ?? DEFAULT_ROLE;
  const isElevated = ELEVATED_ROLES.includes(role);

  function handleToggle() {
    onPatch({ anonymousAccess: { enabled: !enabled } });
  }

  function handleRoleChange(newRole: WorkspaceRole) {
    onPatch({ anonymousAccess: { ...anonymousAccess, enabled, role: newRole } });
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Anonymous Access</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="flex items-center gap-3">
          <Switch checked={enabled} onCheckedChange={handleToggle} />
          <span className="text-sm">
            {enabled ? "Enabled" : "Disabled"}
          </span>
        </div>
        {enabled && (
          <div className="flex items-center gap-3">
            <RoleSelect value={role} onValueChange={handleRoleChange} />
            {isElevated && (
              <Badge variant="destructive" className="flex items-center gap-1">
                <AlertTriangle className="size-3" />
                Elevated access
              </Badge>
            )}
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function RoleBindingsCard({ workspace, onPatch }: AccessTabProps) {
  const [newGroup, setNewGroup] = useState("");
  const [newRole, setNewRole] = useState<WorkspaceRole>(DEFAULT_ROLE);
  const bindings = workspace.spec.roleBindings ?? [];

  function handleDelete(index: number) {
    onPatch({ roleBindings: bindings.filter((_, i) => i !== index) });
  }

  function handleAdd() {
    if (!newGroup.trim()) return;
    onPatch({
      roleBindings: [
        ...bindings,
        { groups: [newGroup.trim()], role: newRole },
      ],
    });
    setNewGroup("");
    setNewRole(DEFAULT_ROLE);
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Role Bindings</CardTitle>
      </CardHeader>
      <CardContent>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Group</TableHead>
              <TableHead>Role</TableHead>
              <TableHead>Actions</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {bindings.map((binding, index) => (
              <TableRow key={`${binding.groups?.[0]}-${binding.role}`}>
                <TableCell>{binding.groups?.[0] ?? "—"}</TableCell>
                <TableCell>
                  <Badge variant="outline">{binding.role}</Badge>
                </TableCell>
                <TableCell>
                  <Button
                    variant="ghost"
                    size="icon"
                    data-testid="delete-binding"
                    onClick={() => handleDelete(index)}
                  >
                    <Trash2 className="size-4" />
                  </Button>
                </TableCell>
              </TableRow>
            ))}
            <TableRow>
              <TableCell>
                <Input
                  placeholder="Group name"
                  value={newGroup}
                  onChange={(e) => setNewGroup(e.target.value)}
                />
              </TableCell>
              <TableCell>
                <RoleSelect value={newRole} onValueChange={setNewRole} />
              </TableCell>
              <TableCell>
                <Button
                  variant="ghost"
                  size="icon"
                  onClick={handleAdd}
                  disabled={!newGroup.trim()}
                >
                  <Plus className="size-4" />
                </Button>
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  );
}

function DirectGrantsCard({ workspace, onPatch }: AccessTabProps) {
  const [newUser, setNewUser] = useState("");
  const [newRole, setNewRole] = useState<WorkspaceRole>(DEFAULT_ROLE);
  const grants = workspace.spec.directGrants ?? [];

  function handleDelete(index: number) {
    onPatch({ directGrants: grants.filter((_, i) => i !== index) });
  }

  function handleAdd() {
    if (!newUser.trim()) return;
    onPatch({
      directGrants: [
        ...grants,
        { user: newUser.trim(), role: newRole },
      ],
    });
    setNewUser("");
    setNewRole(DEFAULT_ROLE);
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Direct Grants</CardTitle>
      </CardHeader>
      <CardContent>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>User</TableHead>
              <TableHead>Role</TableHead>
              <TableHead>Expires</TableHead>
              <TableHead>Actions</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {grants.map((grant, index) => (
              <TableRow key={`${grant.user}-${grant.role}`}>
                <TableCell>{grant.user}</TableCell>
                <TableCell>
                  <Badge variant="outline">{grant.role}</Badge>
                </TableCell>
                <TableCell>{formatExpiry(grant.expires)}</TableCell>
                <TableCell>
                  <Button
                    variant="ghost"
                    size="icon"
                    data-testid="delete-grant"
                    onClick={() => handleDelete(index)}
                  >
                    <Trash2 className="size-4" />
                  </Button>
                </TableCell>
              </TableRow>
            ))}
            <TableRow>
              <TableCell>
                <Input
                  placeholder="User email"
                  value={newUser}
                  onChange={(e) => setNewUser(e.target.value)}
                />
              </TableCell>
              <TableCell>
                <RoleSelect value={newRole} onValueChange={setNewRole} />
              </TableCell>
              <TableCell />
              <TableCell>
                <Button
                  variant="ghost"
                  size="icon"
                  onClick={handleAdd}
                  disabled={!newUser.trim()}
                >
                  <Plus className="size-4" />
                </Button>
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  );
}

export function AccessTab({ workspace, onPatch }: AccessTabProps) {
  return (
    <div className="space-y-6">
      <AnonymousAccessCard workspace={workspace} onPatch={onPatch} />
      <RoleBindingsCard workspace={workspace} onPatch={onPatch} />
      <DirectGrantsCard workspace={workspace} onPatch={onPatch} />
    </div>
  );
}
