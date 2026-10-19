import { isApiError } from "@/shared/kernel";
import { http, onUnauthorized, setCsrfToken } from "@/shared/api/http";
import type { AuthRepository } from "../application/ports";
import type { AuthUser, Credentials } from "../domain/user";
import { toAuthUser, type LoginResponseDto, type MeDto } from "./dto";

export class HttpAuthRepository implements AuthRepository {
  async me(): Promise<AuthUser | null> {
    try {
      return toAuthUser(await http.get<MeDto>("/me", { skipAuthRedirect: true }));
    } catch (e) {
      if (isApiError(e) && e.status === 401) return null;
      throw e;
    }
  }

  async login(c: Credentials): Promise<AuthUser> {
    const res = await http.post<LoginResponseDto>("/auth/login", c, { skipAuthRedirect: true });
    setCsrfToken(res.csrf_token);
    return toAuthUser(res.user);
  }

  async logout(): Promise<void> {
    try {
      await http.post<void>("/auth/logout", undefined, { skipAuthRedirect: true });
    } finally {
      setCsrfToken(null);
    }
  }

  onUnauthorized(listener: (code: string) => void): () => void {
    return onUnauthorized((e) => listener(e.code));
  }
}
