import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http as mswHttp } from "msw";
import { server } from "@/app/mocks/server";
import { api, fail, mockSession } from "@/shared/api/mock";
import { renderApp } from "@/test/render";
import {
  MOCK_CAMPAIGN,
  resetContentMocks,
  setContentMockPolls,
} from "../../infrastructure/mocks";

beforeEach(() => setContentMockPolls(1));
afterEach(() => resetContentMocks());

async function generate() {
  const user = userEvent.setup();
  await user.type(
    await screen.findByLabelText("Đối tượng", undefined, { timeout: 5000 }),
    "Sinh viên năm 2",
  );
  await user.type(
    screen.getByLabelText("Thông điệp chính"),
    "Tự tin giao tiếp",
  );
  await user.click(screen.getByRole("button", { name: "Viết 3 phương án" }));
  return user;
}

describe("Content Studio", { timeout: 30000 }, () => {
  it("Flow A: generate 3 variants → edit → save revision → rewrite → history", async () => {
    mockSession.signInAs("creator@acs.local");
    const { router } = renderApp("/content");
    const user = await generate();

    const tabs = await screen.findByRole(
      "tablist",
      { name: "Phương án" },
      { timeout: 5000 },
    );
    expect(
      within(tabs)
        .getAllByRole("tab")
        .map((t) => t.textContent),
    ).toEqual(["Phương án A", "Phương án B", "Phương án C"]);
    expect(router.state.location.search).toContain("assets=");

    const title = await screen.findByLabelText("Tiêu đề", undefined, {
      timeout: 5000,
    });
    expect(title).toHaveValue("Tự tin giao tiếp — phương án A");
    expect(screen.getByText("≈ $0.02")).toBeInTheDocument();

    // Edit and save a revision (version 2).
    await user.clear(title);
    await user.type(title, "Tiêu đề đã sửa");
    await user.click(screen.getByRole("button", { name: "Lưu phiên bản" }));
    const history = await screen.findByRole(
      "list",
      { name: "Lịch sử phiên bản" },
      { timeout: 5000 },
    );
    await waitFor(
      () =>
        expect(
          within(
            screen.getByRole("list", { name: "Lịch sử phiên bản" }),
          ).getByText(/Phiên bản 2/),
        ).toBeInTheDocument(),
      { timeout: 5000 },
    );
    expect(screen.getByLabelText("Tiêu đề")).toHaveValue("Tiêu đề đã sửa");

    // Quick rewrite creates version 3; older versions stay restorable.
    await user.click(
      within(screen.getByRole("toolbar", { name: "Viết lại nhanh" })).getByRole(
        "button",
        { name: "Rút gọn" },
      ),
    );
    await waitFor(
      () =>
        expect(
          within(
            screen.getByRole("list", { name: "Lịch sử phiên bản" }),
          ).getByText(/Phiên bản 3/),
        ).toBeInTheDocument(),
      {
        timeout: 5000,
      },
    );
    expect(
      screen.getByRole("button", { name: "Khôi phục phiên bản 1" }),
    ).toBeInTheDocument();
  });

  it("shows inline warnings and drops them when edited out", async () => {
    mockSession.signInAs("creator@acs.local");
    renderApp("/content");
    const user = await generate();
    await user.click(
      await screen.findByRole(
        "tab",
        { name: "Phương án B" },
        { timeout: 5000 },
      ),
    );
    const inline = await screen.findByRole(
      "list",
      { name: "Cảnh báo trong nội dung" },
      { timeout: 5000 },
    );
    expect(
      within(inline).getByRole("button", {
        name: /Số liệu chưa xác thực: giảm 50%/,
      }),
    ).toBeInTheDocument();
    const body = screen.getByLabelText("Nội dung") as HTMLTextAreaElement;
    await user.click(within(inline).getByRole("button", { name: /giảm 50%/ }));
    expect(
      body.value.slice(body.selectionStart, body.selectionEnd).toLowerCase(),
    ).toBe("giảm 50%");
    await user.clear(body);
    await user.type(body, "Lớp nhỏ, giáo viên bản ngữ.");
    expect(
      screen.queryByRole("list", { name: "Cảnh báo trong nội dung" }),
    ).not.toBeInTheDocument();
    expect(screen.getByText("Cảnh báo (0)")).toBeInTheDocument();
  });

  it("sends a variant to Video Studio", async () => {
    mockSession.signInAs("creator@acs.local");
    const { router } = renderApp("/content");
    const user = await generate();
    await user.click(
      await screen.findByRole(
        "button",
        { name: "Gửi sang Video Studio" },
        { timeout: 5000 },
      ),
    );
    await waitFor(
      () => expect(router.state.location.pathname).toBe("/videos"),
      { timeout: 5000 },
    );
    expect(router.state.location.search).toMatch(/^\?script_from_asset_id=/);
  });

  it("uses the campaign brief when opened with ?campaign_id", async () => {
    mockSession.signInAs("creator@acs.local");
    renderApp(`/content?campaign_id=${MOCK_CAMPAIGN.id}`);
    expect(
      await screen.findByText(MOCK_CAMPAIGN.name, undefined, { timeout: 5000 }),
    ).toBeInTheDocument();
    expect(
      screen.queryByText("Brand", { selector: "label" }),
    ).not.toBeInTheDocument();
  });

  it("shows the over-budget banner at the generate area", async () => {
    mockSession.signInAs("creator@acs.local");
    server.use(
      mswHttp.post(api("/content/generations"), () =>
        fail(
          429,
          "JOB_BUDGET_EXCEEDED",
          "Chi phí dự toán vượt hạn mức cho một tác vụ",
        ),
      ),
    );
    renderApp("/content");
    await generate();
    const form = screen.getByRole("form", { name: "Brief" });
    expect(
      await within(form).findByText(/vượt hạn mức/, undefined, {
        timeout: 5000,
      }),
    ).toBeInTheDocument();
  });

  it("shows the empty state before any generation", async () => {
    mockSession.signInAs("creator@acs.local");
    renderApp("/content");
    expect(
      await screen.findByText(/Chưa có phương án nào/, undefined, {
        timeout: 5000,
      }),
    ).toBeInTheDocument();
  });

  it("shows an error state when the asset cannot be loaded", async () => {
    mockSession.signInAs("creator@acs.local");
    server.use(
      mswHttp.get(api("/assets/:id"), () =>
        fail(404, "NOT_FOUND", "Không tìm thấy dữ liệu"),
      ),
    );
    renderApp("/content?asset_id=00000000-0000-4000-8000-00000000abcd");
    expect(
      await screen.findByRole("button", { name: "Thử lại" }, { timeout: 5000 }),
    ).toBeInTheDocument();
  });

  it("denies reviewers (no generate.run)", async () => {
    mockSession.signInAs("reviewer@acs.local");
    renderApp("/content");
    expect(
      await screen.findByText(
        "Bạn không có quyền xem mục này. Liên hệ Admin.",
        undefined,
        { timeout: 5000 },
      ),
    ).toBeInTheDocument();
  });
});
