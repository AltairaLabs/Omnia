/** Workspace picker for the CLI browser-login handoff. */

export interface CliPickerWorkspace {
  name: string;
  role: string;
}

export function CliWorkspacePicker({
  flow,
  workspaces,
}: {
  flow: string;
  workspaces: CliPickerWorkspace[];
}) {
  if (workspaces.length === 0) {
    return (
      <main className="mx-auto max-w-md p-8">
        <h1 className="text-lg font-semibold">Authorize CLI</h1>
        <p className="mt-4 text-sm text-muted-foreground">
          You have no workspaces you can deploy to. Ask a workspace owner for
          editor access, then run <code>deploy login</code> again.
        </p>
      </main>
    );
  }

  return (
    <main className="mx-auto max-w-md p-8">
      <h1 className="text-lg font-semibold">Choose a workspace to deploy to</h1>
      <form action="/api/cli/grant" method="post" className="mt-6 space-y-3">
        <input type="hidden" name="flow" value={flow} />
        {workspaces.map((w, i) => (
          <label key={w.name} className="flex items-center gap-2">
            <input
              type="radio"
              name="workspace"
              value={w.name}
              defaultChecked={i === 0}
              required
            />
            <span>{w.name}</span>
            <span className="text-xs text-muted-foreground">[{w.role}]</span>
          </label>
        ))}
        <button type="submit" className="mt-4 rounded bg-primary px-4 py-2 text-primary-foreground">
          Authorize
        </button>
      </form>
    </main>
  );
}
