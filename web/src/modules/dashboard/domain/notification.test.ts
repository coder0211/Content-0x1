import { notificationLink, notificationText } from "./notification";

describe("notifications", () => {
  it("renders Vietnamese text per type", () => {
    expect(notificationText({ type: "asset_approved", payload: { title: "Bài A", actor_name: "Người duyệt" } })).toBe(
      "Người duyệt đã duyệt “Bài A”",
    );
    expect(notificationText({ type: "changes_requested", payload: { title: "Bài A" } })).toBe("Reviewer yêu cầu sửa “Bài A”");
    expect(notificationText({ type: "job_failed", payload: { job_type: "video_production" } })).toBe("Sản xuất video bị lỗi");
    expect(notificationText({ type: "budget_alert", payload: { scope: "global", threshold: 80 } })).toBe(
      "Ngân sách toàn hệ thống đã dùng 80%",
    );
  });

  it("links to the asset, the review queue, the job or budgets", () => {
    expect(notificationLink({ type: "asset_approved", payload: { asset_id: "a1" } })).toBe("/library/a1");
    expect(notificationLink({ type: "review_requested", payload: { asset_id: "a1" } })).toBe("/review");
    expect(notificationLink({ type: "job_failed", payload: { job_id: "j1" } })).toBe("/jobs/j1");
    expect(notificationLink({ type: "budget_alert", payload: {} })).toBe("/usage/budgets");
  });
});
