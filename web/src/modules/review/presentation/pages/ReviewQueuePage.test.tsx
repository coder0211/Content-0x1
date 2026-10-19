import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { server } from "@/app/mocks/server";
import { resetLibraryMocks } from "@/modules/library/infrastructure/mocks";
import { api, fail, mockSession, page, rest } from "@/shared/api/mock";
import { renderApp } from "@/test/render";

afterEach(() => resetLibraryMocks());

const queueList = () => screen.findByRole("complementary", { name: "Danh sách chờ duyệt" });

describe("ReviewQueuePage (ACS-081)", () => {
  it("approves and requests changes without leaving the queue", async () => {
    mockSession.signInAs("reviewer@acs.local");
    const { router } = renderApp("/review");
    const user = userEvent.setup();

    // Oldest submission first, auto-selected.
    const pane = await screen.findByRole("article", { name: "Duyệt: Bài blog: Câu chuyện thương hiệu" });
    const list = await queueList();
    expect(within(list).getAllByRole("button").map((b) => b.textContent)).toEqual([
      expect.stringContaining("Bài blog"),
      expect.stringContaining("Poster khuyến mãi"),
      expect.stringContaining("Bài Facebook"),
    ]);

    await user.type(within(pane).getByLabelText("Nhận xét"), "Ổn");
    await user.click(within(pane).getByRole("button", { name: "Duyệt asset" }));
    expect(await screen.findByText('Đã duyệt "Bài blog: Câu chuyện thương hiệu"')).toBeInTheDocument();

    // Next asset is selected in place; the decided one leaves the queue.
    const poster = await screen.findByRole("article", { name: "Duyệt: Poster khuyến mãi Tết 20%" });
    await waitFor(() => expect(within(list).queryByText("Bài blog: Câu chuyện thương hiệu")).not.toBeInTheDocument());
    expect(within(poster).getByText(/kiểm tra chữ trên poster: có/)).toBeInTheDocument();

    // Request changes requires a comment.
    await user.click(within(poster).getByRole("button", { name: "Yêu cầu sửa" }));
    expect(within(poster).getByText("Nhập nhận xét để người tạo biết cần sửa gì.")).toBeInTheDocument();
    await user.type(within(poster).getByLabelText("Nhận xét"), "Chữ quá nhỏ");
    await user.click(within(poster).getByRole("button", { name: "Yêu cầu sửa" }));
    expect(await screen.findByText('Đã yêu cầu sửa "Poster khuyến mãi Tết 20%"')).toBeInTheDocument();
    await screen.findByRole("article", { name: "Duyệt: Bài Facebook: Bánh kem Tết vị trà xanh" });
    expect(router.state.location.pathname).toBe("/review");
  });

  it("shows a word diff against the previous text version", async () => {
    mockSession.signInAs("reviewer@acs.local");
    renderApp("/review?type=content");
    const user = userEvent.setup();
    const list = await queueList();
    await user.click(await within(list).findByRole("button", { name: /Bài Facebook/ }));
    const pane = await screen.findByRole("article", { name: "Duyệt: Bài Facebook: Bánh kem Tết vị trà xanh" });
    await user.click(await within(pane).findByRole("button", { name: "So với v1" }));
    const diff = within(pane).getByLabelText("So sánh với phiên bản trước");
    expect(diff.querySelector("ins")?.textContent).toContain("giao tận nhà");
    expect(within(pane).getByText(/\+\d+ từ, −\d+ từ so với phiên bản trước/)).toBeInTheDocument();
  });

  it("blocks self-approval for the asset creator", async () => {
    mockSession.signInAs("lead@acs.local");
    renderApp("/review");
    const pane = await screen.findByRole("article", { name: "Duyệt: Bài blog: Câu chuyện thương hiệu" });
    expect(within(pane).getByText(/Bạn không thể tự duyệt asset do chính mình tạo/)).toBeInTheDocument();
    expect(within(pane).queryByRole("button", { name: "Duyệt asset" })).not.toBeInTheDocument();
  });

  it("renders the empty, error and no-permission states", async () => {
    mockSession.signInAs("reviewer@acs.local");
    server.use(rest.get(api("/reviews/queue"), () => page([], null)));
    const first = renderApp("/review");
    expect(await screen.findByText("Không có asset nào chờ duyệt.")).toBeInTheDocument();
    first.unmount();

    server.use(rest.get(api("/reviews/queue"), () => fail(500, "INTERNAL", "boom")));
    const second = renderApp("/review");
    // 5xx is retried twice with backoff before the error state shows.
    expect(await screen.findByText("Không tải được dữ liệu.", undefined, { timeout: 6000 })).toBeInTheDocument();
    second.unmount();

    mockSession.signInAs("creator@acs.local");
    renderApp("/review");
    expect(await screen.findByText("Bạn không có quyền xem mục này. Liên hệ Admin.")).toBeInTheDocument();
  }, 15_000);
});
