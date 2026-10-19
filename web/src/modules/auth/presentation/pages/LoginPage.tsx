import { useState, type FormEvent } from "react";
import { Navigate, useNavigate, useSearchParams } from "react-router";
import { isApiError } from "@/shared/kernel";
import { env } from "@/shared/lib/env";
import { describeError } from "@/shared/lib/errors";
import { useSession } from "@/shared/lib/session";
import { Banner } from "@/shared/ui/banner";
import { Button } from "@/shared/ui/button";
import { Input } from "@/shared/ui/input";
import { FormField } from "@/shared/ui/patterns";
import { useLogin } from "../../application/useLogin";
import { safeRedirectTarget, validateCredentials, type Credentials } from "../../domain/user";

type FieldErrors = Partial<Record<keyof Credentials, string>>;

export default function LoginPage() {
  const session = useSession();
  const navigate = useNavigate();
  const [params] = useSearchParams();
  const next = safeRedirectTarget(params.get("next"));
  const expired = params.get("reason") === "expired";
  const login = useLogin();

  const [form, setForm] = useState<Credentials>({ email: "", password: "" });
  const [fieldErrors, setFieldErrors] = useState<FieldErrors>({});

  if (session.status === "authenticated") return <Navigate to={next} replace />;

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    const errors = validateCredentials(form);
    setFieldErrors(errors);
    if (Object.keys(errors).length > 0) return;
    login.mutate(form, {
      onSuccess: () => navigate(next, { replace: true }),
      onError: (err) => {
        if (isApiError(err) && err.code === "VALIDATION_FAILED") {
          const f = err.fieldErrors();
          setFieldErrors({
            email: f.email ? "Email không hợp lệ." : undefined,
            password: f.password ? "Nhập mật khẩu." : undefined,
          });
        }
      },
    });
  };

  const err = login.error;
  const errorMessage =
    err && !(isApiError(err) && err.code === "VALIDATION_FAILED")
      ? isApiError(err) && err.status === 401
        ? "Email hoặc mật khẩu không đúng."
        : describeError(err).message
      : null;

  return (
    <main className="flex min-h-screen items-center justify-center bg-neutral-25 px-4">
      <div className="w-full max-w-sm rounded-lg border border-neutral-200 bg-neutral-0 p-6">
        <p className="text-caption text-neutral-500">AI Content Studio</p>
        <h1 className="mt-1 text-page text-neutral-900">Đăng nhập</h1>

        {expired && !errorMessage ? (
          <Banner tone="warning" className="mt-4">
            Phiên đăng nhập đã hết hạn. Đăng nhập lại để tiếp tục.
          </Banner>
        ) : null}
        {errorMessage ? (
          <Banner tone="danger" className="mt-4">
            {errorMessage}
          </Banner>
        ) : null}

        <form className="mt-4 flex flex-col gap-4" onSubmit={onSubmit} noValidate>
          <FormField label="Email" required error={fieldErrors.email}>
            <Input
              type="email"
              autoComplete="username"
              inputSize="lg"
              autoFocus
              value={form.email}
              onChange={(e) => setForm((f) => ({ ...f, email: e.target.value }))}
            />
          </FormField>
          <FormField label="Mật khẩu" required error={fieldErrors.password}>
            <Input
              type="password"
              autoComplete="current-password"
              inputSize="lg"
              value={form.password}
              onChange={(e) => setForm((f) => ({ ...f, password: e.target.value }))}
            />
          </FormField>
          <Button type="submit" variant="primary" size="lg" loading={login.isPending}>
            Đăng nhập
          </Button>
        </form>

        {env.mocking ? (
          <p className="mt-4 text-small text-neutral-500">
            Chế độ mock: admin@acs.local, lead@acs.local, creator@acs.local, reviewer@acs.local · mật khẩu matkhau123
          </p>
        ) : null}
      </div>
    </main>
  );
}
