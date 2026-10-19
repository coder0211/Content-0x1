import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { server } from "@/app/mocks/server";
import { api, fail, mockSession, rest } from "@/shared/api/mock";
import { renderApp } from "@/test/render";
import { createMockJob, getMockJob, resetJobMocks } from "../../infrastructure/mocks";

describe("job monitor", () => {
  beforeEach(() => resetJobMocks());
  afterEach(() => resetJobMocks({ seedData: true }));

  it("lists the user's jobs and opens the detail", async () => {
    const me = mockSession.signInAs("creator@acs.local");
    const job = createMockJob({ type: "image_generation", status: "failed", createdBy: me.id });
    createMockJob({ type: "content_rewrite", status: "succeeded", createdBy: "someone-else" });
    const { router } = renderApp("/jobs");

    const table = await screen.findByRole("table", { name: "Danh sách job" });
    expect(await within(table).findByText("Sinh ảnh")).toBeInTheDocument();
    expect(within(table).queryByText("Viết lại nội dung")).not.toBeInTheDocument();
    // creator column only for provider.view
    expect(within(table).queryByRole("columnheader", { name: "Người tạo" })).not.toBeInTheDocument();

    await userEvent.setup().click(within(table).getByText("Sinh ảnh"));
    await waitFor(() => expect(router.state.location.pathname).toBe(`/jobs/${job.id}`));
    expect(await screen.findByRole("heading", { name: "Sinh ảnh" })).toBeInTheDocument();
    expect(await screen.findByRole("table", { name: "Các bước" })).toBeInTheDocument();
  });

  it("shows the empty state", async () => {
    mockSession.signInAs("creator@acs.local");
    renderApp("/jobs");
    expect(await screen.findByText(/Chưa có job nào/)).toBeInTheDocument();
  });

  it("shows the filtered-empty state and clears filters", async () => {
    const me = mockSession.signInAs("creator@acs.local");
    createMockJob({ status: "succeeded", createdBy: me.id });
    const { router } = renderApp("/jobs?status=failed");
    expect(await screen.findByText("Không có job nào khớp bộ lọc.")).toBeInTheDocument();
    await userEvent.setup().click(screen.getByRole("button", { name: "Xóa bộ lọc" }));
    await waitFor(() => expect(router.state.location.search).toBe(""));
    expect(await screen.findByText("Viết nội dung")).toBeInTheDocument();
  });

  it("shows a load error with retry", async () => {
    mockSession.signInAs("creator@acs.local");
    server.use(rest.get(api("/jobs"), () => fail(400, "VALIDATION_FAILED", "boom")));
    renderApp("/jobs");
    expect(await screen.findByRole("alert")).toHaveTextContent("Không tải được dữ liệu.");
  });

  it("shows the creator column for provider.view", async () => {
    const lead = mockSession.signInAs("lead@acs.local");
    createMockJob({ createdBy: lead.id, status: "succeeded" });
    renderApp("/jobs");
    const table = await screen.findByRole("table", { name: "Danh sách job" });
    expect(within(table).getByRole("columnheader", { name: "Người tạo" })).toBeInTheDocument();
    expect(await within(table).findByText("Bạn")).toBeInTheDocument();
  });

  it("retries a failed job from the detail page", async () => {
    const me = mockSession.signInAs("creator@acs.local");
    const job = createMockJob({ status: "failed", createdBy: me.id });
    renderApp(`/jobs/${job.id}`);
    const retry = await screen.findByRole("button", { name: "Thử lại" });
    await userEvent.setup().click(retry);
    await waitFor(() => expect(getMockJob(job.id)?.status).not.toBe("failed"));
    expect(await screen.findByText("Chạy lại")).toBeInTheDocument();
  });

  it("cancels a running job after confirmation", async () => {
    const me = mockSession.signInAs("creator@acs.local");
    const job = createMockJob({ status: "queued", createdBy: me.id });
    renderApp(`/jobs/${job.id}`);
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "Hủy" }));
    const dialog = await screen.findByRole("alertdialog");
    await user.click(within(dialog).getByRole("button", { name: "Hủy job" }));
    await waitFor(() => expect(getMockJob(job.id)?.status).toBe("cancelled"));
    expect(await screen.findByText("Đã hủy", { selector: "p" })).toBeInTheDocument();
  });

  it("shows not-found for someone else's job", async () => {
    mockSession.signInAs("creator@acs.local");
    const other = createMockJob({ createdBy: "someone-else" });
    renderApp(`/jobs/${other.id}`);
    expect(await screen.findByText("Không tìm thấy job này hoặc bạn không có quyền xem.")).toBeInTheDocument();
  });
});
