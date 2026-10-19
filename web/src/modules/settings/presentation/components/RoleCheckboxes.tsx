import type { Role } from "@/shared/kernel";
import { ROLE_LABELS } from "@/shared/lib/labels";
import { Checkbox } from "@/shared/ui/checkbox";
import { ALL_ROLES, ROLE_DESCRIPTIONS } from "../../domain/user";

export function RoleCheckboxes({
  value,
  onChange,
  disabledRoles = [],
  invalid,
  describedBy,
}: {
  value: Role[];
  onChange: (roles: Role[]) => void;
  disabledRoles?: Role[];
  invalid?: boolean;
  describedBy?: string;
}) {
  const toggle = (role: Role, checked: boolean) =>
    onChange(checked ? [...value, role] : value.filter((r) => r !== role));
  return (
    <div role="group" aria-label="Vai trò" aria-invalid={invalid || undefined} aria-describedby={describedBy} className="flex flex-col gap-2">
      {ALL_ROLES.map((role) => {
        const id = `role-${role}`;
        return (
          <div key={role} className="flex items-start gap-2">
            <Checkbox
              id={id}
              className="mt-0.5"
              checked={value.includes(role)}
              disabled={disabledRoles.includes(role)}
              onCheckedChange={(c) => toggle(role, c === true)}
            />
            <label htmlFor={id} className="flex flex-col">
              <span className="text-body text-neutral-900">{ROLE_LABELS[role]}</span>
              <span className="text-small text-neutral-500">{ROLE_DESCRIPTIONS[role]}</span>
            </label>
          </div>
        );
      })}
    </div>
  );
}
