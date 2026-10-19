import { expect, type Page } from "@playwright/test";

/**
 * Mock mode keeps MSW state in the page's memory, so a full reload resets every store.
 * Tests sign in once with `page.goto` and then move only through in-app navigation.
 */
export const PASSWORD = "matkhau123";

export async function login(page: Page, email: string, path = "/") {
  await page.goto(`/login?next=${encodeURIComponent(path)}`);
  await fillLogin(page, email);
}

async function fillLogin(page: Page, email: string) {
  await page.getByLabel(/Email/).fill(email);
  await page.getByLabel(/Mật khẩu/).fill(PASSWORD);
  await page.getByRole("button", { name: "Đăng nhập" }).click();
  await expect(page).not.toHaveURL(/\/login/);
}

/** Logs out through the account menu and signs in as another user without reloading. */
export async function switchUser(page: Page, email: string) {
  await page.getByRole("button", { name: /Tài khoản/ }).click();
  await page.getByRole("menuitem", { name: "Đăng xuất" }).click();
  await expect(page).toHaveURL(/\/login/);
  await fillLogin(page, email);
}

export function nav(page: Page) {
  return page.getByRole("complementary", { name: "Điều hướng chính" });
}

/** Mock jobs advance on the 3 s polling fallback (no SSE in mock mode). */
export const JOB_TIMEOUT = 20_000;
