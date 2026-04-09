import { Badge } from "@/components/ui/badge";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import type { Workspace } from "@/types/workspace";

interface OverviewTabProps {
  workspace: Workspace;
}

function getPhaseBadgeVariant(
  phase: string | undefined
): "default" | "secondary" | "destructive" | "outline" {
  switch (phase) {
    case "Ready":
      return "default";
    case "Suspended":
      return "outline";
    case "Error":
      return "destructive";
    default:
      return "secondary";
  }
}

interface DetailRowProps {
  label: string;
  value: string | number | undefined | null;
}

function DetailRow({ label, value }: DetailRowProps) {
  return (
    <div className="flex items-start py-2 border-b last:border-0">
      <dt className="w-40 shrink-0 text-sm font-medium text-muted-foreground">
        {label}
      </dt>
      <dd className="text-sm">{value ?? "—"}</dd>
    </div>
  );
}

export function OverviewTab({ workspace }: OverviewTabProps) {
  const phase = workspace.status?.phase;
  const spec = workspace.spec;
  const status = workspace.status;
  const meta = workspace.metadata;

  const namespaceName =
    status?.namespace?.name ?? spec.namespace?.name ?? "—";

  const conditions = status?.conditions ?? [];

  return (
    <div className="space-y-6">
      {/* Phase badge */}
      <div className="flex items-center gap-2">
        <span className="text-sm font-medium text-muted-foreground">Phase</span>
        <Badge variant={getPhaseBadgeVariant(phase)}>{phase ?? "Pending"}</Badge>
      </div>

      {/* Details card */}
      <Card>
        <CardHeader>
          <CardTitle>Details</CardTitle>
        </CardHeader>
        <CardContent>
          <dl>
            <DetailRow label="Display Name" value={spec.displayName} />
            <DetailRow label="Description" value={spec.description} />
            <DetailRow label="Environment" value={spec.environment} />
            <DetailRow label="Namespace" value={namespaceName} />
            <DetailRow label="Created" value={meta.creationTimestamp} />
            <DetailRow
              label="Observed Generation"
              value={status?.observedGeneration}
            />
          </dl>
        </CardContent>
      </Card>

      {/* Service Accounts card */}
      {status?.serviceAccounts && (
        <Card>
          <CardHeader>
            <CardTitle>Service Accounts</CardTitle>
          </CardHeader>
          <CardContent>
            <dl>
              <DetailRow label="Owner" value={status.serviceAccounts.owner} />
              <DetailRow label="Editor" value={status.serviceAccounts.editor} />
              <DetailRow label="Viewer" value={status.serviceAccounts.viewer} />
            </dl>
          </CardContent>
        </Card>
      )}

      {/* Conditions card */}
      <Card>
        <CardHeader>
          <CardTitle>Conditions</CardTitle>
        </CardHeader>
        <CardContent>
          {conditions.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              No conditions reported
            </p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Type</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead>Reason</TableHead>
                  <TableHead>Message</TableHead>
                  <TableHead>Last Transition</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {conditions.map((condition) => (
                  <TableRow
                    key={condition.type}
                    className={
                      condition.status === "False" ? "bg-destructive/5" : ""
                    }
                  >
                    <TableCell className="font-medium">
                      {condition.type}
                    </TableCell>
                    <TableCell>
                      <Badge
                        variant={
                          condition.status === "True"
                            ? "default"
                            : condition.status === "False"
                              ? "destructive"
                              : "secondary"
                        }
                      >
                        {condition.status}
                      </Badge>
                    </TableCell>
                    <TableCell>{condition.reason ?? "—"}</TableCell>
                    <TableCell
                      className="max-w-xs truncate"
                      title={condition.message}
                    >
                      {condition.message ?? "—"}
                    </TableCell>
                    <TableCell>{condition.lastTransitionTime ?? "—"}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
