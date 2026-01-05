// Notes storage using localStorage (MVP)
// Future: migrate to K8s annotations on resources

const STORAGE_KEY = "omnia-topology-notes";

export interface ResourceNote {
  resourceType: "agent" | "promptpack" | "toolregistry";
  namespace: string;
  name: string;
  note: string;
  updatedAt: string;
}

export type NotesMap = Record<string, ResourceNote>;

function getResourceKey(type: string, namespace: string, name: string): string {
  return `${type}/${namespace}/${name}`;
}

export function loadNotes(): NotesMap {
  if (typeof window === "undefined") return {};
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    return stored ? JSON.parse(stored) : {};
  } catch {
    return {};
  }
}

export function saveNotes(notes: NotesMap): void {
  if (typeof window === "undefined") return;
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(notes));
  } catch (e) {
    console.error("Failed to save notes:", e);
  }
}

export function getNote(
  type: string,
  namespace: string,
  name: string
): ResourceNote | undefined {
  const notes = loadNotes();
  return notes[getResourceKey(type, namespace, name)];
}

export type ResourceType = "agent" | "promptpack" | "toolregistry";

export function setNote(
  type: string,
  namespace: string,
  name: string,
  note: string
): void {
  const notes = loadNotes();
  const key = getResourceKey(type, namespace, name);

  if (note.trim() === "") {
    delete notes[key];
  } else {
    notes[key] = {
      resourceType: type as ResourceType,
      namespace,
      name,
      note: note.trim(),
      updatedAt: new Date().toISOString(),
    };
  }

  saveNotes(notes);
}

export function deleteNote(type: string, namespace: string, name: string): void {
  const notes = loadNotes();
  delete notes[getResourceKey(type, namespace, name)];
  saveNotes(notes);
}

export function getNotesForNamespaces(namespaces: string[]): ResourceNote[] {
  const notes = loadNotes();
  return Object.values(notes).filter((note) =>
    namespaces.length === 0 || namespaces.includes(note.namespace)
  );
}
