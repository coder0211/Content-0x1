import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useRepository } from "@/shared/lib/di";
import { makeCan, SessionContext, type Session } from "@/shared/lib/session";
import type { AuthUser } from "../domain/user";
import { authKeys } from "./keys";

/** Why the session ended — drives the login redirect (`next`/`reason`). */
export type SessionEndReason = "logout" | "expired" | null;

interface AuthInternals {
  endReason: SessionEndReason;
  meError: unknown;
  retryMe: () => void;
}

const AuthInternalsContext = createContext<AuthInternals | null>(null);

export function useAuthInternals(): AuthInternals {
  const ctx = useContext(AuthInternalsContext);
  if (!ctx) throw new Error("useAuthInternals must be used inside <AuthProvider>");
  return ctx;
}

/** Loads `GET /me`, exposes the session via `SessionContext`, reacts to 401s globally. */
export function AuthProvider({ children }: { children: ReactNode }) {
  const repo = useRepository("auth");
  const queryClient = useQueryClient();
  const [endReason, setEndReason] = useState<SessionEndReason>(null);

  const me = useQuery<AuthUser | null>({
    queryKey: authKeys.me,
    queryFn: () => repo.me(),
    staleTime: 5 * 60_000,
    retry: false,
  });

  useEffect(
    () =>
      repo.onUnauthorized((code) => {
        if (queryClient.getQueryData(authKeys.me) == null) return;
        setEndReason(code === "SESSION_EXPIRED" || code === "UNAUTHENTICATED" ? "expired" : null);
        queryClient.clear();
        queryClient.setQueryData(authKeys.me, null);
      }),
    [repo, queryClient],
  );

  const logout = useCallback(async () => {
    try {
      await repo.logout();
    } finally {
      setEndReason("logout");
      queryClient.clear();
      queryClient.setQueryData(authKeys.me, null);
    }
  }, [repo, queryClient]);

  const user = me.data ?? null;
  const session = useMemo<Session>(
    () => ({
      status: me.isPending ? "loading" : user ? "authenticated" : "anonymous",
      user,
      can: makeCan(user),
      logout,
    }),
    [me.isPending, user, logout],
  );

  const { error: meError, refetch } = me;
  const internals = useMemo<AuthInternals>(
    () => ({ endReason, meError, retryMe: () => void refetch() }),
    [endReason, meError, refetch],
  );

  useEffect(() => {
    if (user) setEndReason(null);
  }, [user]);

  return (
    <AuthInternalsContext.Provider value={internals}>
      <SessionContext.Provider value={session}>{children}</SessionContext.Provider>
    </AuthInternalsContext.Provider>
  );
}
