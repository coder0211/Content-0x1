import { screen, within } from "@testing-library/react";
import { server } from "@/app/mocks/server";
import { api, fail, mockSession, rest } from "@/shared/api/mock";
import { renderApp } from "@/test/render";

describe("DashboardPage (ACS-095)", () => {
  it("shows all widgets including cost for a marketing lead", async () => {
    mockSession.signInAs("lead@acs.local");
    renderApp("/");
    const metrics = await screen.findByRole("group", { name: "Số liệu chính" });
    expect(within(metrics).getByText("Campaign đang chạy")).toBeInTheDocument();
    expect(within(metrics).getByText("Chi phí tháng này")).toBeInTheDocument();
    expect(screen.getByRole("region", { name: "Job gần đây" })).toHaveTextContent("PROVIDER_TIMEOUT");
    expect(screen.getByRole("link", { name: /Tạo nội dung/ })).toHaveAttribute("href", "/content");
  });

  it("hides cost without usage.view and shows the creator's own to-dos", async () => {
    mockSession.signInAs("creator@acs.local");
    renderApp("/");
    const metrics = await screen.findByRole("group", { name: "Số liệu chính" });
    expect(within(metrics).queryByText("Chi phí tháng này")).not.toBeInTheDocument();
    const todo = screen.getByRole("region", { name: "Cần bạn xử lý" });
    expect(within(todo).getByRole("link", { name: /Asset bị yêu cầu sửa/ })).toHaveAttribute("href", "/library?status=changes_requested");
    expect(within(todo).queryByText("Asset chờ bạn duyệt")).not.toBeInTheDocument();
  });

  it("shows an error state with retry", async () => {
    mockSession.signInAs("lead@acs.local");
    server.use(rest.get(api("/dashboard/summary"), () => fail(500, "INTERNAL", "Lỗi máy chủ")));
    renderApp("/");
    // 5xx is retried twice with backoff before the error state shows.
    expect(await screen.findByRole("button", { name: /Thử lại/ }, { timeout: 6000 })).toBeInTheDocument();
  }, 15_000);
});
