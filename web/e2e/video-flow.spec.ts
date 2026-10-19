import { expect, test } from "@playwright/test";
import { JOB_TIMEOUT, login, nav } from "./helpers";

/**
 * Flow C (SPEC-01, ACS-105): Video Studio → storyboard → edit a scene → estimate → produce →
 * regenerate the failed scene → submit for review. In mock mode a scene whose visual prompt
 * contains "[lỗi]" fails during production (partial failure demo).
 */
test("Flow C: storyboard, produce, regenerate a scene and submit", async ({ page }) => {
  test.setTimeout(120_000);
  const prompt = `Video giới thiệu khóa giao tiếp E2E ${Date.now() % 100000}`;

  await login(page, "creator@acs.local", "/campaigns");
  await page.getByRole("cell", { name: "Khóa giao tiếp T11" }).click();
  await page.getByRole("tab", { name: "Video" }).click();
  await page.getByRole("link", { name: /Tạo mới|Tạo video/ }).click();
  await expect(page).toHaveURL(/\/videos\?campaign_id=/);

  await page.getByRole("textbox", { name: /Mô tả video/ }).fill(prompt);
  await page.getByRole("button", { name: "Tạo storyboard" }).click();
  await expect(page.getByRole("heading", { level: 1, name: prompt })).toBeVisible();

  // Storyboard ready → edit scene 2 (marked to fail in production) and save.
  const scene2 = page.getByRole("textbox", { name: "Mô tả hình ảnh cảnh 2" });
  await expect(scene2).toBeVisible({ timeout: JOB_TIMEOUT });
  await scene2.fill("Lớp học nhỏ, học viên luyện nói theo cặp [lỗi]");
  await page.getByRole("button", { name: "Lưu storyboard" }).click();
  await expect(page.getByRole("button", { name: "Lưu storyboard" })).toBeDisabled();

  // Estimate → produce.
  const estimate = page.getByRole("region", { name: "Dự toán" });
  await expect(estimate.getByText("Tổng dự toán")).toBeVisible();
  await estimate.getByRole("button", { name: "Sản xuất video" }).click();
  await page.getByRole("tab", { name: "Sản xuất" }).click();

  // Scene 2 fails → regenerate only that scene with a new prompt.
  const card = page.getByRole("listitem", { name: "Cảnh 2" });
  await expect(card.getByText(/ContentPolicy/)).toBeVisible({ timeout: JOB_TIMEOUT });
  await card.getByRole("button", { name: "Tạo lại cảnh 2" }).click();
  const dialog = page.getByRole("dialog", { name: "Tạo lại cảnh 2" });
  await dialog.getByRole("textbox", { name: "Mô tả hình ảnh mới" }).fill("Học viên cười trong lớp học sáng sủa");
  await dialog.getByRole("button", { name: "Tạo lại cảnh 2" }).click();

  // Render finishes → submit for review.
  const submit = page.getByRole("button", { name: "Gửi duyệt" });
  await expect(submit).toBeEnabled({ timeout: JOB_TIMEOUT });
  await submit.click();
  await expect(page.getByText("Đã gửi duyệt video")).toBeVisible();

  // The asset module now lists it as pending review.
  await nav(page).getByRole("link", { name: /Thư viện/ }).click();
  await expect(page.getByRole("row", { name: new RegExp(prompt) })).toContainText("Chờ duyệt");
});
