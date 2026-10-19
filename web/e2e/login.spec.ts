import { expect, test } from "@playwright/test";

test("login smoke: redirect, sign in, permission-based menu, logout", async ({ page }) => {
  await page.goto("/campaigns");
  await expect(page).toHaveURL(/\/login\?next=%2Fcampaigns/);

  await page.getByLabel(/Email/).fill("creator@acs.local");
  await page.getByLabel(/Mật khẩu/).fill("matkhau123");
  await page.getByRole("button", { name: "Đăng nhập" }).click();

  await expect(page).toHaveURL(/\/campaigns$/);
  await expect(page.getByRole("heading", { name: "Campaign" })).toBeVisible();

  const nav = page.getByRole("complementary", { name: "Điều hướng chính" });
  await expect(nav.getByRole("link", { name: /Nội dung/ })).toBeVisible();
  await expect(nav.getByRole("link", { name: /Chi phí & ngân sách/ })).toHaveCount(0);

  await page.getByRole("button", { name: /Tài khoản/ }).click();
  await page.getByRole("menuitem", { name: "Đăng xuất" }).click();
  await expect(page).toHaveURL(/\/login$/);
});

test("wrong password shows inline error", async ({ page }) => {
  await page.goto("/login");
  await page.getByLabel(/Email/).fill("admin@acs.local");
  await page.getByLabel(/Mật khẩu/).fill("sai-mat-khau");
  await page.getByRole("button", { name: "Đăng nhập" }).click();
  await expect(page.getByText("Email hoặc mật khẩu không đúng.")).toBeVisible();
});

test("design page renders without login", async ({ page }) => {
  await page.goto("/_design");
  await expect(page.getByRole("heading", { name: "Design system" })).toBeVisible();
});
