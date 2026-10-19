import { useState, type FormEvent } from "react";
import type { Role } from "@/shared/kernel";
import { Button } from "@/shared/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/shared/ui/dialog";
import { Input } from "@/shared/ui/input";
import { FormField } from "@/shared/ui/patterns";
import { toast } from "@/shared/ui/toast";
import { useUpdateUser } from "../../application/useUsers";
import type { ManagedUser } from "../../domain/user";
import { RoleCheckboxes } from "./RoleCheckboxes";

/** Edit name and roles. Mount with a `key` per user so the form resets. */
export function EditUserDialog({
  user,
  isSelf,
  onClose,
}: {
  user: ManagedUser;
  isSelf: boolean;
  onClose: () => void;
}) {
  const [name, setName] = useState(user.name);
  const [roles, setRoles] = useState<Role[]>(user.roles);
  const [errors, setErrors] = useState<{ name?: string; roles?: string }>({});
  const update = useUpdateUser();

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    const next: typeof errors = {};
    if (!name.trim()) next.name = "Nhập họ tên.";
    if (roles.length === 0) next.roles = "Chọn ít nhất một vai trò.";
    setErrors(next);
    if (Object.keys(next).length > 0) return;
    const rolesChanged = roles.length !== user.roles.length || roles.some((r) => !user.roles.includes(r));
    await update.mutateAsync({
      id: user.id,
      input: { name: name.trim() !== user.name ? name : undefined, roleCodes: rolesChanged ? roles : undefined },
    });
    toast.success(`Đã cập nhật ${name.trim()}`);
    onClose();
  };

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent>
        <form onSubmit={(e) => void submit(e).catch(() => undefined)} noValidate className="flex flex-col gap-4">
          <DialogHeader>
            <DialogTitle>Sửa user</DialogTitle>
            <DialogDescription>{user.email}</DialogDescription>
          </DialogHeader>
          <FormField label="Họ tên" required error={errors.name}>
            <Input value={name} onChange={(e) => setName(e.target.value)} />
          </FormField>
          <div className="flex flex-col gap-1.5">
            <span className="text-body font-medium text-neutral-900">Vai trò</span>
            {/* Admins cannot drop their own admin role (backend rule). */}
            <RoleCheckboxes value={roles} onChange={setRoles} disabledRoles={isSelf ? ["admin"] : []} invalid={Boolean(errors.roles)} />
            {errors.roles ? (
              <p role="alert" className="text-small text-danger">
                {errors.roles}
              </p>
            ) : null}
          </div>
          <DialogFooter>
            <Button type="button" variant="secondary" onClick={onClose}>
              Hủy
            </Button>
            <Button type="submit" variant="primary" loading={update.isPending}>
              Lưu thay đổi
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
