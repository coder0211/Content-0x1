import { expect, test } from "@playwright/test";
import { login, nav } from "./helpers";

/** ACS-105: login + RBAC basics (SPEC-01 §2 role matrix). Menus are hidden; routes show NoPermission. */
test("content creator: studios visible, no Budgets/Settings/Review", async ({ page }) => {
  await login(page, "creator@acs.local");
  const menu = nav(page);
  for (const label of ["Nội dung", "Hình ảnh", "Video", "Campaign", "Thư viện"]) {
    await expect(menu.getByRole("link", { name: new RegExp(label) })).toBeVisible();
  }
  for (const label of ["Chi phí & ngân sách", "Cài đặt", "Duyệt"]) {
    await expect(menu.getByRole("link", { name: new RegExp(label) })).toHaveCount(0);
  }
  // Direct URL → NoPermission (never a silent hide).
  await page.goto("/usage/budgets");
  await expect(page.getByText("Bạn không có quyền xem mục này. Liên hệ Admin.")).toBeVisible();
  await page.goto("/settings");
  await expect(page.getByText("Bạn không có quyền xem mục này. Liên hệ Admin.")).toBeVisible();
});

test("reviewer: can review, cannot generate", async ({ page }) => {
  await login(page, "reviewer@acs.local");
  const menu = nav(page);
  await expect(menu.getByRole("link", { name: /Nội dung/ })).toHaveCount(0);
  await expect(menu.getByRole("link", { name: /Cài đặt/ })).toHaveCount(0);

  await menu.getByRole("link", { name: /Duyệt/ }).click();
  const queue = page.getByRole("complementary", { name: "Danh sách chờ duyệt" });
  const first = queue.getByRole("button", { name: /Bài Facebook: Bánh kem Tết vị trà xanh/ });
  await first.click();
  const pane = page.getByRole("article", { name: "Duyệt: Bài Facebook: Bánh kem Tết vị trà xanh" });
  await pane.getByLabel("Nhận xét").fill("Thêm giá bán lẻ vào bài.");
  await pane.getByRole("button", { name: "Yêu cầu sửa" }).click();
  await expect(first).toHaveCount(0);

  await page.goto("/content");
  await expect(page.getByText("Bạn không có quyền xem mục này. Liên hệ Admin.")).toBeVisible();
});

test("admin sees Settings and Budgets", async ({ page }) => {
  await login(page, "admin@acs.local");
  const menu = nav(page);
  await expect(menu.getByRole("link", { name: /Cài đặt/ })).toBeVisible();
  await expect(menu.getByRole("link", { name: /Chi phí & ngân sách/ })).toBeVisible();
  await menu.getByRole("link", { name: /Cài đặt/ }).click();
  await expect(page).toHaveURL(/\/settings/);
});
