import { useEffect, useMemo, useRef, useState } from "react";
import {
  blankScene,
  isDirty,
  moveItem,
  toDrafts,
  toSceneInputs,
  validateStoryboard,
  type SceneDraft,
  type StoryboardIssue,
  type VideoProject,
} from "../domain/video";
import { useSaveScenes } from "./useVideo";

export interface StoryboardDrafts {
  drafts: SceneDraft[];
  issues: StoryboardIssue[];
  dirty: boolean;
  totalMs: number;
  update: (key: string, patch: Partial<SceneDraft>) => void;
  move: (from: number, to: number) => void;
  remove: (key: string) => void;
  add: () => void;
  reset: () => void;
  save: () => void;
  saving: boolean;
  saveError: unknown;
}

/** Local, editable copy of the storyboard; re-syncs from the server when it is not dirty. */
export function useStoryboardDrafts(project: VideoProject): StoryboardDrafts {
  const [drafts, setDrafts] = useState<SceneDraft[]>(() => toDrafts(project.scenes));
  const dirtyRef = useRef(false);
  const mutation = useSaveScenes(project.id);

  const dirty = useMemo(() => isDirty(drafts, project.scenes), [drafts, project.scenes]);
  dirtyRef.current = dirty;

  // Server changed (storyboard job finished, scenes regenerated…): adopt it unless the user is editing.
  useEffect(() => {
    if (!dirtyRef.current) setDrafts(toDrafts(project.scenes));
  }, [project.scenes]);

  const issues = useMemo(() => validateStoryboard(drafts, project.limits), [drafts, project.limits]);
  const totalMs = drafts.reduce((s, d) => s + d.durationMs, 0);

  return {
    drafts,
    issues,
    dirty,
    totalMs,
    update: (key, patch) => setDrafts((list) => list.map((d) => (d.key === key ? { ...d, ...patch } : d))),
    move: (from, to) => setDrafts((list) => moveItem(list, from, to)),
    remove: (key) => setDrafts((list) => list.filter((d) => d.key !== key)),
    add: () => setDrafts((list) => [...list, blankScene()]),
    reset: () => setDrafts(toDrafts(project.scenes)),
    save: () =>
      mutation.mutate(toSceneInputs(drafts), {
        onSuccess: (p) => setDrafts(toDrafts(p.scenes)),
      }),
    saving: mutation.isPending,
    saveError: mutation.error,
  };
}
