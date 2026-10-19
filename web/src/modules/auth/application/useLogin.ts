import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useRepository } from "@/shared/lib/di";
import type { Credentials } from "../domain/user";
import { authKeys } from "./keys";

export function useLogin() {
  const repo = useRepository("auth");
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (c: Credentials) => repo.login({ email: c.email.trim(), password: c.password }),
    onSuccess: (user) => queryClient.setQueryData(authKeys.me, user),
    // The login form renders errors inline.
    meta: { silentError: true },
  });
}
