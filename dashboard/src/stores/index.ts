export {
  useConsoleStore,
  useSession,
  useTabs,
  useActiveTab,
  type ConsoleStore,
  type ConsoleTab,
  type SessionState,
} from "./console-store";

export {
  useProjectEditorStore,
  useActiveFile,
  useHasUnsavedChanges,
  useCurrentProject,
  type ProjectEditorStore,
  type ProjectEditorState,
  type ProjectEditorActions,
} from "./project-editor-store";
