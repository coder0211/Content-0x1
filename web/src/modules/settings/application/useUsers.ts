import { useInfiniteQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useRepository } from "@/shared/lib/di";
import type { CreateUserInput, ManagedUser, UpdateUserInput, UserQuery } from "../domain/user";
import { settingsKeys } from "./keys";

export function useUsers(q: Omit<UserQuery, "cursor">, opts: { enabled?: boolean } = {}) {
  const repo = useRepository("settings");
  const query = useInfiniteQuery({
    queryKey: settingsKeys.userList(q),
    queryFn: ({ pageParam }) => repo.listUsers({ ...q, cursor: pageParam }),
    initialPageParam: null as string | null,
    getNextPageParam: (last) => last.nextCursor,
    enabled: opts.enabled ?? true,
  });
  const users: ManagedUser[] | undefined = query.data?.pages.flatMap((p) => p.items);
  return { ...query, users };
}

export function useCreateUser() {
  const repo = useRepository("settings");
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateUserInput) => repo.createUser(input),
    // Field errors are rendered in the form; other codes still toast via the dialog.
    meta: { silentError: true },
    onSuccess: () => qc.invalidateQueries({ queryKey: settingsKeys.users() }),
  });
}

export function useUpdateUser() {
  const repo = useRepository("settings");
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, input }: { id: string; input: UpdateUserInput }) => repo.updateUser(id, input),
    onSuccess: () => qc.invalidateQueries({ queryKey: settingsKeys.users() }),
  });
}
