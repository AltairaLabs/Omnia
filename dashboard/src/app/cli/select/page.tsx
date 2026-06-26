/**
 * CLI browser-login workspace picker page.
 *
 * GET /cli/select?flow=<flowId> — rendered after OIDC login. Lists the
 * workspaces the user can deploy to (editor+) and posts the choice to
 * /api/cli/grant. Reload-safe: reads (does not consume) the flow record.
 */
import { redirect } from "next/navigation";
import { getUser } from "@/lib/auth";
import { getSessionStore } from "@/lib/auth/session-store";
import { getAccessibleWorkspaces } from "@/lib/auth/workspace-authz";
import { CliWorkspacePicker } from "./cli-workspace-picker";

function ExpiredState() {
  return (
    <main className="mx-auto max-w-md p-8">
      <h1 className="text-lg font-semibold">Login expired</h1>
      <p className="mt-4 text-sm text-muted-foreground">
        This CLI login link is no longer valid. Run <code>deploy login</code> again.
      </p>
    </main>
  );
}

export default async function CliSelectPage({
  searchParams,
}: {
  searchParams: Promise<{ flow?: string }>;
}) {
  const { flow } = await searchParams;

  const user = await getUser();
  if (user.provider === "anonymous") {
    const selectPath = `/cli/select?flow=${encodeURIComponent(flow ?? "")}`;
    redirect(`/api/auth/login?returnTo=${encodeURIComponent(selectPath)}`);
  }

  if (!flow) return <ExpiredState />;
  const record = await getSessionStore().getCliFlow(flow);
  if (!record) return <ExpiredState />;

  const accessible = await getAccessibleWorkspaces("editor");
  const workspaces = accessible.map((a) => ({
    name: a.workspace.metadata.name,
    role: a.access.role ?? "editor",
  }));

  return <CliWorkspacePicker flow={flow} workspaces={workspaces} />;
}
