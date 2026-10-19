import { Check } from "lucide-react";
import { PERMISSIONS } from "@/shared/kernel";
import { ROLE_LABELS } from "@/shared/lib/labels";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/shared/ui/table";
import { ALL_ROLES, PERMISSION_LABELS, ROLE_MATRIX } from "../../domain/user";

/** Read-only role → permission matrix (SPEC-01 §2). */
export function RoleMatrix() {
  return (
    <div className="flex flex-col gap-3">
      <p className="text-body text-neutral-500">
        Quyền được gán theo vai trò. Người tạo asset không được tự duyệt asset của mình (trừ Admin).
      </p>
      <Table aria-label="Ma trận quyền">
        <TableHeader>
          <TableRow className="hover:bg-transparent">
            <TableHead>Quyền</TableHead>
            {ALL_ROLES.map((r) => (
              <TableHead key={r} align="center">
                {ROLE_LABELS[r]}
              </TableHead>
            ))}
          </TableRow>
        </TableHeader>
        <TableBody>
          {PERMISSIONS.map((p) => (
            <TableRow key={p} className="hover:bg-transparent">
              <TableCell>
                <span className="text-neutral-900">{PERMISSION_LABELS[p]}</span>
                <span className="ml-2 font-mono text-caption text-neutral-500">{p}</span>
              </TableCell>
              {ALL_ROLES.map((r) => (
                <TableCell key={r} align="center">
                  {ROLE_MATRIX[r].includes(p) ? (
                    <Check className="mx-auto size-4 text-success" aria-label="Có" />
                  ) : (
                    <span className="text-neutral-300" aria-label="Không">
                      –
                    </span>
                  )}
                </TableCell>
              ))}
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  );
}
