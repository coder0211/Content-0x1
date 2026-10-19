import type { AuthUser, Credentials } from "../domain/user";

export interface AuthRepository {
  /** Current user, or `null` when not signed in (401). */
  me(): Promise<AuthUser | null>;
  login(credentials: Credentials): Promise<AuthUser>;
  logout(): Promise<void>;
  /** Subscribe to 401s from any API call. Returns an unsubscribe function. */
  onUnauthorized(listener: (code: string) => void): () => void;
}

declare module "@/shared/lib/di" {
  interface RepositoryRegistry {
    auth: AuthRepository;
  }
}
