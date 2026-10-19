import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http as mswHttp } from "msw";
import { server } from "@/app/mocks/server";
import { api, mockSession, ok } from "@/shared/api/mock";
import { renderApp } from "@/test/render";
import { resetBudgetMocks } from "../../infrastructure/mocks";

afterEach(resetBudgetMocks);

describe("BudgetsPage (ACS-093)", () => {
  it("edits global and per-job limits", async () => {
    let body: unknown = null;
    server.events.on("request:start", async ({ request }) => {
      if (request.method === "PATCH") body = await request.clone().json();
    });
    mockSession.signInAs("lead@acs.local");
    renderApp("/usage/budgets");
    const limit = await screen.findByLabelText(/Hạn mức tháng/);
    const user = userEvent.setup();
    await user.clear(limit);
    await user.type(limit, "1500");
    const video = screen.getByLabelText(/Video \(USD\)/);
    await user.clear(video);
    await user.type(video, "10");
    await user.click(screen.getByRole("button", { name: "Lưu cài đặt ngân sách" }));
    expect(await screen.findByText("Đã lưu cài đặt ngân sách")).toBeInTheDocument();
    expect(body).toMatchObject({ global: { limit_amount: 1500, alert_thresholds: [80, 100], hard_stop: true }, per_job_limits: { video: 10 } });
    server.events.removeAllListeners();
  });

  it("validates thresholds client-side", async () => {
    mockSession.signInAs("admin@acs.local");
    renderApp("/usage/budgets");
    const th = await screen.findByLabelText(/Ngưỡng cảnh báo/);
    const user = userEvent.setup();
    await user.clear(th);
    await user.type(th, "120");
    await user.click(screen.getByRole("button", { name: "Lưu cài đặt ngân sách" }));
    expect(await screen.findByText("Ngưỡng là số nguyên từ 1 đến 100.")).toBeInTheDocument();
  });

  it("edits a campaign budget from the table", async () => {
    mockSession.signInAs("admin@acs.local");
    renderApp("/usage/budgets");
    const table = await screen.findByRole("table", { name: "Ngân sách campaign" });
    expect(within(table).getByText("Ra mắt Thu 2026")).toBeInTheDocument();
    const user = userEvent.setup();
    await user.click(within(table).getByRole("button", { name: "Thao tác" }));
    await user.click(await screen.findByRole("menuitem", { name: "Sửa ngân sách" }));
    const dialog = await screen.findByRole("dialog");
    const input = within(dialog).getByLabelText(/Hạn mức \(USD\)/);
    await user.clear(input);
    await user.type(input, "200");
    await user.click(within(dialog).getByRole("button", { name: "Lưu ngân sách" }));
    expect(await screen.findByText("Đã lưu ngân sách campaign")).toBeInTheDocument();
    expect(await within(await screen.findByRole("table", { name: "Ngân sách campaign" })).findByText("$200.00")).toBeInTheDocument();
  });

  it("warns when the global budget is exhausted", async () => {
    server.use(
      mswHttp.get(api("/settings/budgets"), () => {
        const g = { id: "g", scope: "global", campaign_id: null, period_start: "2026-10-01", period_end: "2026-10-31", limit_amount: 10, reserved_amount: 0, spent_amount: 10, remaining_amount: 0, percent_used: 100, currency: "USD", alert_thresholds: [80, 100], alerts_sent: [80, 100], hard_stop: true, updated_at: "2026-10-19T00:00:00Z" };
        return ok({ currency: "USD", per_job_limits: { text: 0.5, image: 2, video: 8 }, global_template: { monthly_limit: 10, alert_thresholds: [80, 100], hard_stop: true }, global: g, campaigns: [], max_concurrent_video_jobs_per_user: 3, updated_by: null, updated_at: "2026-10-19T00:00:00Z" });
      }),
    );
    mockSession.signInAs("admin@acs.local");
    renderApp("/usage/budgets");
    expect(await screen.findByText("Đã hết ngân sách tháng")).toBeInTheDocument();
    expect(screen.getByText(/Chưa campaign nào có ngân sách riêng/)).toBeInTheDocument();
  });
});
