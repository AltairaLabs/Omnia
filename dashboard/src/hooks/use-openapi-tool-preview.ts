import { useEffect, useReducer } from "react";
import type { OpenAPIToolPreview, OpenAPIToolPreviewItem } from "@/types/tool-registry";

interface PreviewState {
  tools: OpenAPIToolPreviewItem[];
  specURL: string | null;
  error: string | null;
  loading: boolean;
}

type Action =
  | { type: "loading" }
  | { type: "success"; payload: OpenAPIToolPreview }
  | { type: "error"; message: string }
  | { type: "reset" };

const INITIAL: PreviewState = { tools: [], specURL: null, error: null, loading: false };

function reducer(state: PreviewState, action: Action): PreviewState {
  switch (action.type) {
    case "loading":
      return { ...INITIAL, loading: true };
    case "success":
      return {
        tools: action.payload.tools ?? [],
        specURL: action.payload.specURL ?? null,
        error: action.payload.error ?? null,
        loading: false,
      };
    case "error":
      return { ...INITIAL, error: action.message };
    case "reset":
      return INITIAL;
  }
}

export function useOpenAPIToolPreview(
  workspaceName: string,
  registryName: string,
  handlerName: string
): PreviewState {
  const [state, dispatch] = useReducer(reducer, INITIAL);

  useEffect(() => {
    if (!handlerName) {
      dispatch({ type: "reset" });
      return;
    }

    let cancelled = false;
    dispatch({ type: "loading" });

    const url = `/api/workspaces/${workspaceName}/toolregistries/${registryName}/tools?handler=${encodeURIComponent(handlerName)}`;
    fetch(url)
      .then((r) => r.json() as Promise<OpenAPIToolPreview>)
      .then((data) => {
        if (cancelled) return;
        dispatch({ type: "success", payload: data });
      })
      .catch((e: unknown) => {
        if (cancelled) return;
        dispatch({ type: "error", message: e instanceof Error ? e.message : "Failed to load tools" });
      });

    return () => {
      cancelled = true;
    };
  }, [workspaceName, registryName, handlerName]);

  return state;
}
