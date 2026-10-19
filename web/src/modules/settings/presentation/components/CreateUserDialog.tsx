import { useState, type FormEvent } from "react";
import { isApiError, type Role } from "@/shared/kernel";
import { Button } from "@/shared/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/shared/ui/dialog";
import { Input } from "@/shared/ui/input";
import { FormField } from "@/shared/ui/patterns";
import { toast } from "@/shared/ui/toast";
import { useCreateUser } from "../../application/useUsers";
import { MIN_PASSWORD_LENGTH, validateCreateUser, type CreateUserErrors } from "../../domain/user";
import { RoleCheckboxes } from "./RoleCheckboxes";

const SERVER_ISSUES: Record<string, string> = {
  required: "Trường này bắt buộc.",
  invalid: "Giá trị không hợp lệ.",
  taken: "Email đã được dùng cho user khác.",
  too_short: `Mật khẩu tạm cần ít nhất ${MIN_PASSWORD_LENGTH} ký tự.`,
};

export function CreateUserDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (o: boolean) => void }) {
  const [email, setEmail] = useState("");
  const [name, setName] = useState("");
  const [roles, setRoles] = useState<Role[]>(["content_creator"]);
  const [password, setPassword] = useState("");
  const [errors, setErrors] = useState<CreateUserErrors>({});
  const create = useCreateUser();

  const reset = () => {
    setEmail("");
    setName("");
    setRoles(["content_creator"]);
    setPassword("");
    setErrors({});
  };

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    const input = { email, name, roleCodes: roles, tempPassword: password };
    const v = validateCreateUser(input);
    setErrors(v);
    if (Object.keys(v).length > 0) return;
    try {
      const user = await create.mutateAsync(input);
      toast.success(`Đã tạo tài khoản ${user.email}`, { description: "Gửi mật khẩu tạm cho user qua kênh nội bộ." });
      reset();
      onOpenChange(false);
    } catch (err) {
      if (isApiError(err) && (err.code === "VALIDATION_FAILED" || err.code === "CONFLICT")) {
        const fe = err.fieldErrors();
        const mapped: CreateUserErrors = {};
        for (const [field, issue] of Object.entries(fe)) {
          if (field === "email" || field === "name" || field === "role_codes" || field === "temp_password") {
            mapped[field] = SERVER_ISSUES[issue] ?? issue;
          }
        }
        if (Object.keys(mapped).length > 0) {
          setErrors(mapped);
          return;
        }
      }
      toast.error(err);
    }
  };

  return (
    <Dialog
      open={open}
      onOpenChange={(o) => {
        if (!o) reset();
        onOpenChange(o);
      }}
    >
      <DialogContent>
        <form onSubmit={(e) => void submit(e)} noValidate className="flex flex-col gap-4">
          <DialogHeader>
            <DialogTitle>Tạo user</DialogTitle>
            <DialogDescription>User đăng nhập bằng mật khẩu tạm và nên đổi ngay sau lần đầu.</DialogDescription>
          </DialogHeader>
          <FormField label="Email" required error={errors.email}>
            <Input type="email" autoComplete="off" value={email} onChange={(e) => setEmail(e.target.value)} />
          </FormField>
          <FormField label="Họ tên" required error={errors.name}>
            <Input value={name} onChange={(e) => setName(e.target.value)} />
          </FormField>
          <div className="flex flex-col gap-1.5">
            <span className="text-body font-medium text-neutral-900">
              Vai trò<span className="ml-0.5 text-danger" aria-hidden>*</span>
            </span>
            <RoleCheckboxes value={roles} onChange={setRoles} invalid={Boolean(errors.role_codes)} />
            {errors.role_codes ? (
              <p role="alert" className="text-small text-danger">
                {errors.role_codes}
              </p>
            ) : null}
          </div>
          <FormField label="Mật khẩu tạm" required helper={`Tối thiểu ${MIN_PASSWORD_LENGTH} ký tự.`} error={errors.temp_password}>
            <Input type="password" autoComplete="new-password" value={password} onChange={(e) => setPassword(e.target.value)} />
          </FormField>
          <DialogFooter>
            <Button type="button" variant="secondary" onClick={() => onOpenChange(false)}>
              Hủy
            </Button>
            <Button type="submit" variant="primary" loading={create.isPending}>
              Tạo user
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
