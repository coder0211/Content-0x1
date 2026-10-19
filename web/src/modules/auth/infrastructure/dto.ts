import type { Permission, Role } from "@/shared/kernel";
import type { AuthUser } from "../domain/user";

/** `GET /me` and `user` in `POST /auth/login` (SPEC-04 §2.1). */
export interface MeDto {
  id: string;
  email: string;
  name: string;
  roles: string[];
  permissions: string[];
}

export interface LoginResponseDto {
  user: MeDto;
  csrf_token: string;
}

export function toAuthUser(dto: MeDto): AuthUser {
  return {
    id: dto.id,
    email: dto.email,
    name: dto.name,
    roles: dto.roles as Role[],
    permissions: dto.permissions as Permission[],
  };
}
