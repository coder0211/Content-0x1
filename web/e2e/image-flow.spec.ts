import { expect, test } from "@playwright/test";
import { JOB_TIMEOUT, login, nav } from "./helpers";

/** Flow B (SPEC-01, ACS-105): Image Studio → 4 images → variations of one → submit for review. */
test("Flow B: generate 4 images, add variations and submit for review", async ({ page }) => {
  test.setTimeout(90_000);
  const prompt = `Banner ưu đãi học phí E2E ${Date.now() % 100000}`;

  await login(page, "creator@acs.local", "/campaigns");
  await page.getByRole("cell", { name: "Khóa giao tiếp T11" }).click();
  await page.getByRole("tab", { name: "Hình ảnh" }).click();
  await page.getByRole("link", { name: /Tạo mới|Tạo ảnh/ }).click();
  await expect(page).toHaveURL(/\/images\?campaign_id=/);

  const form = page.getByRole("form", { name: "Tạo ảnh" });
  await form.getByRole("radiogroup", { name: "Mục đích" }).getByRole("radio").first().click();
  await form.getByLabel(/Mô tả ảnh/).fill(prompt);
  await form.getByRole("radiogroup", { name: "Số ảnh" }).getByRole("radio", { name: "4" }).click();
  await form.getByRole("button", { name: "Tạo 4 ảnh" }).click();
  await expect(page.getByText("Đã gửi yêu cầu tạo 4 ảnh")).toBeVisible();

  const gallery = page.getByRole("main", { name: "Kết quả" });
  const images = gallery.getByRole("button", { name: new RegExp(`^Ảnh \\d — ${prompt}`) });
  await expect(images).toHaveCount(4, { timeout: JOB_TIMEOUT });

  // Variations of the first image → a new version of the same asset.
  await images.first().click();
  const inspector = page.getByRole("complementary", { name: "Chi tiết ảnh" });
  await inspector.getByRole("button", { name: /^Tạo \d biến thể$/ }).click();
  await expect(page.getByText("Đã gửi yêu cầu tạo biến thể")).toBeVisible();
  await expect(images).toHaveCount(6, { timeout: JOB_TIMEOUT });

  // Submit the current version (the newest batch is first in the gallery).
  await images.first().click();
  await inspector.getByRole("button", { name: "Gửi duyệt" }).click();
  await expect(page.getByText("Đã gửi duyệt asset")).toBeVisible();

  // The library (asset module) shows it pending review.
  await nav(page).getByRole("link", { name: /Thư viện/ }).click();
  const row = page.getByRole("row", { name: new RegExp(prompt) });
  await expect(row).toContainText("Chờ duyệt");
});
