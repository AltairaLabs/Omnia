"use client";

import { useSearchParams } from "next/navigation";
import { AlertCircle } from "lucide-react";
import { Header } from "@/components/layout";
import { Skeleton } from "@/components/ui/skeleton";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { useWorkspace } from "@/contexts/workspace-context";
import { ServiceHealthPanel } from "@/components/services/service-health-panel";

/** Query-string key used for deep-linking into a specific service group. */
const GROUP_QUERY_PARAM = "group";

export default function ServicesPage() {
  const searchParams = useSearchParams();
  const initialExpandedGroup = searchParams.get(GROUP_QUERY_PARAM) ?? undefined;
  const { currentWorkspace, isLoading } = useWorkspace();

  return (
    <div className="flex h-full flex-col">
      <Header
        title="Services"
        description="Per-service health for this workspace, grouped by service group"
      />

      <div className="flex-1 space-y-6 p-6">
        {isLoading && <Skeleton className="h-[400px] rounded-lg" />}

        {!isLoading && !currentWorkspace && (
          <Alert data-testid="no-workspace-notice">
            <AlertCircle className="h-4 w-4" />
            <AlertTitle>No workspace selected</AlertTitle>
            <AlertDescription>
              Select a workspace to view its service health.
            </AlertDescription>
          </Alert>
        )}

        {!isLoading && currentWorkspace && (
          <ServiceHealthPanel
            workspaceName={currentWorkspace.name}
            initialExpandedGroup={initialExpandedGroup}
          />
        )}
      </div>
    </div>
  );
}
