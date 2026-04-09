"use client";

import { useParams } from "next/navigation";
import { AlertCircle } from "lucide-react";
import { Header } from "@/components/layout";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useWorkspaceDetail, useWorkspacePatch } from "@/hooks/use-workspace-detail";
import { useToast } from "@/hooks/core";
import { OverviewTab } from "./overview-tab";
import { ServicesTab } from "./services-tab";
import { AccessTab } from "./access-tab";

export default function WorkspaceSettingsPage() {
  const { name } = useParams<{ name: string }>();
  const { data: workspace, isLoading, error } = useWorkspaceDetail(name);
  const { toast } = useToast();
  const { mutate } = useWorkspacePatch(name, {
    onError: (err: Error) => {
      toast({
        title: "Update failed",
        description: err.message,
        variant: "destructive",
      });
    },
  });

  if (isLoading) {
    return (
      <div className="flex flex-col h-full" data-testid="settings-loading">
        <Header title="Workspace Settings" />
        <div className="flex-1 p-6 space-y-4">
          <Skeleton className="h-8 w-64" />
          <Skeleton className="h-48 rounded-lg" />
          <Skeleton className="h-48 rounded-lg" />
        </div>
      </div>
    );
  }

  if (error || !workspace) {
    return (
      <div className="flex flex-col h-full">
        <Header title="Workspace Settings" />
        <div className="flex-1 p-6">
          <Alert variant="destructive">
            <AlertCircle className="h-4 w-4" />
            <AlertTitle>Error</AlertTitle>
            <AlertDescription>
              {error instanceof Error ? error.message : "Failed to load workspace"}
            </AlertDescription>
          </Alert>
        </div>
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full">
      <Header title="Workspace Settings" description={workspace.spec.displayName} />
      <div className="flex-1 p-6">
        <Tabs defaultValue="overview">
          <TabsList>
            <TabsTrigger value="overview">Overview</TabsTrigger>
            <TabsTrigger value="services">Services</TabsTrigger>
            <TabsTrigger value="access">Access</TabsTrigger>
          </TabsList>
          <TabsContent value="overview" className="mt-4">
            <OverviewTab workspace={workspace} />
          </TabsContent>
          <TabsContent value="services" className="mt-4">
            <ServicesTab workspace={workspace} />
          </TabsContent>
          <TabsContent value="access" className="mt-4">
            <AccessTab workspace={workspace} onPatch={mutate} />
          </TabsContent>
        </Tabs>
      </div>
    </div>
  );
}
