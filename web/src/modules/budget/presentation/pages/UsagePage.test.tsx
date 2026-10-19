import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { delay, http as mswHttp } from "msw";
import { server } from "@/app/mocks/server";
import { api, fail, mockSession, ok } from "@/shared/api/mock";
import { formatUsd } from "@/shared/lib/format";
import { renderApp } from "@/test/render";
import { resetBudgetMocks, setMockUsage } from "../../infrastructure/mocks";

afterEach(resetBudgetMocks);

const today = new Date().toISOString().slice(0, 10);

describe("UsagePage", () => {
  it("shows totals that equal the sum of the table rows (ACS-092)", async () => {
    setMockUsage([
      { occurredAt: `${today}T01:00:00Z`, campaignId: null, provider: "fake", model: "fake-text", userId: "u1", jobType: "content_generation", cost: 0.123456, source: "calculated" },
      { occurredAt: `${today}T02:00:00Z`, campaignId: null, provider: "fake", model: "fake-image", userId: "u1", jobType: "image_generation", cost: 0.34, source: "provider_reported" },
      { occurredAt: `${today}T03:00:00Z`, campaignId: null, provider: "fake", model: "fake-video", userId: "u1", jobType: "video_production", cost: 2.5, source: "estimated" },
    ]);
    mockSession.signInAs("lead@acs.local");
    renderApp("/usage");
    const table = await screen.findByRole("table", { name: "Bảng chi phí" });
    const totalRow = await within(table).findByRole("row", { name: /^Tổng/ });
    expect(within(totalRow).getByText(formatUsd(2.963456))).toBeInTheDocument();
    expect(screen.getByRole("img", { name: "Chi phí theo ngày" })).toBeInTheDocument();

    const user = userEvent.setup();
    await user.click(screen.getByRole("tab", { name: "Model" }));
    await within(table).findByText("fake-video");
    expect(within(screen.getByRole("table", { name: "Bảng chi phí" })).getAllByRole("row")).toHaveLength(5); // header + 3 + total
  });

  it("shows the empty state", async () => {
    setMockUsage([]);
    mockSession.signInAs("admin@acs.local");
    renderApp("/usage");
    expect(await screen.findByText("Chưa có chi phí AI trong khoảng thời gian này.")).toBeInTheDocument();
  });

  it("shows loading then an error with retry", async () => {
    server.use(
      mswHttp.get(api("/usage/summary"), async () => {
        await delay(50);
        return fail(400, "VALIDATION_FAILED", "Lỗi");
      }),
    );
    mockSession.signInAs("admin@acs.local");
    renderApp("/usage");
    expect(await screen.findByRole("table", { name: "Bảng chi phí" })).toHaveAttribute("aria-busy", "true");
    expect(await screen.findByRole("button", { name: /Thử lại/ })).toBeInTheDocument();
  });

  it("denies users without usage.view", async () => {
    mockSession.signInAs("creator@acs.local");
    renderApp("/usage/budgets");
    expect(await screen.findByText("Bạn không có quyền xem mục này. Liên hệ Admin.")).toBeInTheDocument();
  });

  it("sends the date range to the API", async () => {
    let seen = "";
    server.use(
      mswHttp.get(api("/usage/summary"), ({ request }) => {
        seen = new URL(request.url).search;
        return ok({ from: "2026-10-01", to: "2026-10-19", group_by: "day", currency: "USD", rows: [], totals: { provider_reported: 0, calculated: 0, estimated: 0, total: 0, count: 0 } });
      }),
    );
    mockSession.signInAs("admin@acs.local");
    renderApp("/usage");
    await waitFor(() => expect(seen).toContain("group_by=day"));
    expect(seen).toContain("from=");
  });
});
