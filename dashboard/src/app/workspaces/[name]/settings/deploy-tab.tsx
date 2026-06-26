"use client";

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import ExportDeployProfile from "@/components/workspace/export-deploy-profile";

export function DeployTab({ workspaceName }: { workspaceName: string }) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>Deploy profile</CardTitle>
        <CardDescription>
          Export a ready-to-paste config block for the promptarena-deploy-omnia
          adapter, pre-filled with this workspace&apos;s providers and skills.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <ExportDeployProfile workspace={workspaceName} />
      </CardContent>
    </Card>
  );
}
