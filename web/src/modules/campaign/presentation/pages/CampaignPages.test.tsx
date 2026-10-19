import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { server } from "@/app/mocks/server";
import { resetBrandMocks } from "@/modules/brand/infrastructure/mocks";
import { api, fail, mockSession, page, rest } from "@/shared/api/mock";
import { renderApp } from "@/test/render";
import { MOCK_CAMPAIGN_ID, MOCK_DRAFT_CAMPAIGN_ID, resetCampaignMocks, setCampaignMockPlanStepMs } from "../../infrastructure/mocks";

beforeEach(() => setCampaignMockPlanStepMs(0));

afterEach(() => {
  resetCampaignMocks();
  resetBrandMocks();
});

describe("Campaign list (ACS-026)", () => {
  it("lists campaigns and filters by status tab", async () => {
    mockSession.signInAs("lead@acs.local");
    renderApp("/campaigns");
    expect(await screen.findByRole("cell", { name: "Khóa giao tiếp T11" })).toBeInTheDocument();
    expect(screen.getByRole("cell", { name: "Tuyển sinh Tết" })).toBeInTheDocument();
    expect(await screen.findAllByRole("cell", { name: "Anh ngữ Mẫu" })).toHaveLength(2);
    const user = userEvent.setup();
    await user.click(screen.getByRole("tab", { name: "Nháp" }));
    await waitFor(() => expect(screen.queryByRole("cell", { name: "Khóa giao tiếp T11" })).not.toBeInTheDocument());
    expect(screen.getByRole("cell", { name: "Tuyển sinh Tết" })).toBeInTheDocument();
  });

  it("shows the empty state with one action", async () => {
    mockSession.signInAs("lead@acs.local");
    server.use(rest.get(api("/campaigns"), () => page([])));
    renderApp("/campaigns");
    expect(await screen.findByText("Chưa có campaign nào.")).toBeInTheDocument();
    expect(screen.getAllByRole("button", { name: /Tạo campaign/ }).length).toBeGreaterThan(0);
  });

  it("shows the error state with retry", async () => {
    mockSession.signInAs("lead@acs.local");
    server.use(rest.get(api("/campaigns"), () => fail(403, "FORBIDDEN", "no")));
    renderApp("/campaigns");
    expect(await screen.findByText("Không tải được dữ liệu.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Thử lại" })).toBeInTheDocument();
  });

  it("hides creation from reviewers and blocks the wizard route", async () => {
    mockSession.signInAs("reviewer@acs.local");
    renderApp("/campaigns");
    expect(await screen.findByRole("cell", { name: "Khóa giao tiếp T11" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Tạo campaign/ })).not.toBeInTheDocument();
  });
});

describe("Brief wizard (ACS-026)", () => {
  it("validates both steps and creates a draft campaign", async () => {
    mockSession.signInAs("creator@acs.local");
    const { router } = renderApp("/campaigns/new");
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "Tiếp tục" }));
    expect(screen.getAllByText("Bắt buộc nhập.").length).toBe(3);

    await user.type(screen.getByLabelText(/Tên campaign/), "Hè 2027");
    await waitFor(() => expect(screen.getByRole("option", { name: "Anh ngữ Mẫu" })).toBeInTheDocument());
    await user.selectOptions(screen.getByLabelText(/^Brand/), "Anh ngữ Mẫu");
    await user.selectOptions(screen.getByLabelText(/Mục tiêu/), "Thu lead");
    await user.click(screen.getByRole("button", { name: "Tiếp tục" }));

    await user.click(await screen.findByRole("button", { name: "Tạo campaign" }));
    expect(screen.getAllByText("Bắt buộc nhập.").length).toBe(5);

    await user.click(await screen.findByLabelText("Khóa giao tiếp 12 tuần"));
    await user.selectOptions(screen.getByLabelText(/Ưu đãi đã xác thực/), "Giảm 20% đến 31/12 — Khóa giao tiếp 12 tuần");
    expect(screen.queryByRole("option", { name: /Tặng giáo trình/ })).not.toBeInTheDocument();
    await user.type(screen.getByLabelText(/Đối tượng mục tiêu/), "Sinh viên");
    await user.type(screen.getByLabelText(/Thông điệp chính/), "Nói tự tin");
    await user.click(screen.getByLabelText("Facebook"));
    await user.click(screen.getByRole("button", { name: "Thêm đầu ra" }));
    await user.selectOptions(screen.getByLabelText("Loại đầu ra 1"), "Poster");
    await user.type(screen.getByLabelText(/Deadline/), "2027-06-01");
    await user.click(screen.getByRole("button", { name: "Tạo campaign" }));

    await waitFor(() => expect(router.state.location.pathname).toMatch(/^\/campaigns\/[0-9a-f-]+$/));
    expect(await screen.findByRole("heading", { name: "Hè 2027" })).toBeInTheDocument();
    expect(screen.getByText("Nháp")).toBeInTheDocument();
  });
});

describe("Campaign detail", () => {
  it("links 'Tạo mới' to the studio with the campaign preselected (ACS-026 AC)", async () => {
    mockSession.signInAs("creator@acs.local");
    renderApp(`/campaigns/${MOCK_CAMPAIGN_ID}?tab=images`);
    // Assets come from the library module's /assets mock (shared asset store seed).
    expect(await screen.findByRole("cell", { name: "Poster khuyến mãi Tết 20%" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Tạo mới/ })).toHaveAttribute("href", `/images?campaign_id=${MOCK_CAMPAIGN_ID}`);
    const user = userEvent.setup();
    await user.click(screen.getByRole("tab", { name: "Video" }));
    expect(await screen.findByRole("cell", { name: "Video TikTok: Mở hộp bánh Tết" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Tạo mới/ })).toHaveAttribute("href", `/videos?campaign_id=${MOCK_CAMPAIGN_ID}`);
    server.use(rest.get(api("/assets"), () => page([])));
    renderApp(`/campaigns/${MOCK_CAMPAIGN_ID}?tab=videos`);
    expect(await screen.findByText("Campaign chưa có video nào.")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Tạo video" })).toHaveAttribute("href", `/videos?campaign_id=${MOCK_CAMPAIGN_ID}`);
  });

  it("is read-only for reviewers", async () => {
    mockSession.signInAs("reviewer@acs.local");
    renderApp(`/campaigns/${MOCK_CAMPAIGN_ID}`);
    expect(await screen.findByRole("heading", { name: "Khóa giao tiếp T11" })).toBeInTheDocument();
    expect(screen.getByText("Sinh viên năm 2–4 tại Hà Nội, TP.HCM")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Sửa brief" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Chuyển sang duyệt" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Tạo kế hoạch" })).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: /Tạo mới/ })).not.toBeInTheDocument();
  });

  it("moves the status along the state machine", async () => {
    mockSession.signInAs("lead@acs.local");
    renderApp(`/campaigns/${MOCK_DRAFT_CAMPAIGN_ID}`);
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "Bắt đầu thực hiện" }));
    expect(await screen.findByRole("button", { name: "Chuyển sang duyệt" })).toBeInTheDocument();
    expect(screen.getByText("Đang thực hiện")).toBeInTheDocument();
  });

  it("generates, edits and approves the content plan (ACS-037)", async () => {
    mockSession.signInAs("lead@acs.local");
    renderApp(`/campaigns/${MOCK_DRAFT_CAMPAIGN_ID}`);
    const user = userEvent.setup();
    expect(await screen.findByText("Chưa có content plan.")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Tạo kế hoạch" }));
    expect(await screen.findByText("Đề xuất từ AI đang chờ duyệt")).toBeInTheDocument();
    const editor = screen.getByRole("form", { name: "Sửa content plan" });
    const topic = within(editor).getByLabelText("Chủ đề 1");
    await user.clear(topic);
    await user.type(topic, "Chủ đề đã sửa");
    await user.click(within(editor).getByRole("button", { name: "Duyệt plan" }));
    const list = await screen.findByRole("list", { name: "Checklist đầu ra" });
    expect(within(list).getByText("Chủ đề đã sửa")).toBeInTheDocument();
    await user.click(within(list).getByRole("checkbox", { name: "Đã xong: Chủ đề đã sửa" }));
    await waitFor(() => expect(within(screen.getByRole("list", { name: "Checklist đầu ra" })).getByRole("checkbox", { name: "Đã xong: Chủ đề đã sửa" })).toBeChecked());
  });

  it("shows the error state for an unknown campaign", async () => {
    mockSession.signInAs("lead@acs.local");
    renderApp("/campaigns/20000000-0000-4000-8000-0000000000ff");
    expect(await screen.findByText("Không tải được dữ liệu.")).toBeInTheDocument();
  });
});
