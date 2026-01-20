"use client";

import { useState, useCallback } from "react";
import useSWR from "swr";
import { Trash2, Server, Clock, RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
import { useLicense } from "@/hooks/use-license";
import type { ClusterActivation } from "@/app/api/license/activations/route";

interface ActivationsResponse {
  activations: ClusterActivation[];
}

async function fetcher(url: string): Promise<ActivationsResponse> {
  const response = await fetch(url);
  if (!response.ok) {
    throw new Error("Failed to fetch activations");
  }
  return response.json();
}

function formatDate(dateString: string): string {
  return new Date(dateString).toLocaleDateString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
  });
}

function formatRelativeTime(dateString: string): string {
  const date = new Date(dateString);
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffMins = Math.floor(diffMs / (1000 * 60));
  const diffHours = Math.floor(diffMs / (1000 * 60 * 60));
  const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24));

  if (diffMins < 1) return "just now";
  if (diffMins < 60) return `${diffMins} minute${diffMins === 1 ? "" : "s"} ago`;
  if (diffHours < 24) return `${diffHours} hour${diffHours === 1 ? "" : "s"} ago`;
  return `${diffDays} day${diffDays === 1 ? "" : "s"} ago`;
}

interface DeactivateButtonProps {
  fingerprint: string;
  clusterName: string;
  onDeactivate: () => void;
}

function DeactivateButton({ fingerprint, clusterName, onDeactivate }: DeactivateButtonProps) {
  const [isLoading, setIsLoading] = useState(false);

  const handleDeactivate = useCallback(async () => {
    setIsLoading(true);
    try {
      const response = await fetch(`/api/license/activations/${fingerprint}`, {
        method: "DELETE",
      });

      if (!response.ok) {
        throw new Error("Failed to deactivate");
      }

      onDeactivate();
    } catch {
      // Error handling will be improved in a future iteration
    } finally {
      setIsLoading(false);
    }
  }, [fingerprint, onDeactivate]);

  return (
    <AlertDialog>
      <AlertDialogTrigger asChild>
        <Button variant="ghost" size="icon" className="h-8 w-8">
          <Trash2 className="h-4 w-4 text-muted-foreground hover:text-destructive" />
        </Button>
      </AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Deactivate Cluster</AlertDialogTitle>
          <AlertDialogDescription>
            Are you sure you want to deactivate <strong>{clusterName}</strong>? This will
            free up a license slot but the cluster will need to be reactivated to use
            enterprise features.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction onClick={handleDeactivate} disabled={isLoading}>
            {isLoading ? "Deactivating..." : "Deactivate"}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

/**
 * Cluster activation management section for settings page.
 */
export function ActivationSection() {
  const { isEnterprise } = useLicense();
  const { data, isLoading, mutate } = useSWR<ActivationsResponse>(
    isEnterprise ? "/api/license/activations" : null,
    fetcher
  );

  const activations = data?.activations ?? [];

  // Don't show for non-enterprise
  if (!isEnterprise) {
    return null;
  }

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Server className="h-5 w-5 text-primary" />
            <CardTitle>Cluster Activations</CardTitle>
          </div>
          <Button
            variant="outline"
            size="sm"
            onClick={() => mutate()}
            disabled={isLoading}
          >
            <RefreshCw className={`h-4 w-4 mr-2 ${isLoading ? "animate-spin" : ""}`} />
            Refresh
          </Button>
        </div>
        <CardDescription>
          Manage clusters activated with your enterprise license
        </CardDescription>
      </CardHeader>
      <CardContent>
        {activations.length === 0 ? (
          <div className="text-center py-8 text-muted-foreground">
            <Server className="h-8 w-8 mx-auto mb-2 opacity-50" />
            <p>No clusters activated yet</p>
          </div>
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Cluster Name</TableHead>
                <TableHead>Activated</TableHead>
                <TableHead>Last Seen</TableHead>
                <TableHead className="w-[50px]"></TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {activations.map((activation) => (
                <TableRow key={activation.fingerprint}>
                  <TableCell className="font-medium">
                    {activation.clusterName}
                  </TableCell>
                  <TableCell>{formatDate(activation.activatedAt)}</TableCell>
                  <TableCell>
                    <div className="flex items-center gap-1 text-muted-foreground">
                      <Clock className="h-3 w-3" />
                      {formatRelativeTime(activation.lastSeen)}
                    </div>
                  </TableCell>
                  <TableCell>
                    <DeactivateButton
                      fingerprint={activation.fingerprint}
                      clusterName={activation.clusterName}
                      onDeactivate={() => mutate()}
                    />
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </CardContent>
    </Card>
  );
}
