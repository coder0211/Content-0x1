import { expect, test } from "@playwright/test";
import { JOB_TIMEOUT, login, nav, switchUser } from "./helpers";

/**
 * Flow A (SPEC-01, ACS-105): campaign → Content Studio → 3 variants → rewrite → restore a revision →
 * submit for review → reviewer approves → creator downloads. Mock mode (fake provider).
 */
test("Flow A: content from a campaign to an approved download", async ({ page }) => {
  test.setTimeout(90_000);
  const message = `Giao tiếp E2E ${Date.now() % 100000}`;
  const title = `${message} — phương án A`;

  await login(page, "creator@acs.local", "/campaigns");
  await page.getByRole("cell", { name: "Khóa giao tiếp T11" }).click();
  await page.getByRole("tab", { name: "Nội dung" }).click();
  await page.getByRole("link", { name: /Tạo mới|Tạo nội dung/ }).click();
  await expect(page).toHaveURL(/\/content\?campaign_id=/);

  // Brief → 3 variants.
  const brief = page.getByRole("form", { name: "Brief" });
  await expect(brief.getByText("Khóa giao tiếp T11")).toBeVisible();
  await brief.getByLabel("Đối tượng").fill("Sinh viên năm 2");
  await brief.getByLabel("Thông điệp chính").fill(message);
  await brief.getByRole("button", { name: "Viết 3 phương án" }).click();
  const tabs = page.getByRole("tablist", { name: "Phương án" });
  await expect(tabs.getByRole("tab")).toHaveText(["Phương án A", "Phương án B", "Phương án C"], { timeout: JOB_TIMEOUT });
  await expect(page.getByLabel("Tiêu đề")).toHaveValue(title);

  // Quick rewrite → version 2.
  const history = page.getByRole("list", { name: "Lịch sử phiên bản" });
  await page.getByRole("toolbar", { name: "Viết lại nhanh" }).getByRole("button", { name: "Rút gọn" }).click();
  await expect(history.getByText(/Phiên bản 2/)).toBeVisible({ timeout: JOB_TIMEOUT });

  // Restore version 1 → version 3 (history is never rewritten).
  await page.getByRole("button", { name: "Khôi phục phiên bản 1" }).click();
  await expect(history.getByText(/Phiên bản 3/)).toBeVisible();
  await expect(page.getByText("Đã khôi phục thành phiên bản 3")).toBeVisible();

  // Submit for review.
  await page.getByRole("button", { name: "Gửi duyệt" }).click();
  await expect(page.getByText("Đã gửi duyệt", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Gửi duyệt" })).toHaveCount(0);

  // Reviewer approves from the queue.
  await switchUser(page, "reviewer@acs.local");
  await nav(page).getByRole("link", { name: /Duyệt/ }).click();
  const queue = page.getByRole("complementary", { name: "Danh sách chờ duyệt" });
  await queue.getByRole("button", { name: new RegExp(title) }).click();
  const pane = page.getByRole("article", { name: `Duyệt: ${title}` });
  await pane.getByRole("button", { name: "Duyệt asset" }).click();
  await expect(queue.getByRole("button", { name: new RegExp(title) })).toHaveCount(0);

  // Creator downloads the approved asset from the library.
  await switchUser(page, "creator@acs.local");
  await nav(page).getByRole("link", { name: /Thư viện/ }).click();
  await page.getByRole("cell", { name: title }).click();
  await expect(page.getByRole("heading", { level: 1, name: title })).toBeVisible();
  // Text content is exported by the asset API as a .txt (signed, audited URL opened in a new tab).
  const popup = page.waitForEvent("popup");
  await page.getByRole("button", { name: "Tải xuống" }).click();
  expect((await popup).url()).toMatch(/\.txt$/);
});
